# 宝塔面板安装说明（配合 Docker）

## 先把一件事说清楚

**宝塔能用，但用法和你装普通网站不一样。**

| 你熟悉的普通网站 | 这套系统 |
| --- | --- |
| 建站点 → 把 PHP/HTML 传进网站目录 → 完事 | 建站点只是"给域名指路"，真正跑服务的是 Docker |
| 一个目录就是一个网站 | 四个常驻服务：Go 网关、PostgreSQL、Redis、前端界面 |

所以正确做法是：

- **宝塔负责**：装 Docker、传项目文件、改配置、配域名和免费 SSL 证书、看日志、管防火墙。
- **Docker 负责**：跑网关、数据库、Redis、前端。
- **宝塔的"网站"功能**：只用来把域名反向代理到已经跑起来的服务上，**网站根目录是空的，不要把项目文件传进去**。

如果只把文件丢进 `/www/wwwroot/xxx` 然后点"建网站"，页面可能能打开一个空壳，但网关、数据库、限流全部不会工作。

下面按顺序做。命令行部分都在宝塔的「终端」里执行（面板左侧菜单找「终端」或「SSH 终端」）。

---

## 第 1 步：宝塔里装 Docker

1. 登录宝塔面板。
2. 打开「终端」，执行下面这段（Ubuntu/Debian 系统）：

```bash
sudo apt update
sudo apt install -y ca-certificates curl openssl git
sudo install -m 0755 -d /etc/apt/keyrings
sudo curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
sudo chmod a+r /etc/apt/keyrings/docker.asc
echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu $(. /etc/os-release && echo "$VERSION_CODENAME") stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
sudo apt update
sudo apt install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
docker --version && docker compose version
```

（也可以去宝塔「软件商店」搜 Docker 装可视化插件，但启动服务仍然要在终端敲命令。）

---

## 第 2 步：上传项目

**在你自己电脑上**打包（排除临时文件）：

```bash
cd /Users/Evan/WorkBuddy/spaces/2026_11_20_17-10-14
tar czf aiporto.tar.gz \
  --exclude='frontend/node_modules' --exclude='frontend/dist' \
  --exclude='.toolcache' --exclude='.go-toolchain' \
  --exclude='.go-path' --exclude='.go-cache' \
  aiporto
```

在宝塔里：

1. 左侧「文件」→ 进入 `/www/wwwroot` 目录。
2. 点「上传」，把 `aiporto.tar.gz` 传上去。
3. 右键这个文件 →「解压」，得到 `/www/wwwroot/aiporto.net`。

回到「终端」确认：

```bash
cd /www/wwwroot/aiporto.net && ls
```

能看到 `docker-compose.prod.yaml`、`migrations/`、`frontend/` 就对了。

---

## 第 3 步：生成三个密钥

在宝塔「终端」执行，把三行输出**复制到密码管理器保存**：

```bash
echo "① API_KEY_PEPPER_B64:"; openssl rand -base64 32
echo "② UPSTREAM_MASTER_KEY_B64:"; openssl rand -base64 32
echo "③ ADMIN_TOTP_SECRET:"; openssl rand -base64 20 | tr -d '\n' | base32 | tr -d '='
```

三把钥匙不能复用，丢了无法找回：①丢了客户 key 全失效，②丢了上游 key 全解不开，③丢了后台没人能登录。

---

## 第 4 步：填配置

宝塔「终端」：

```bash
cd /www/wwwroot/aiporto.net
cp .env.example .env
chmod 600 .env
```

然后在宝塔「文件」里找到 `/www/wwwroot/aiporto.net/.env`，双击用编辑器打开，按 `CONFIG-CHECKLIST.md` 的表格填。要点：

- `POSTGRES_DB`、`POSTGRES_USER`、`POSTGRES_PASSWORD`：自己定
- `API_KEY_PEPPER_B64`：填上面的 ①
- `UPSTREAM_MASTER_KEY_B64`：填上面的 ②
- `ADMIN_USER`、`ADMIN_PASSWORD`：自己定
- `ADMIN_TOTP_SECRET`：填上面的 ③
- `DATABASE_URL` 不用改（Docker 里会自动覆盖）
- **值不要加引号，不要在行尾写 `#` 注释**

---

## 第 5 步：启动（关键一步，不能跳过）

在宝塔「终端」执行：

```bash
cd /www/wwwroot/aiporto.net
docker compose -f docker-compose.prod.yaml up -d --build
docker compose -f docker-compose.prod.yaml ps
```

等 `STATUS` 全部是 `healthy`。如果某个服务起不来：

```bash
docker compose -f docker-compose.prod.yaml logs --tail=100 服务名
```

确认服务活着：

```bash
curl -fsS http://127.0.0.1:8080/healthz
curl -fsS http://127.0.0.1:8080/readyz
```

---

## 第 6 步：在宝塔里建站点（只是配域名，不是传文件）

这一步才是你熟悉的"宝塔开网站"，但它只做指路。

