# Aiporto Workbench

**品牌：Aiporto｜智港　·　万模入港，统一调度**

多租户大模型 API 网关。包含数据面网关、运营后台、客户控制台与生产部署编排。

> **要安装到阿里云服务器？** 直接看 **[ALIYUN-DEPLOY.md](ALIYUN-DEPLOY.md)** —— 按阿里云控制台的实际操作写的，含最容易漏的「安全组放行端口」，一条命令装完全套。
>
> **其他服务器 / 想了解全部方式？** 看 **[INSTALL.md](INSTALL.md)** —— 三种安装方式，以及装完必做的四件事。
> - **方式一（推荐）**：工作区根目录的 `aiporto-install.sh`，传一个文件、跑一条命令，全程不用 GitHub
> - 方式二：终端 `git clone` + 一键脚本
> - 方式三：宝塔网页上传 ZIP
> - 纯命令行细节 → `SERVER-INSTALL.md`；宝塔网页细节 → `BAOTA-INSTALL.md` / `宝塔面板安装手册.docx`
>
> 装好之后**运行不依赖 GitHub**：镜像本地编译、数据在本地 Docker 卷。GitHub 只用于更新代码。
> 想重新生成离线安装包：在项目目录跑 `bash scripts/build-offline.sh`。
>
> 发布到 GitHub 与日常更新看 `GITHUB-UPDATE.md`，部署细节看 `DEPLOYMENT.md`，设计看上级目录的 `Aiporto-Workbench-Design.md`。

## 本项目实现什么

**网关数据面**
1. 客户 key 鉴权：只接受 `sk-wb-` 前缀，数据库中只存 HMAC-SHA256 摘要，比对摘要不比对明文。
2. 额度校验：Redis Lua 原子预扣，请求结束后按真实用量校正；RPM、并发数、单日 token 三重限制，超限返回 429 + `Retry-After`。
3. 上游路由：按模型别名映射 provider 与真实模型；同一模型多个上游 key 做加权轮询；失败最多重试一次，已产生部分输出的流式请求禁止重试。
4. 流式透传：SSE 全链路不缓冲，逐块 Flush；优先读上游 `usage` 字段，缺失时本地估算 prompt/completion tokens。
5. 用量落库：`usage_logs` 异步批量写入，入队非阻塞，不增加请求延迟。
6. 每 key 限流：见第 2 项。

**运营后台 `/admin`**：租户与 key、**厂商管理、模型路由管理**、套餐与价格、用量看板、账本与手动充值、三类异常告警；密码 + TOTP 双因素登录，所有写操作进 `audit_logs`。
> 厂商 API key 可直接在后台「厂商管理」页填写，经 AES-256-GCM 加密入库，页面只显示掩码、无法再次查看明文——无需再用 seed 命令写入上游 key。seed 命令仍可用于快速初始化演示数据。
**客户控制台 `/console`**：本月用量与余额、按模型/按天曲线、key 停用与重建、充值记录与脱敏 CSV 下载、调用示例；所有查询强制带租户过滤。
**用户端 `user.aiporto.net`（B 方案）**：邮箱注册与验证、密码登录、套餐浏览、支付宝/微信/对公转账下单、支付回调自动开通 API Key 与控制台令牌；完整手册见 `USER-PORTAL.md`。
**生产配套**：Docker Compose 全套编排、健康检查、日志轮转、备份/恢复/演练脚本、Prometheus 指标与告警、200 并发 SSE 压测工具、20 条上线验收清单。

明确不做：多区域部署。（用户自助注册与支付渠道已实现，见 `USER-PORTAL.md`、`PAYMENT-SETUP.md`、`SMTP-SETUP.md`。）

## 目录结构

