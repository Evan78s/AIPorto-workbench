# Aiporto Workbench 安装（两种方式）

> Aiporto｜智港　·　万模入港，统一调度

这份文档是总入口。三种方式的**前提条件一样**，**装完后的配置一样**，只有"把代码弄到服务器上"这一步不同。

## 先回答一个问题：装完之后还依赖 GitHub 吗？

**不依赖。** 装好之后系统在本机自足运行，GitHub 关掉都不影响：

- 网关和前端是从你服务器上的源码编译的容器镜像
- 数据库、Redis 的数据存在服务器本地的 Docker 卷里
- 日常调用、计费、后台访问都不出网去 GitHub

GitHub 只在两件事上有用：**首次拿代码**、**日后 `git pull` 更新**。如果你用下面的「方式一：离线包」，连首次拿代码都不用 GitHub。

**但要说清楚**：安装时服务器必须能联网，因为要从 Docker Hub 拉 Postgres/Redis 等基础镜像，编译时要拉 Go/npm 依赖。这些不是 GitHub，往阿里云/官方镜像站走。如果你要求「服务器完全无外网」，需要在有网的机器上预导镜像再搬过去——那是另一种做法，本手册默认服务器能上网。

## 先决条件（三种方式都要）

- 一台 Linux 服务器，建议 2 核 4G 以上，系统 Ubuntu 22.04 / Debian 12 / CentOS 7+
- 服务器能访问 Docker Hub（或已配置国内镜像加速）
- 一个域名（可选，但强烈建议；没有域名就只能通过 SSH 隧道访问后台）
- 你自己的厂商 API key（DeepSeek / 阿里百炼 / 火山方舟等），**本文档不提供任何 key**
- GitHub 仓库只在方式二、方式三更新时用到

---

## 准备：把项目首次发布到 GitHub（只做一次）

在你自己电脑上，项目目录里执行。先去 GitHub 网页建一个**空仓库**，仓库地址 `https://github.com/Evan78s/AIPorto-workbench.git`，**不要勾选** README、LICENSE、.gitignore（否则首次推送会冲突）。

```bash
cd aiporto
git remote add origin https://github.com/Evan78s/AIPorto-workbench.git
git add .
git commit -m "Aiporto Workbench 首个版本"
git branch -M main
git push -u origin main
```

提交前扫一眼输出里有没有 `.env`——`.gitignore` 已经挡住了，正常情况下不会出现。

私有仓库在服务器上 `git clone` 需要认证，二选一：
- **令牌方式**（简单）：GitHub → Settings → Developer settings → Personal access tokens → 生成只勾 `repo` 权限的 token。clone 时用户名填 GitHub 用户名，密码填这个 token。
- **SSH 密钥方式**（长期更省事）：服务器生成 SSH key，把公钥加到 GitHub 账号的 SSH keys，clone 用 `git@github.com:Evan78s/AIPorto-workbench.git`。

---

## 方式一：离线包一键安装（完全不用 GitHub，推荐）

适合：就想传一个文件、跑一条命令把整套系统装起来，不想碰 git 和 GitHub。

**第 1 步 · 拿到安装包**

工作区根目录有一个文件：`aiporto-install.sh`（约 176KB，源码已内嵌在里面）。用宝塔「文件」或 `scp` 把它传到服务器的任意目录，比如 `/root/`。

**第 2 步 · 执行一条命令**

```bash
bash /root/aiporto-install.sh
```

就这一条。脚本会自动完成：**解压源码到 `/www/wwwroot/aiporto` → 检查并安装 Docker → 生成 `.env` 和随机技术密钥 → 拉镜像、编译、启动全部容器 → 等待网关 healthy**。全程不访问 GitHub。

**第 3 步 · 按下面的「装完必做的四件事」配置**

填管理员账号、录 TOTP、写厂商 key、建租户——这几步任何方式都省不了，因为只有你知道这些值。

> 如果 `/www/wwwroot/aiporto` 已经有数据，脚本会**主动停止**而不是覆盖，保护你的现有配置和数据。要重装得先自己备份移走旧目录。

> 生成新安装包：在你自己电脑的项目目录跑 `bash scripts/build-offline.sh`。代码改了之后重新生成一次再传服务器。

---

## 方式二：服务器终端 git 安装

适合：会用 SSH 登录服务器、愿意敲几行命令。更新最省事，`git pull` 一行搞定。

**整段复制到服务器终端执行**（把尖括号内容换成你自己的）：

```bash
git clone https://github.com/Evan78s/AIPorto-workbench.git /www/wwwroot/aiporto && cd /www/wwwroot/aiporto && bash install.sh
```

这三件事 `install.sh` 会自动做完：

1. 检查并安装 Docker（含 Compose 插件）
2. 首次运行时从模板生成 `.env`，自动生成数据库密码、key  pepper、上游加密主密钥、TOTP 私钥（**只显示一次，立刻存进密码管理器**）
3. 启动全部容器，并等到网关 `healthy`

脚本**不会**替你做两件事：不生成客户 key（签发时明文只出现一次），也不替你决定后台管理员账号密码（需要你自己填）。

命令跑完后，按"装完必做的四件事"继续。

---

## 方式三：下载上传到服务器，用网页（宝塔）安装

适合：不想敲 git 命令，习惯宝塔面板点来点去。更新时要重新下载上传，比方式一麻烦一点。

**第 1 步 · 下载压缩包**

浏览器登录 GitHub → 打开你的 `AIPorto-workbench` 私有仓库 → 点绿色 `Code` → `Download ZIP`。得到一个 `AIPorto-workbench-main.zip`。

