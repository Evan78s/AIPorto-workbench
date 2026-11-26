# 服务器安装手册（按顺序照做即可）

这份文档是给"不做运维的人"写的。你只需要按顺序复制命令、填几个只有你知道的值，就能把整套系统装到自己的服务器上。

> **服务器装了宝塔面板？** 改看 **[BAOTA-INSTALL.md](BAOTA-INSTALL.md)**，那份是按宝塔的操作方式写的（文件管理器、反向代理、SSL、计划任务）。本文档是纯命令行版本。

**全程不引入任何付费云服务**：Postgres、Redis、Prometheus、nginx 都是开源软件，运行在你自己的机器上。唯一需要你付费的是服务器本身和上游模型厂商的 API 用量。

---

## 开始前你要准备的东西

| 准备项 | 说明 |
| --- | --- |
| 一台服务器 | Linux（Ubuntu 22.04 / 24.04 最省事），2 核 4G 内存、20G 磁盘起 |
| 一个 SSH 登录账号 | 能 `ssh 用户名@服务器IP` 登进去 |
| 上游厂商的真实 API key | 在你选用的模型厂商控制台自己申请。**本文档不提供任何 key** |
| 一个密码管理器 | 用来离线保存下面生成的三个密钥；丢了就无法恢复 |

> 没有域名也能装：管理后台通过 SSH 隧道在你自己电脑上打开，最安全。
> 有域名再加 HTTPS，见第 8 节。

---

## 第 1 步：服务器安装 Docker

SSH 登录服务器后执行（Ubuntu / Debian）：

```bash
sudo apt update
sudo apt install -y ca-certificates curl openssl git
sudo install -m 0755 -d /etc/apt/keyrings
sudo curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
sudo chmod a+r /etc/apt/keyrings/docker.asc
echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu $(. /etc/os-release && echo "$VERSION_CODENAME") stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
sudo apt update
sudo apt install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
sudo usermod -aG docker $USER
```

**退出 SSH 重新登录一次**（让 `docker` 权限生效），然后验证：

```bash
docker --version
docker compose version
```

两条都打印出版本号就成功了。其他 Linux 发行版请用 Docker 官方文档的命令替代这一节。

---

## 第 2 步：把项目传到服务器

**在你自己的电脑上**（不是服务器）执行。先进入项目的上一级目录：

```bash
cd /Users/Evan/WorkBuddy/spaces/2026_11_20_17-10-14
tar czf aiporto.tar.gz \
  --exclude='frontend/node_modules' \
  --exclude='frontend/dist' \
  --exclude='.toolcache' \
  --exclude='.go-toolchain' \
  --exclude='.go-path' \
  --exclude='.go-cache' \
  aiporto
scp aiporto.tar.gz 用户名@服务器IP:/www/docker/
```

**然后在服务器上**：

```bash
mkdir -p /www/docker && cd /www/docker
tar xzf aiporto.tar.gz
cd aiporto
ls
```

能看到 `docker-compose.prod.yaml`、`migrations/`、`frontend/` 等就对了。

> 以后更新代码：重复第 2 步上传覆盖，再执行第 9 节的升级命令。**不要删数据卷。**

---

## 第 3 步：生成三个密钥（只有你能看到）

在服务器上执行，把每一行的输出**复制保存到密码管理器**（这三把钥匙丢了无法找回）：

```bash
echo "① API_KEY_PEPPER_B64:"; openssl rand -base64 32
echo "② UPSTREAM_MASTER_KEY_B64:"; openssl rand -base64 32
echo "③ ADMIN_TOTP_SECRET:"; openssl rand -base64 20 | tr -d '\n' | base32 | tr -d '='
```

三把钥匙的作用，**绝对不要复用同一个值**：

| 输出 | 用在哪 | 丢了会怎样 |
| --- | --- | --- |
| ① | 客户 key 的哈希盐 | 全部客户 key 立即失效，需重新签发 |
| ② | 上游 key 的加密主密钥 | 全部上游 key 无法解密，需重新录入 |
| ③ | 管理后台第二步验证 | 所有人无法登录后台，需重新生成 |

---

## 第 4 步：填写配置文件

```bash
cd /www/docker/aiporto
cp .env.example .env
chmod 600 .env
nano .env
```

按下面的表逐项改。**模板里带 `REPLACE_` 的一律要换掉；不要照抄示例值；值不要加引号；不要在行尾写 `#` 注释。**

| 配置项 | 你填什么 |
| --- | --- |
| `POSTGRES_DB` | 数据库名，你自己起一个，例如 `gwdb` |
| `POSTGRES_USER` | 数据库用户名，例如 `gwuser` |
| `POSTGRES_PASSWORD` | 数据库密码，自己想一个长的 |
| `DATABASE_URL` | **容器部署不用改**， Compose 会自动覆盖它 |
| `REDIS_ADDR` / `REDIS_PASSWORD` / `REDIS_DB` | 保持默认即可 |
| `API_KEY_PEPPER_B64` | 第 3 步的 ① |
| `UPSTREAM_MASTER_KEY_B64` | 第 3 步的 ② |
| `ADMIN_USER` | 管理后台账号，你自己定 |
| `ADMIN_PASSWORD` | 管理后台密码，你自己定 |
| `ADMIN_TOTP_SECRET` | 第 3 步的 ③ |
| `ALERT_LOW_BALANCE_THRESHOLD` | 余额低于多少告警，与充值金额同单位；不填默认 100 |
| 其余 | 保持默认 |

