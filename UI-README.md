# 运营后台 + 客户控制台（React）

**先读 `CONFIG-CHECKLIST.md` 把配置填完，再回来执行下面的步骤。生产部署请优先看 `DEPLOYMENT.md`（一条 Compose 命令起全套服务）。**

## 1. 整体结构

```
浏览器  ──(nginx 或 Vite 的 /api 代理)──>  Go 网关 :8080  ──>  PostgreSQL + Redis
   /admin          运营后台 API /api/admin/**     （必须双因素：密码 + TOTP）
   /console        控制台 API   /api/console/**   （Bearer cw- 租户令牌）
   /v1/**          模型调用（原有网关逻辑）
```

没有引入任何新的付费云服务。前端依赖只有 React / Vite / react-router，后端是 Go + PostgreSQL + Redis，全部跑在你自己的服务器上。

## 2. 路由设计

### 前端页面路由

| 路径 | 页面 | 说明 |
| --- | --- | --- |
| `/admin/tenants` | 租户与 key | 列表、创建租户并签发 key、签发额外 key、设置额度 |
| `/admin/providers` | 厂商管理 | 新增/停用厂商、加密写入厂商 key（只显示掩码）、查看该厂商 key 列表。协议见下方「厂商地址与协议」 |
| `/admin/routes` | 模型路由 | 新增/编辑/启停/删除路由；全局路由与租户专属路由，别名→真实模型 |
| `/admin/usage` | 用量看板 | 按天/按租户/按模型聚合，含 token 与费用，按天有曲线 |
| `/admin/ledger` | 账本流水 | 手动充值、流水列表 |
| `/admin/alerts` | 异常告警 | 余额低、单日用量 3 倍突增、上游 429 激增 |
| `/admin/plans` | 套餐与价格 | 新增可售套餐（带字段说明）、套餐列表、模型价格录入 |
| `/admin/system` | 系统更新 | 查看当前版本、复制一键升级命令、手动回滚说明（不执行系统操作） |
| `/console/overview` | 本月总览 | 本月 token、剩余额度、费用、余额、30 天曲线 |
| `/console/usage` | 用量曲线 | 按天曲线 + 按模型表格 |
| `/console/keys` | API key | 列表、停用、重建（明文只显示一次） |
| `/console/billing` | 充值与账单 | 充值记录、用量 CSV / 账单 CSV 下载 |
| `/console/snippets` | 调用示例 | curl / Python 示例，占位符不替换成真实 key |
| （用户端独立站点 `frontend-user`） | 首页/注册/登录/套餐/订单 | 见 `USER-PORTAL.md`，独立域名 `user.aiporto.net`，端口 8082 |

### 厂商地址与协议（重要）

网关对外的 API 路径是 `/v1/chat/completions`，转发时会把完整路径拼到厂商「API 地址」后面。不同厂商的 OpenAI 兼容端点并不统一，所以协议字段决定要不要去掉 `/v1`：

| 厂商 | API 地址（不要带尾部 `/`） | 协议 | 实际请求地址 |
| --- | --- | --- | --- |
| DeepSeek | `https://api.deepseek.com` | `openai_compatible` | `https://api.deepseek.com/v1/chat/completions` |
| OpenAI | `https://api.openai.com/v1` | `openai_compatible` | `https://api.openai.com/v1/chat/completions` |
| 火山方舟 | `https://ark.cn-beijing.volces.com/api/v3` | `openai_compatible_no_v1` | `https://ark.cn-beijing.volces.com/api/v3/chat/completions` |

选错协议上游会返回 404；在厂商列表点「编辑」可直接改地址和协议，改完路由缓存最多 5 秒后生效。厂商地址不要填 `/chat/completions`，网关自己会拼路径。

### 后端接口清单