```
aiporto/
├── cmd/
│   ├── gateway/main.go          # 数据面服务入口
│   └── seed/main.go             # 一次性种子：租户/key/上游/路由（明文 key 只打印一次）
├── internal/
│   ├── config/config.go         # 环境变量与 .env 读取
│   ├── store/store.go           # Postgres 连接池 + Redis 客户端
│   ├── util/util.go             # UUID、HMAC、AES-GCM、token 估算
│   ├── auth/auth.go             # 客户 key 鉴权（Redis 缓存 + DB 回源）
│   ├── quota/quota.go           # Redis Lua 原子预扣 / 校正
│   ├── routing/routing.go       # 模型路由、加权轮询、密钥解密
│   ├── metering/metering.go     # 非阻塞队列 + CopyFrom 批量落库
│   ├── proxy/proxy.go           # 转发、SSE 透传、重试、token 统计
│   └── server/server.go         # chi 路由、健康检查
│   ├── consoleapi/              # 管理后台 + 控制台 API、双因素、审计
│   ├── userapi/                 # B 方案：注册/登录/订单/支付回调/自动开通
│   ├── mailer/                  # SMTP 验证码邮件（标准库）
│   ├── payment/                 # 支付宝 RSA2 / 微信 v3 / 对公转账
│   └── metrics/                 # /metrics 监控指标
├── frontend/                    # React 后台 + 控制台（Vite）
├── frontend-user/               # React 用户端网站（Vite，独立端口 8082）
├── migrations/                  # 001 网关 / 002 控制台 / 003 审计+双因素 / 004 用户端
├── monitoring/                  # Prometheus 抓取与告警规则
├── scripts/                     # update.sh 一键升级、snapshot.sh 全量快照、migrate.sh 一键迁移、备份/恢复/压测
├── docker-compose.yaml          # 本地开发：Postgres + Redis
├── docker-compose.prod.yaml     # 生产：全套六个服务
├── .env.example                 # 配置项模板（无真实值）
├── SERVER-INSTALL.md            # ★ 服务器安装手册（新手走这份）
├── DEPLOYMENT.md                # 生产部署细节
├── UI-README.md                 # 两个界面的路由与接口清单
├── USER-PORTAL.md               # ★ 用户端部署与购买流程（B 方案）
├── PAYMENT-SETUP.md             # 支付宝/微信申请与密钥配置
├── SMTP-SETUP.md                # 邮件验证码配置
├── CONFIG-CHECKLIST.md          # 需要你填写的配置清单
├── MIGRATION.md                 # ★ 换服务器迁移 / 全量快照与恢复
├── SECURITY.md / BACKUP-RESTORE.md / MONITORING.md / LOADTEST.md / ACCEPTANCE.md
└── TESTING.md                   # 网关 8 个必测场景
```

## 本地开发启动

下面是"本机已装 Go 和 Docker"时的开发流程。**服务器部署不要用这一节**，走 `SERVER-INSTALL.md`。

```bash
cd aiporto

# 1. 启动依赖（Postgres 16 + Redis 7；迁移 SQL 会在 Postgres 首次启动时自动执行）
docker compose up -d

# 2. 准备配置：复制模板并填入自己的值
cp .env.example .env
# 至少需要填：DATABASE_URL、REDIS_ADDR、API_KEY_PEPPER_B64、UPSTREAM_MASTER_KEY_B64
# 两个 32 字节密钥可用：openssl rand -base64 32

# 3. 拉取依赖并生成 go.sum
go mod tidy

# 4. 初始化演示数据（上游 key 明文通过环境变量传入）
export SEED_UPSTREAM_KEY_PLAIN='替换为上游厂商给你的 key'
go run ./cmd/seed \
  -tenant-id t_demo \
  -tenant-name "Demo" \
  -provider-code deepseek \
  -provider-base-url https://api.deepseek.com \
  -model-alias demo-model \
  -model deepseek-chat \
  -rpm 60 -concurrency 4 -daily-tokens 1000000 -weight 1
# 终端会打印 CUSTOMER_KEY_PLAINTEXT=sk-wb-...，只此一次，请立即保存

# 5. 启动网关（必须在项目根目录执行，程序会读取当前目录的 .env）
go run ./cmd/gateway

# 6. 验证
curl -s http://127.0.0.1:8080/healthz
curl -s http://127.0.0.1:8080/readyz
```