`nano` 里改完按 `Ctrl+O` 回车保存，`Ctrl+X` 退出。

---

## 第 5 步：一条命令启动全套服务

```bash
docker compose -f docker-compose.prod.yaml up -d --build
docker compose -f docker-compose.prod.yaml ps
```

第一次构建镜像会慢几分钟。`ps` 输出里 `STATUS` 一列**全部显示 `healthy`** 才算成功。六个服务分别是：

| 服务 | 作用 | 端口 |
| --- | --- | --- |
| `gateway` | API 网关（`/v1/*`、`/api/*`） | 8080 |
| `postgres` | 数据库 | 内部 |
| `redis` | 限流与缓存 | 内部 |
| `admin-ui` | 两个界面的网页 | 8081 |
| `prometheus` | 监控 | 9090（仅内网） |
| `node-exporter` | 机器指标 | 内部 |

如果某个服务没 healthy，看日志：

```bash
docker compose -f docker-compose.prod.yaml logs --tail=100 服务名
```

健康检查命令（三个都返回 `ok` 或指标文本才算好）：

```bash
curl -fsS http://127.0.0.1:8080/healthz
curl -fsS http://127.0.0.1:8080/readyz
curl -fsS http://127.0.0.1:8080/metrics | head
```

---

## 第 6 步：录入你自己的业务数据

系统里**没有预置任何假账号、假 key 或假租户**，全部由你填。

### 6.1 先写入真实的厂商 API key（命令行，只做一次）

```bash
cd /www/docker/aiporto
read -rs SEED_UPSTREAM_KEY_PLAIN && export SEED_UPSTREAM_KEY_PLAIN
docker compose -f docker-compose.prod.yaml run --rm --entrypoint seed \
  -e SEED_UPSTREAM_KEY_PLAIN gateway \
  -tenant-name "你的公司名" \
  -provider-code "厂商代号" \
  -provider-base-url "厂商域名" \
  -model-alias "你给客户的模型别名" \
  -model "厂商真实模型名"
unset SEED_UPSTREAM_KEY_PLAIN
```

填法说明：

- `厂商代号`：自己起，如 `deepseek`、`bailian`，只用英文小写。
- `厂商域名`：只到域名，**不要带 `/v1`**，例如 `https://api.厂商域名.com`。具体地址以厂商官方文档为准。
- `你给客户的模型别名`：客户调用时填的名字，如 `gpt-pro`。
- `厂商真实模型名`：网关会改写成这个名字再转发。

屏幕上会出现一行 `CUSTOMER_KEY_PLAINTEXT=sk-wb-...`，**这是客户 key 唯一的明文展示机会，立刻复制保存**。数据库里只有哈希，之后再也看不到。

### 6.2 打开管理后台，建套餐、建租户

**最安全的做法：在你自己电脑上开 SSH 隧道，不要在公网开放 8081。**

```bash
ssh -L 8081:127.0.0.1:8081 用户名@服务器IP
```

保持这个终端不关，然后在你自己电脑的浏览器打开：

```
http://127.0.0.1:8081/admin
```

登录分两步：

1. 第 4 步填的 `ADMIN_USER` 和 `ADMIN_PASSWORD`
2. 打开验证器 App（Aegis / FreeOTP 等开源客户端），用第 3 步的 ③ 私钥录入账号，填 6 位动态码

进去以后按这个顺序填：

| 顺序 | 页面 | 填什么 |
| --- | --- | --- |
| 1 | 套餐与价格 | 先建一个套餐，记住它的 code；再录入模型价格，**模型名必须和上面 `-model` 完全一致**，否则费用显示 0 |
| 2 | 租户与 key | 创建租户，套餐 code 填刚建的；会同时签发出客户 key 和 `cw-` 开头的控制台令牌，都只显示一次，立刻保存 |
| 3 | 租户与 key → 设置额度 | 设置每分钟请求数、并发数、单日 token 上限 |
| 4 | 账本流水 → 手动充值 | 按真实收款填写，流水号自己定（重复流水号不会重复入账） |

### 6.3 客户控制台

同样通过隧道打开：

```
http://127.0.0.1:8081/console
```

在页面顶部填入第 6.2 步签发的 `cw-` 控制台令牌。客户只能看到自己租户的数据。

### 6.4 验证网关能正常转发

```bash
curl -N -X POST http://127.0.0.1:8080/v1/chat/completions \
  -H "Authorization: Bearer 刚才保存的sk-wb-客户key" \
  -H "Content-Type: application/json" \
  -d '{"model":"你给客户的模型别名","stream":true,"max_tokens":64,"messages":[{"role":"user","content":"你好"}]}'
```

