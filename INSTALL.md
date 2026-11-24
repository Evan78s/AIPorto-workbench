# Aiporto Workbench 安装（两种方式）

> Aiporto｜智港　·　万模入港，统一调度

这份文档是总入口。两种方式的**前提条件一样**，**装完后的配置一样**，只有"把代码弄到服务器上"这一步不同。

## 先决条件（两种方式都要）

- 一台 Linux 服务器，建议 2 核 4G 以上，系统 Ubuntu 22.04 / Debian 12 / CentOS 7+
- 一个 GitHub **私有仓库**（代码里有计费和租户隔离逻辑，不能公开）
- 一个域名（可选，但强烈建议；没有域名就只能通过 SSH 隧道访问后台）
- 你自己的厂商 API key（DeepSeek / 阿里百炼 / 火山方舟等），**本文档不提供任何 key**

---

## 准备：把项目首次发布到 GitHub（只做一次）

在你自己电脑上，项目目录里执行。先去 GitHub 网页建一个**空仓库**，仓库地址 `https://github.com/Evan78s/AIPorto-workbench.git`，**不要勾选** README、LICENSE、.gitignore（否则首次推送会冲突）。

```bash
cd aiport
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

## 方式一：服务器终端一键安装

适合：会用 SSH 登录服务器、愿意敲几行命令。更新最省事，`git pull` 一行搞定。

**整段复制到服务器终端执行**（把尖括号内容换成你自己的）：

```bash
git clone https://github.com/Evan78s/AIPorto-workbench.git /opt/aiport && cd /opt/aiport && bash install.sh
```

这三件事 `install.sh` 会自动做完：

1. 检查并安装 Docker（含 Compose 插件）
2. 首次运行时从模板生成 `.env`，自动生成数据库密码、key  pepper、上游加密主密钥、TOTP 私钥（**只显示一次，立刻存进密码管理器**）
3. 启动全部容器，并等到网关 `healthy`

脚本**不会**替你做两件事：不生成客户 key（签发时明文只出现一次），也不替你决定后台管理员账号密码（需要你自己填）。

命令跑完后，按"装完必做的四件事"继续。

---

## 方式二：下载上传到服务器，用网页（宝塔）安装

适合：不想敲 git 命令，习惯宝塔面板点来点去。更新时要重新下载上传，比方式一麻烦一点。

**第 1 步 · 下载压缩包**

浏览器登录 GitHub → 打开你的 `AIPorto-workbench` 私有仓库 → 点绿色 `Code` → `Download ZIP`。得到一个 `AIPorto-workbench-main.zip`。

**第 2 步 · 上传到服务器**

宝塔面板 → 「文件」→ 进入 `/opt` → 点「上传」→ 选中刚下载的 zip。

**第 3 步 · 解压并改名**

右键 zip → 「解压」。解压出来的文件夹通常叫 `AIPorto-workbench-main`，把它重命名为 `aiport`，最终路径必须是 `/opt/aiport`。

**第 4 步 · 一键起服务**

宝塔 → 「终端」，执行：

```bash
cd /opt/aiport && bash install.sh
```

脚本不依赖 git，方式二照样能用。它会自动装 Docker、生成 `.env`、起容器。

**第 5 步 · 建站点、配域名（网页操作）**

宝塔 → 「网站」→ 建两个**纯静态站点**，都不要建数据库：

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
nano /opt/aiport/.env
```

找到 `ADMIN_USER=` 和 `ADMIN_PASSWORD=`，填你自己的值（脚本故意留空）。填完重启网关：

```bash
cd /opt/aiport && docker compose -f docker-compose.prod.yaml restart gateway
```

**2. 把 TOTP 私钥录进验证器**

安装开源验证器 App（Aegis / FreeOTP），手动录入安装时打印的 TOTP 私钥，得到 6 位动态码。登录后台时要密码 + 动态码两步。

**3. 写入你自己的厂商 key**

```bash
cd /opt/aiport
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
cd /opt/aiport
docker compose -f docker-compose.prod.yaml ps          # 所有服务都是 healthy
curl http://127.0.0.1:8080/healthz                     # 返回 ok
```

浏览器打开 `https://admin.你的域名`，用第 1、2 步的账号密码 + 动态码登录。能进去就通了。

---

## 两种方式怎么选

| | 方式一：终端 git | 方式二：下载 ZIP |
|---|---|---|
| 首次安装 | 一条命令 | 下载→上传→解压→一条命令 |
| 日常更新 | `git pull` + 重建容器 | 重新下载上传，再重建容器 |
| 回滚版本 | `git checkout <版本号>` | 靠自己留旧压缩包 |
| 适合谁 | 能 SSH 敲命令 | 只想在宝塔网页操作 |

**建议用方式一。** 方式二首次上手更直观，但后面每次更新都要手动传一遍，容易漏文件。

更新步骤统一看 `GITHUB-UPDATE.md`。特别注意：数据库迁移 SQL 只在第一次初始化时空数据卷自动执行，老数据卷要手动补，文档里有现成命令。

## 详细文档索引

- `GITHUB-UPDATE.md` — 首次发布、日常更新、迁移与回滚
- `SERVER-INSTALL.md` — 纯命令行安装全过程
- `BAOTA-INSTALL.md` / `宝塔面板安装手册.docx` — 宝塔网页版全过程
- `CONFIG-CHECKLIST.md` — 所有需要你自己填的配置项
- `ACCEPTANCE.md` — 上线前 20 条验收清单