## .env 配置项说明

所有值均为占位符，模板中不包含真实凭据。

| 配置项 | 是否必填 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `GATEWAY_LISTEN_ADDR` | 否 | `:8080` | 网关监听地址 |
| `DATABASE_URL` | **是** | 无 | PostgreSQL 连接串，`postgres://用户:密码@主机:端口/库名?sslmode=disable` |
| `REDIS_ADDR` | 否 | `127.0.0.1:6379` | Redis 地址 |
| `REDIS_PASSWORD` | 否 | 空 | Redis 密码 |
| `REDIS_DB` | 否 | `0` | Redis 库号 |
| `API_KEY_PEPPER_B64` | **是** | 无 | 客户 key HMAC 用的 pepper，32 字节 base64。轮换会导致全部客户 key 失效 |
| `UPSTREAM_MASTER_KEY_B64` | **是** | 无 | 上游 key AES-256-GCM 主密钥，32 字节 base64。轮换需重新写入全部上游 key |
| `UPSTREAM_TIMEOUT` | 否 | `10m` | 预留项；当前代码不设整体超时以免切断 SSE，由响应头超时与 request context 控制 |
| `UPSTREAM_HEADER_TIMEOUT` | 否 | `15s` | 等待上游响应头的超时 |
| `METERING_BATCH_SIZE` | 否 | `200` | 批量落库行数阈值 |
| `METERING_FLUSH_INTERVAL` | 否 | `1s` | 批量落库最大等待时间 |
| `AUTH_CACHE_TTL` | 否 | `60s` | 鉴权结果在 Redis 的缓存时间 |
| `ROUTE_CACHE_TTL` | 否 | `5s` | 路由表在进程内的缓存时间 |
| `LOG_LEVEL` | 否 | `info` | 预留项 |

种子命令还会读取一个**不属于 .env** 的变量：`SEED_UPSTREAM_KEY_PLAIN`，仅在初始化时用于加密写入，命令结束后无需保留。

## 调用示例

```bash
curl -N -X POST http://127.0.0.1:8080/v1/chat/completions \
  -H "Authorization: Bearer sk-wb-替换为种子命令打印的key" \
  -H "Content-Type: application/json" \
  -d '{"model":"demo-model","stream":true,"max_tokens":128,"messages":[{"role":"user","content":"你好"}]}'
```

`model` 传的是 `routing_rules.model_alias`；网关会改写成 `model` 真实名再转发。`provider-base-url` 填 origin，不要带 `/v1`，因为客户端路径会原样拼接。

## 关键实现说明

- **鉴权**：`sk-wb-` 前缀 + HMAC-SHA256(pepper, 明文)。Redis 缓存 `gw:auth:<hash>`，未命中才查 `api_keys`。日志与错误响应中从不出现 key 明文。
- **额度**：三个 Lua 脚本动作在一个原子脚本里完成。并发用 ZSET（request_id 为 member，1 小时自动清理），因此进程崩溃也不会永久占用并发额度。预扣量 = 输入 token 估算 + `max_tokens`（上限 4096），流结束后 `Settle` 用真实用量校正。
- **重试**：只在上游返回 5xx 且尚未向客户端写出任何字节时重试一次；SSE 一旦写出第一块就锁定当前上游。
- **SSE**：响应头立即回写，每次 Write 后调用 Flush；禁用 gzip 与 Transport 缓冲。上游 `usage` 缺失时用 `choices[].delta.content` 文本估算。
- **落库**：`Reporter.Enqueue` 用非阻塞 channel，队列满则丢弃并计数；后台 worker 用 `pgx.CopyFrom` 批量写，服务退出时 drain。
- **限制**：token 估算为启发式（字符数/2），生产环境应替换为对应模型的 tokenizer；`cost_amount` 恒为 0，计费不在 MVP 范围。

## 自测

见 `TESTING.md`，包含 8 个必须验证的场景与预期结果。