`-N` 会让你看到文字一段一段蹦出来（SSE 逐块透传），而不是等全部生成完一次性出现。这就对了。

---

## 第 7 步：设置每日自动备份

```bash
crontab -e
```

在打开的文件末尾加一行（路径按你实际安装目录改）：

```
0 3 * * * cd /www/docker/aiporto && ./scripts/backup.sh >> /var/log/wb-backup.log 2>&1
```

保存退出。之后每天凌晨 3 点自动备份，保留 30 天。恢复演练的做法见 `BACKUP-RESTORE.md`——**至少在上线前演练一次**。

---

## 第 8 步（可选）：域名与 HTTPS

只在你需要让客户从公网访问控制台时做。用开源 Caddy（自动申请免费证书）：

```bash
sudo apt install -y caddy
sudo nano /etc/caddy/Caddyfile
```

填入：

```
admin.你的域名 {
    reverse_proxy 127.0.0.1:8081
}
gw.你的域名 {
    reverse_proxy 127.0.0.1:8080
}
```

然后：

```bash
sudo systemctl restart caddy
```

域名解析指向服务器、80/443 端口放行后，Caddy 会自动配好 HTTPS。管理后台仍建议加 IP 白名单或继续走隧道。

**防火墙**：只开放 SSH、80、443。不要直接把 8080 / 8081 / 9090 暴露到公网。

---

## 第 9 步：以后怎么升级 / 停止

```bash
cd /www/docker/aiporto

# 上传新版本覆盖后，重建并启动（数据卷保留）
docker compose -f docker-compose.prod.yaml up -d --build

# 只看状态 / 看日志
docker compose -f docker-compose.prod.yaml ps
docker compose -f docker-compose.prod.yaml logs -f gateway

# 停止服务（数据还在）
docker compose -f docker-compose.prod.yaml down
```

**永远不要加 `-v`**，那会删掉数据库卷。删除前先跑一次 `scripts/backup.sh`。

如果你是在旧版本（只有 MVP）的数据卷上升级，需要手动补三个迁移，命令见 `DEPLOYMENT.md` 第 6 节。

---

## 上线前必须确认的 5 件事

完整的 20 条在 `ACCEPTANCE.md`。最低限度先确认这五条：

```bash
# 1. 三个健康检查都正常
curl -fsS http://127.0.0.1:8080/healthz && curl -fsS http://127.0.0.1:8080/readyz

# 2. 脱敏自检六项全 PASS（在项目目录执行）
./scripts/check-redaction.sh

# 3. 三把密钥都填了、且不是占位符
grep -c 'REPLACE_' .env    # 输出必须是 0

# 4. 备份脚本能跑通
./scripts/backup.sh && ls -lh backups/ | tail -3

# 5. 未登录访问后台业务接口必须是 401
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8080/api/admin/tenants
```

第 5 条输出 `401` 才对；输出 `200` 说明双因素没生效，**立刻停止上线**。

---

## 出问题了先看这里

| 现象 | 原因 / 怎么办 |
| --- | --- |
| `docker compose ps` 某服务起不来 | `docker compose logs --tail=100 服务名`；最常见是 `.env` 没填或填了带引号的值 |
| 后台接口返回 503 | `ADMIN_USER` / `ADMIN_PASSWORD` 没填 |
| 第一步登录过了，第二步一直 401 | 验证器时间和标准时间差太多；或 `.env` 里的 TOTP 私钥与 App 里录入的不一致 |
| 客户请求返回 401 | 客户 key 复制错了；或换过 `API_KEY_PEPPER_B64`（换了就得重新签发所有 key） |
| 客户请求返回 429 | 触发了 RPM / 并发 / 日 token 限制，看响应头的 `Retry-After`；去后台调整额度 |
| SSE 一次性返回全部内容 | 中间有代理缓冲了。确认用了 `curl -N`；若经过 nginx，确认 `proxy_buffering off` |
| 费用一直显示 0 | 后台「套餐与价格」没录模型价格，或价格里的模型名和真实模型名不一致 |
| 升级后报 `relation ... does not exist` | 老数据卷没补迁移，执行 `DEPLOYMENT.md` 第 6 节 |
| 客户 key 明文找不到了 | 无法找回，只能在后台重新签发一把 |

---

## 当前验证状态（如实说明）

这部分不做美化，是什么就是什么：

- **已通过**：Go 的 `build` / `vet` / 全部单元测试；前端 `npm run build`；脱敏自检六项全 PASS。
- **未在本环境验证**：Docker Compose 启动、真实 Postgres/Redis 联调、真实厂商 API 调用、200 并发压测。原因是本机没有 Docker，且你还没有填真实配置和上游 key。
- 你按本手册装完、跑通第 6.4 步的 curl 之后，才算真正端到端验证通过。压测按 `LOADTEST.md` 做，结果填进 `ACCEPTANCE.md` 第 20 条。