**第 2 步 · 上传到服务器**

宝塔面板 → 「文件」→ 进入 `/www/wwwroot` → 点「上传」→ 选中刚下载的 zip。

**第 3 步 · 解压并改名**

右键 zip → 「解压」。解压出来的文件夹通常叫 `AIPorto-workbench-main`，把它重命名为 `aiporto`，最终路径必须是 `/www/wwwroot/aiporto`。

**第 4 步 · 一键起服务**

宝塔 → 「终端」，执行：

```bash
cd /www/wwwroot/aiporto && bash install.sh
```

脚本不依赖 git，方式二照样能用。它会自动装 Docker、生成 `.env`、起容器。

**第 5 步 · 建站点、配域名（网页操作）**

宝塔 → 「网站」→ 建两个**纯静态站点**，都不要建数据库：

> **⚠️ 项目在 `/www/wwwroot/aiporto/`（网站根目录），里面有含密钥的 `.env`。建站点时根目录一定用宝塔默认值，绝不要指向 `/www/wwwroot/aiporto/`。** 再把项目里 `nginx-deny.conf` 的内容粘贴进两个站点的「配置文件」，防万一。建完用 `https://admin.你的域名/.env` 验证，必须返回 403。详见 `BAOTA-INSTALL.md` 第 6 步。

| 站点域名 | 反向代理目标 | 用途 |
|---|---|---|
| `admin.你的域名` | `http://127.0.0.1:8081` | 运营后台 + 客户控制台 |
| `gw.你的域名` | `http://127.0.0.1:8080` | 模型 API |

两个站点都申请 Let's Encrypt 免费证书。

**`gw` 站点的反向代理配置必须加一行 `proxy_buffering off;`**，否则 SSE 流式会退化成"等全部生成完一次性返回"。完整配置见 `BAOTA-INSTALL.md` 第 8 章。

防火墙只放行 80/443，**不要把 8080 / 8081 / 9090 暴露到公网**。

---

## 装完必做的四件事（两种方式都一样）

**1. 填后台管理员账号密码**

```bash
nano /www/wwwroot/aiporto/.env
```

找到 `ADMIN_USER=` 和 `ADMIN_PASSWORD=`，填你自己的值（脚本故意留空）。填完重启网关：

```bash
cd /www/wwwroot/aiporto && docker compose -f docker-compose.prod.yaml restart gateway
```

**2. 把 TOTP 私钥录进验证器**

安装开源验证器 App（Aegis / FreeOTP），手动录入安装时打印的 TOTP 私钥，得到 6 位动态码。登录后台时要密码 + 动态码两步。

**3. 写入你自己的厂商 key**

```bash
cd /www/wwwroot/aiporto
SEED_UPSTREAM_KEY_PLAIN='你的厂商key' go run ./cmd/seed \
  -tenant-name '<你的租户名>' \
  -provider-code <厂商代号> \
  -provider-base-url <厂商API根地址> \
  -model-alias <客户端用的模型别名> \
  -model <厂商真实模型名>
```

还有四个可选参数：`-rpm`、`-concurrency`、`-daily-tokens`、`-weight`。完整参数表见 `cmd/seed/main.go` 第 25–34 行。**不要**把 key 写进 `.env`、git 或任何文档。

**4. 建套餐、建租户、签发客户 key**

登录管理后台 `/admin` 操作。客户 key 只在签发那一刻明文显示一次，存进密码管理器后再也看不到了。

---

## 装完怎么验证

```bash
cd /www/wwwroot/aiporto
docker compose -f docker-compose.prod.yaml ps          # 所有服务都是 healthy
curl http://127.0.0.1:8080/healthz                     # 返回 ok
```

浏览器打开 `https://admin.你的域名`，用第 1、2 步的账号密码 + 动态码登录。能进去就通了。

---

## 三种方式怎么选

| | 方式一：离线包 | 方式二：终端 git | 方式三：下载 ZIP |
|---|---|---|---|
| 首次安装 | 传一个文件 + 一条命令 | 一条命令 | 下载→上传→解压→一条命令 |
| 依赖 GitHub | ❌ 完全不依赖 | ✅ 依赖 | ✅ 依赖（下载时） |
| 日常更新 | 重新生成并传安装包 | `git pull` + 重建容器 | 重新下载上传，再重建容器 |
| 回滚版本 | 靠自己留旧安装包 | `git checkout <版本号>` | 靠自己留旧压缩包 |
| 适合谁 | 只想一键装好，不在乎更新 | 能 SSH 敲命令，想要好更新 | 只想在宝塔网页操作 |

**建议**：首次安装用方式一最省事；如果以后要频繁更新，用方式二更好维护。方式一和方式二装出来的系统**完全一样**，区别只在于代码怎么送到服务器。

更新步骤统一看 `GITHUB-UPDATE.md`。特别注意：数据库迁移 SQL 只在第一次初始化时空数据卷自动执行，老数据卷要手动补，文档里有现成命令。用离线包更新时，先重新生成安装包、把新包里的 `migrations/` 目录内容同步到服务器，再执行迁移命令。

## 详细文档索引

- `GITHUB-UPDATE.md` — 首次发布、日常更新、迁移与回滚
- `SERVER-INSTALL.md` — 纯命令行安装全过程
- `BAOTA-INSTALL.md` / `宝塔面板安装手册.docx` — 宝塔网页版全过程
- `CONFIG-CHECKLIST.md` — 所有需要你自己填的配置项
- `ACCEPTANCE.md` — 上线前 20 条验收清单