运营后台（登录两步；除登录接口外全部需要双因素会话 Bearer 令牌）：

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| POST | `/api/admin/auth/login` | 第一步：账号密码换 90 秒 challenge，不签发会话 |
| POST | `/api/admin/auth/verify` | 第二步：TOTP 动态码换 8 小时会话，同码不可重放 |
| GET | `/api/admin/tenants` | 租户列表（余额、本月用量、有效 key 数） |
| POST | `/api/admin/tenants` | 创建租户 + 套餐 + 订阅 + API key + 控制台令牌，明文只返回一次 |
| POST | `/api/admin/tenants/{id}/keys` | 为租户签发额外 API key |
| GET | `/api/admin/tenants/{id}/keys` | 租户的 key 列表（无哈希、无明文） |
| PUT | `/api/admin/tenants/{id}/quota` | 设置套餐与限流额度 |
| GET / POST | `/api/admin/plans` | 套餐列表 / 新建或更新套餐 |
| GET | `/api/admin/system/version` | 当前构建版本（git commit）与升级命令，只读 |
| GET | `/api/admin/usage?group=day\|tenant\|model&tenant_id=&days=` | 用量与费用聚合 |
| GET | `/api/admin/ledger?tenant_id=` | 账本流水 |
| POST | `/api/admin/recharge` | 手动充值（reference_id 幂等） |
| GET | `/api/admin/alerts` | 三类异常告警 |
| PUT | `/api/admin/pricing` | 录入/更新模型价格 |
| GET / POST | `/api/admin/providers` | 厂商列表（含活跃 key 数）/ 新增厂商 |
| PUT / DELETE | `/api/admin/providers/{id}` | 修改厂商（地址/协议/启停）/ 归档厂商并停用其 key 与路由 |
| GET / POST | `/api/admin/providers/{id}/keys` | 厂商 key 列表（仅掩码）/ 加密写入新 key（明文只收一次） |
| DELETE | `/api/admin/provider-keys/{id}` | 停用厂商 key |
| GET / POST | `/api/admin/routes` | 路由列表 / 新增路由 |
| PUT / DELETE | `/api/admin/routes/{id}` | 修改路由（厂商/模型/优先级/启停）/ 删除路由 |

用户端公开接口（无需鉴权）：

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| POST | `/api/user/register` | 发邮箱验证码（密码 bcrypt 哈希，验证码只存摘要） |
| POST | `/api/user/verify` | 核对验证码，创建用户+租户+会话，返回一次性会话令牌 |
| POST | `/api/user/login` | 邮箱密码登录 |
| GET | `/api/user/plans` | 可售套餐列表 |
| GET | `/api/user/pay/channels` | 当前启用的支付渠道 |
| POST | `/api/user/pay/alipay/notify` | 支付宝异步回调（RSA2 验签 + 金额比对 + 幂等） |
| POST | `/api/user/pay/wechat/notify` | 微信 v3 回调（平台公钥验签 + AES-GCM 解密） |

用户端登录后（`Authorization: Bearer us-...`）：

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| POST | `/api/user/logout` | 登出 |
| GET | `/api/user/me` | 当前用户与套餐 |
| POST | `/api/user/orders` | 下单（金额从套餐表读取，不接受前端传入） |
| GET | `/api/user/orders` | 我的订单 |
| GET | `/api/user/orders/{order_no}` | 订单详情（含支付二维码/转账信息） |
| POST | `/api/user/orders/{order_no}/cancel` | 取消待支付订单 |
| POST | `/api/user/orders/{order_no}/credentials` | 领取一次性明文 API Key 与控制台令牌 |

运营后台新增（复用双因素鉴权）：

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| GET | `/api/admin/user-orders` | 用户端订单列表 |
| POST | `/api/admin/user-orders/{order_no}/confirm` | 对公转账收款确认并开通 |

客户控制台（全部需要 `Authorization: Bearer cw-...`）：

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| GET | `/api/console/overview` | 本月用量、剩余额度、余额 |
| GET | `/api/console/usage/by-model?days=` | 按模型用量 |
| GET | `/api/console/usage/daily?days=` | 按天用量 |
| GET | `/api/console/keys` | 本租户 key 列表 |
| POST | `/api/console/keys/{id}/disable` | 停用本租户 key |
| POST | `/api/console/keys/rotate` | 停用全部旧 key 并签发新 key |
| GET | `/api/console/recharges` | 本租户充值记录 |
| GET | `/api/console/export/usage.csv?days=` | 用量 CSV（脱敏） |
| GET | `/api/console/export/bill.csv?days=` | 账单 CSV（脱敏） |
| GET | `/api/console/snippets` | 调用示例代码 |

## 3. 两条硬约束怎么保证