> **⚠️ 安全红线（项目现在放在 `/www/wwwroot/aiporto.net/`，是网站根目录）**
>
> 项目目录里有个 `.env`，装着数据库密码、加密主密钥、TOTP 私钥。它绝对不能被浏览器直接下载。
>
> **建站点时，根目录必须用宝塔的默认值（每个域名自己的目录），千万不要指向 `/www/wwwroot/aiporto.net/`。** 站点只做反向代理，不托管项目文件。
>
> 另外给两个站点都加一份兜底防护：宝塔 → 站点 →「配置文件」→ 把项目里 `nginx-deny.conf` 的内容粘贴进 `server { }` 内保存。这份规则会拒绝 `.env`、`*.key`、`migrations/`、`scripts/` 等敏感路径的直接访问，即使站点根目录设错也不会泄露。
>
> 验证：浏览器打开 `https://admin.你的域名/.env`，必须返回 **403**。返回文件内容就说明配置没生效，立刻停掉站点来排查。

### 6.1 管理后台 + 客户控制台

1. 宝塔左侧「网站」→「添加站点」
2. 域名填：`admin.你的域名.com`
3. 根目录用默认值，**不要动，也不要往里面传文件**
4. 数据库选「不创建」，PHP 版本选「纯静态」
5. 确定后，点这个站点 →「反向代理」→「添加反向代理」
   - 代理名称：随便写，如 `gateway-ui`
   - 目标 URL：`http://127.0.0.1:8081`
6. 点「设置」→「SSL」→「Let's Encrypt」→ 申请免费证书 → 打开「强制 HTTPS」

### 6.2 模型调用 API（给客户调用的）

同样再建一个站点 `gw.你的域名.com`，反向代理目标填 `http://127.0.0.1:8080`，也申请 SSL。

⚠️ **这个站点必须关掉缓冲，否则流式输出会退化成一次性返回**：

在反向代理的「配置文件」里加上（或确认有）：

```
proxy_buffering off;
proxy_request_buffering off;
proxy_read_timeout 600s;
```

### 6.3 防火墙

宝塔左侧「安全」→ 放行 80、443。**不要放行 8080、8081、9090 到公网**。管理后台建议只走 6.1 的域名 + HTTPS，或者继续用 SSH 隧道（见 `SERVER-INSTALL.md` 第 6.2 节）。

---

## 第 7 步：录入你自己的数据

系统里没有预置任何假账号或 key。

1. **写入厂商 key**：在宝塔「终端」执行 `SERVER-INSTALL.md` 第 6.1 节的 `seed` 命令，把真实厂商 key 用环境变量传进去。屏幕打印的 `CUSTOMER_KEY_PLAINTEXT=sk-wb-...` 只出现一次，立刻复制。
2. **建套餐和租户**：浏览器打开 `https://admin.你的域名.com/admin`，用第 4 步的账号密码登录，第二步填验证器里的 6 位码。进去后先建套餐和模型价格，再建租户（会签发客户 key 和 `cw-` 控制台令牌）。
3. **客户控制台**：`https://admin.你的域名.com/console`，填 `cw-` 令牌。
4. **验证转发**：`SERVER-INSTALL.md` 第 6.4 节的 curl 命令，把地址换成 `https://gw.你的域名.com/v1/chat/completions`。

---

## 第 8 步：备份和升级

宝塔左侧「计划任务」→「添加任务」：

- 任务类型：`Shell 脚本`
- 名称：每日备份
- 执行周期：每天，凌晨 3 点
- 脚本内容：

```bash
cd /www/wwwroot/aiporto.net && ./scripts/backup.sh >> /var/log/wb-backup.log 2>&1
```

升级时：重新上传新的 tar.gz 覆盖解压，然后在终端执行：

```bash
cd /www/wwwroot/aiporto.net
docker compose -f docker-compose.prod.yaml up -d --build
```

**不要执行 `down -v`**，会删数据库。删除前先跑备份脚本。

---

## 常见问题（宝塔特有）

| 现象 | 怎么办 |
| --- | --- |
| 宝塔里 `docker` 命令不存在 | 第 1 步没执行完，或执行后没重新打开终端；重开一个终端窗口 |
| 站点打开是 502 | 第 5 步的服务没 healthy；先看 `docker compose ps`，再看 nginx 反向代理日志 |
| 后台接口 503 | `.env` 里 `ADMIN_USER` / `ADMIN_PASSWORD` 没填 |
| 第二步登录一直 401 | 验证器时间和标准时间差太多，或 TOTP 私钥和 App 里录的不一致 |
| 流式输出一次性返回 | 6.2 的反代没关缓冲；检查 `proxy_buffering off` 是否加在正确的站点配置里 |
| 费用显示 0 | 后台没录模型价格，或价格里的模型名和真实模型名不一致 |
| 客户 key 找不到了 | 无法找回，只能去后台重新签发 |

---

## 验证结果（本轮实际跑的）

- `go build ./...` → `BUILD_OK`
- `go vet ./...` → `VET_OK`
- `go test ./...` → `TEST_OK`（`consoleapi`、`util` 两个包通过，其余无测试文件）
- `scripts/check-redaction.sh` → 六项全 PASS
- 文档引用检查：手册引用的 14 个文件全部存在；`README.md` 的过时描述已清除

没有在本机验证的部分：宝塔面板本身的操作、Docker 编排启动、真实厂商调用——本机没有宝塔也没有 Docker，且你还没填真实配置。按本手册做完第 7 步的 curl 验证，才算真正跑通。
