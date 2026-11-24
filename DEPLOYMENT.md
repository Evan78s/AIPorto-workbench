# 生产部署：一条命令起全套服务

## 0. 前置条件

- 一台你自己的服务器（Linux / macOS），已装 Docker Engine 24+ 与 Docker Compose v2
- `openssl`（用来生成三个密钥）
- **不需要在本机安装 Go 或 Node**：镜像里会自动编译

没有引入任何新的付费云服务；Prometheus / node-exporter / nginx 都是开源组件，跑在你自己的机器上。

## 1. 只填你自己的值

```bash
cd aiporto
cp .env.example .env
chmod 600 .env
```

用文本编辑器打开 `.env`，按 `CONFIG-CHECKLIST.md` 填写。生产编排额外需要这三个变量（模板里是占位符）：

| 变量 | 谁提供 | 说明 |
| --- | --- | --- |
| `POSTGRES_DB` | 你定 | 数据库名 |
| `POSTGRES_USER` | 你定 | 数据库用户 |
| `POSTGRES_PASSWORD` | 你定 | 数据库密码 |

注意：`.env` 里的 `DATABASE_URL` 和 `REDIS_ADDR` 在容器里会被 Compose 自动改写为服务名（`postgres:5432` / `redis:6379`），所以你不需要为容器单独计算连接串。本地非容器运行时才用 `.env` 里的 `127.0.0.1`。

三个密钥各自独立生成，不要复用：

```bash
openssl rand -base64 32   # 填给 API_KEY_PEPPER_B64
openssl rand -base64 32   # 填给 UPSTREAM_MASTER_KEY_B64
```

`ADMIN_TOTP_SECRET` 的生成与录入方法见 `SECURITY.md` 第 4 节。生成结果请离线备份；丢失 pepper 会让全部客户 key 失效，丢失主密钥会让全部上游 key 无法解密。

## 2. 启动

```bash
docker compose -f docker-compose.prod.yaml up -d --build
docker compose -f docker-compose.prod.yaml ps
```

`STATUS` 列全部变成 `healthy` 才算成功。第一次构建会慢一些。

| 端口 | 服务 | 建议暴露范围 |
| --- | --- | --- |
| 8080 | 网关 API（`/v1/*`、`/api/*`） | 内网或你的 TLS 反代之后 |
| 8081 | 管理后台 / 客户控制台界面 | 仅内网；不要直接对公网开放 |
| 9090 | Prometheus | 仅内网 |

## 3. 健康检查

```bash
curl -fsS http://127.0.0.1:8080/healthz   # 进程活着 → ok
curl -fsS http://127.0.0.1:8080/readyz    # Postgres + Redis 都可达 → ok
curl -fsS http://127.0.0.1:8080/metrics   # 监控指标
```

每个容器都配了 `HEALTHCHECK`：网关查 `/healthz`，Postgres 用 `pg_isready`，Redis 用 `redis-cli ping`，界面容器查首页。启动顺序由 `depends_on.condition: service_healthy` 控制，不会出现网关先于数据库启动。

## 4. 日志轮转

所有服务统一使用 `json-file` 驱动，`max-size: 10m`、`max-file: 5`。单个容器日志最多约 50MB，写满后 oldest 自动丢弃，不会撑爆磁盘。

```bash
docker compose -f docker-compose.prod.yaml logs -f gateway
docker compose -f docker-compose.prod.yaml logs --tail=100 postgres
```

日志内容本身也已脱敏：不打印鉴权头、客户 key 明文、上游 key 明文或请求体。可用 `./scripts/check-redaction.sh` 静态复核。

## 5. 初始化业务数据（没有预置假数据）

上游 key 明文只通过环境变量传入，不写进 `.env` 或命令行历史文件：

```bash
read -rs SEED_UPSTREAM_KEY_PLAIN && export SEED_UPSTREAM_KEY_PLAIN
docker compose -f docker-compose.prod.yaml run --rm --entrypoint seed \
  -e SEED_UPSTREAM_KEY_PLAIN gateway \
  -tenant-name "你的租户名" -provider-code "你的provider代码" \
  -provider-base-url "厂商origin" -model-alias "你的模型别名" -model "厂商真实模型名"
unset SEED_UPSTREAM_KEY_PLAIN
```

屏幕只会出现一次客户 key 明文，立即复制保存；数据库里只有哈希。之后所有租户、套餐、额度、充值都在管理后台页面里填。

## 6. 已有老数据卷时怎么升级

`003_audit_2fa.sql` 只在**全新空数据卷**时自动执行。如果你沿用上一轮 MVP 的数据卷，手动补一次三个迁移：

```bash
for f in 001_mvp.sql 002_console.sql 003_audit_2fa.sql; do
  docker compose -f docker-compose.prod.yaml cp "migrations/$f" "postgres:/tmp/$f"
  docker compose -f docker-compose.prod.yaml exec -T postgres \
    sh -c "psql -U \"\$POSTGRES_USER\" -d \"\$POSTGRES_DB\" -f /tmp/$f"
done
```

看到所有语句执行完、没有 `ERROR` 即可。这些脚本只新增表/列/索引，不改动已有数据。

## 7. 停止与升级

```bash
docker compose -f docker-compose.prod.yaml down          # 停止但保留数据卷
docker compose -f docker-compose.prod.yaml up -d --build # 拉新代码后重建
```

`down` 不要加 `-v`，否则会删掉数据库卷。**删除数据卷前先执行 `scripts/backup.sh`。**