**租户隔离**：控制台令牌在中间件里解析成 `tenant_id`，写进 request context；所有控制台 SQL 常量都以 `$1` 接收这个 id，并且带 `WHERE tenant_id = $1`。前端传来的 `tenant_id` 参数一律不参与控制台查询。单元测试 `TestConsoleQueriesHaveTenantFilter` 会逐条检查；漏写租户过滤时测试失败。

**CSV 脱敏**：导出走白名单结构体 `UsageExportRow`，字段只有 request_id、时间、模型、路径、token 数、状态码、耗时、费用。代码里没有 prompt 正文字段，因此不可能把 prompt 写进 CSV；`TestCSVExportAllowlist` 校验表头与禁用字段。

写操作审计与双因素登录的实现细节见 `SECURITY.md`。

## 4. 从零跑起来（按顺序做）

1. 完成 `CONFIG-CHECKLIST.md`，确保项目根目录有填好的 `.env`。
2. 启动数据库和 Redis：
   ```bash
   cd aiporto
   docker compose up -d
   ```
   老数据卷还要补三个迁移，命令见 `DEPLOYMENT.md` 第 6 节。
3. 启动 Go 网关（**必须在项目根目录执行**，程序读当前目录的 `.env`）：
   ```bash
   go run ./cmd/gateway
   ```
   看到 `aiporto listening on :8080` 即成功。
4. 新开一个终端，初始化前端依赖（只需一次）：
   ```bash
   cd aiporto/frontend
   npm install
   ```
5. 启动界面：
   ```bash
   npm run dev
   ```
   看到 `Local: http://127.0.0.1:5173/` 即成功。
6. 浏览器打开 `http://127.0.0.1:5173/admin`，在页面顶部输入 `ADMIN_USER` + `ADMIN_PASSWORD`，点「下一步」，再输入验证器里的 6 位动态码，点「登录」。会话只存在 sessionStorage，关闭标签页需重新登录。
7. 在「套餐与价格」录入套餐和模型价格；在「租户与 key」创建租户，**立刻复制显示的 API key 和控制台令牌**。
8. 浏览器打开 `http://127.0.0.1:5173/console`，填入刚复制的 `cw-` 令牌，即可只看到该租户的数据。

> 如果只想看界面不启后端，可以 `npm run build` 生成静态文件，但页面会报「请求失败」，因为没有 API。本项目不提供假数据模式，避免误把演示数据当成真实账目。

## 5. 费用为什么可能是 0

`usage_logs.cost_amount` 在 MVP 阶段恒为 0。本轮通过视图 `v_usage_cost` 用 `model_pricing` 表实时计算费用：**模型名必须与 `usage_logs.model_resolved` 一致**。价格表为空时费用显示 0，不影响 token 统计和限流。

## 6. 自测结果（2026-11-23，本轮实际执行）

| 跑了什么 | 实际输出 |
| --- | --- |
| `go mod tidy` | 下载已有依赖完成，无报错 |
| `go build ./...` | `BUILD_OK` |
| `go vet ./...` | `VET_OK` |
| `go test ./...` | `ok aiporto/internal/consoleapi 0.795s`；`ok aiporto/internal/util 1.507s`；其余包无测试文件；`TEST_OK` |
| `npm run build`（Vite 5.4.21） | `✓ 49 modules transformed`，`✓ built in 1.66s`，产物 `dist/index.html` + CSS 3.23kB + JS 190.63kB |
| `bash scripts/check-redaction.sh` | 六项全部 PASS（正文列、前端 key、日志、审计 detail、租户过滤、无 Basic Auth） |

本轮修复的三处问题：`exportBill` 里 CSV writer 覆盖了响应写入器 `w`（编译错误）；TOTP 测试向量误用了 RFC 文档的 8 位码而实现是 6 位（测试失败）；`.env.example` 缺 `POSTGRES_*`、开发编排缺 003 迁移挂载（部署一致性）。

**没有跑的**：需要真实 PostgreSQL / Redis / 上游 key 的集成场景，以及 200 并发压测。原因是你还没有填真实配置和上游凭据（见 `CONFIG-CHECKLIST.md`）。填完后按 `LOADTEST.md` 跑，结果记入 `ACCEPTANCE.md` 第 20 条。
