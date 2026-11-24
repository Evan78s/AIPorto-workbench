# 配置清单（需要你填写，模板里没有真实值）

按下面顺序填。**任何一项都不要照抄示例值**；示例只是说明格式。

## 1. 项目根目录的 `.env`

从 `.env.example` 复制：`cp .env.example .env`，然后 `chmod 600 .env`，再用文本编辑器打开。

| 配置项 | 谁提供 | 怎么获得 / 填写说明 | 不填会怎样 |
| --- | --- | --- | --- |
| `POSTGRES_DB` | 你定 | 生产 Compose 用的数据库名 | compose 启动失败 |
| `POSTGRES_USER` | 你定 | 数据库用户 | 同上 |
| `POSTGRES_PASSWORD` | 你定 | 数据库密码 | 同上 |
| `DATABASE_URL` | 本地运行时才需要 | 格式 `postgres://用户:密码@地址:端口/库名?sslmode=disable`。**容器里由 Compose 自动改写为服务名**，不用你填容器版连接串 | 本地启动网关失败 |
| `REDIS_ADDR` | 你自己的 Redis | 本地默认 `127.0.0.1:6379`；容器里由 Compose 覆盖 | 用默认值即可 |
| `REDIS_PASSWORD` | 你自己的 Redis | 没设密码就留空 | 留空即可 |
| `API_KEY_PEPPER_B64` | 你生成 | 执行 `openssl rand -base64 32`，整段贴进来。**必须离线备份**，丢失会让全部客户 key 失效 | 网关启动失败 |
| `UPSTREAM_MASTER_KEY_B64` | 你生成 | 再执行一次 `openssl rand -base64 32`。**必须离线备份**，丢失会让全部上游 key 无法解密 | 网关启动失败 |
| `ADMIN_USER` | 你定 | 运营后台登录账号 | 后台接口返回 503 |
| `ADMIN_PASSWORD` | 你定 | 后台登录第一步密码 | 同上 |
| `ADMIN_TOTP_SECRET` | 你生成 | 执行 `openssl rand -base64 20 \| tr -d '\n' \| base32 \| tr -d '='`；把输出填进来，同时录入验证器 App。留空则后台拒绝所有业务请求 | 后台无法完成第二步登录 |
| `ALERT_LOW_BALANCE_THRESHOLD` | 你定 | 余额告警阈值，与充值金额同单位。不填默认 100 | 用默认值 |
| `UPSTREAM_HEADER_TIMEOUT` | 一般不用改 | 等待上游响应头的时长，如 `15s` | 用默认值 |
| `METERING_BATCH_SIZE` / `METERING_FLUSH_INTERVAL` | 一般不用改 | 用量落库批量大小与最大等待 | 用默认值 |

> `.env` 的值不要加引号，不要在行尾写 `#` 注释，否则程序会按原样读取。

## 2. 种子命令的额外变量（不属于 .env）

| 变量 | 谁提供 | 说明 |
| --- | --- | --- |
| `SEED_UPSTREAM_KEY_PLAIN` | 上游模型厂商 | 厂商控制台里创建的真实 API key。只在执行 `cmd/seed` 时通过环境变量传入，用完从 shell 里 unset。**不要在代码、文档或聊天里粘贴真实值** |

## 3. 数据库迁移（已有数据库时必须手动执行）

全新安装（`docker compose up -d` 且数据卷为空）时三个 SQL 会自动执行，可跳过本节。

已经跑过上一轮 MVP、数据卷里有数据时，执行三个迁移（`003` 新增 `audit_logs`、TOTP 防重放表，并补上 `api_keys.revoked_at`）：

```bash
for f in 001_mvp.sql 002_console.sql 003_audit_2fa.sql; do
  psql "$DATABASE_URL" -f "migrations/$f"
done
```

容器版命令见 `DEPLOYMENT.md` 第 6 节。看到所有语句执行完、没有 ERROR 即可；这些脚本只新增表/列/索引，不改动已有数据。

## 4. 需要你录入的业务数据（没有预置假数据）

| 数据 | 在哪里填 | 说明 |
| --- | --- | --- |
| 套餐 | 运营后台 →「套餐与价格」 | 先建套餐，创建租户时填它的 code |
| 模型价格 | 运营后台 →「套餐与价格」 | 模型名必须与 `usage_logs.model_resolved` 完全一致；不填则费用显示 0 |
| 租户 + API key + 控制台令牌 | 运营后台 →「租户与 key」→ 创建租户 | 明文只显示一次，立刻复制保存 |
| 额度 | 运营后台 →「租户与 key」→ 设置额度 | 会同时更新该租户现有 active key 的限流参数 |
| 充值 | 运营后台 →「账本流水」→ 手动充值 | 按真实收款填写，流水号用于防重复 |
| 上游 provider 与 key | `cmd/seed` 或自行录入 | 明文只作为环境变量传入，库里只存密文 |

## 5. 浏览器里需要填的（不写入代码）

| 位置 | 填什么 |
| --- | --- |
| `/admin` 页面顶部 | 第一步填 `ADMIN_USER` / `ADMIN_PASSWORD`，第二步填验证器里的 6 位动态码。会话只放 sessionStorage，关闭标签页失效 |
| `/console` 页面顶部 | 运营方签发给该租户的 `cw-` 开头控制台令牌 |

不要在公用电脑上保存这些值。

## 6. 上游地址

`cmd/seed` 的 `-provider-base-url` 填厂商的 origin（只到域名，不带 `/v1`），因为客户端请求路径会原样拼接。具体地址以厂商官方文档为准，本清单不提供示例域名。
