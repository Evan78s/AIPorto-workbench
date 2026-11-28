# 用 GitHub 安装与更新（一页纸）

## 结论

值得。安装从「打包整个文件夹传到服务器」变成一行 `git clone`；更新从「再传一次、人工比对哪些文件变了」变成一行 `git pull`，还能打版本号、出问题退回去。

**必须建私有仓库（Private）。** 这套代码里有计费和租户隔离逻辑，不要放公开仓库。

## 首次安装

在服务器上装 git，然后把项目拉下来：

```bash
sudo apt update && sudo apt install -y git
mkdir -p /www/docker && cd /www/docker
git clone https://github.com/Evan78s/AIPorto-workbench.git aiporto
cd aiporto
```

`.env` 不会跟仓库下来（被 `.gitignore` 挡住了）。在服务器上从模板复制一份，再按 `CONFIG-CHECKLIST.md` 填真实值：

```bash
cp .env.example .env
nano .env
```

后面的步骤照 `SERVER-INSTALL.md` 或 `BAOTA-INSTALL.md` 继续——装 Docker、起容器、配域名、录厂商 key，这些一点没变。

## 日常更新（推荐：一条命令）

```bash
cd /www/docker/aiporto && bash scripts/update.sh
```

这一条命令自动完成全部流程，并且每一步失败都会停下来处理：

1. 检查前置条件（有未提交改动会拒绝升级，避免覆盖你的修改）
2. **备份数据库并校验完整性**（备份失败直接中止，不带病升级）
3. `git pull` 拉新代码
4. **幂等补跑全部数据库迁移**（解决"已有库不会自动跑新迁移"这个最大的坑）
5. 重建 `gateway / admin-ui / user-ui` 容器（`--force-recreate` 确保 `.env` 变更生效）
6. 健康检查（网关 `/healthz` + 容器状态）
7. 成功打印新版本号；任何一步失败自动回滚代码到旧版本

升级日志在 `backups/update.log`，成功时会打印回滚命令。也可以登录后台「系统更新」页查看当前版本号、复制更新命令——页面本身不执行任何操作。

> **注意**：新配置不会自动进你的 `.env`。每次升级后扫一眼 `.env.example` 有没有新增配置项，有就手动补进 `.env`，然后 `docker compose -f docker-compose.prod.yaml up -d --force-recreate gateway` 让新配置生效。

<details>
<summary>手动更新步骤（不用脚本时参考）</summary>

```bash
cd /www/docker/aiporto
bash scripts/backup.sh                 # 1. 备份，必做
git pull                               # 2. 拉新代码
bash scripts/migrate-all.sh            # 3. 幂等补跑全部迁移
docker compose -f docker-compose.prod.yaml \
  up -d --build --force-recreate gateway admin-ui user-ui   # 4. 重建
docker compose -f docker-compose.prod.yaml ps               # 5. 查 healthy
```

迁移 SQL 只在数据库**第一次初始化**时自动跑；已有数据卷不会自动补。`scripts/migrate-all.sh` 会把 `migrations/` 下所有幂等 SQL 重新执行一遍（全部是 `CREATE TABLE IF NOT EXISTS` / `ADD COLUMN IF NOT EXISTS`，重复执行安全，不动已有数据）。

</details>

## 升级到「用户自助注册/购买」版（本次新增）

如果你已经在服务器上装过旧版（只有网关 + 后台 + 控制台），这次升级**不能只重跑首次安装那条 `git clone` 命令**——目录已经存在，clone 会失败；就算强行覆盖，`.env` 和新表也不会自动就位。按下面这套做：

```bash
cd /www/docker/aiporto

# 1. 备份，必做
bash scripts/backup.sh

# 2. 拉新代码（不是重新 clone）
git pull

# 3. 补跑用户端数据库迁移
#    迁移只在数据库首次初始化时空库自动执行；已有数据卷必须手动补一次
bash scripts/migrate-user-portal.sh

# 4. 重建容器（--build 让新增的 user-ui 服务也起来）
docker compose -f /www/docker/aiporto/docker-compose.prod.yaml \
  up -d --build --force-recreate gateway admin-ui user-ui

# 5. 检查全部 healthy
docker compose -f /www/docker/aiporto/docker-compose.prod.yaml ps
```

还要补三件事，否则功能不完整：

1. **`.env` 新增配置**：对照新的 `.env.example` 手动补 `USER_PORTAL_URL`、`PUBLIC_BASE_URL`、`SMTP_HOST/PORT/USER/PASS/FROM/TLS`。SMTP 不配的话，用户注册接口会返回 503。旧 `.env` 不会被 `git pull` 覆盖。
2. **宝塔加一个用户端站点**：域名 `user.你的域名.com`，反向代理目标 `http://127.0.0.1:8082`，申请 SSL。网关和 admin 两个站点不用动。
3. **厂商 key 改到后台填**：进入 `/admin` →「厂商管理」「模型路由」页面直接填，不用再跑 seed 命令；已录的 key 不受影响。

## 出问题怎么退回

```bash
git log --oneline            # 找到上次正常的版本号
git checkout <那个版本号>
docker compose -f docker-compose.prod.yaml up -d --build
```

代码能退，**数据库退不回去**。所以更新前那条备份不是可选项。

## 三条仓库红线

1. 仓库设为 **Private**。
2. `.env` 永远不提交。`.gitignore` 已经挡住了，但每次提交前扫一眼 `git status`，确认里面没有 `.env`、没有 `seed*.txt`。
3. 签发 key 时终端打印的明文，只存进你自己的密码管理器。**不要**贴进 Issue、提交说明或聊天记录。

## 首次发布（在你自己电脑上做一次）

1. 去 GitHub 网页建一个**空私有仓库**，仓库地址 `https://github.com/Evan78s/AIPorto-workbench.git`。**不要勾选** README、LICENSE、.gitignore，否则首次推送会冲突。
2. 在项目目录执行：

```bash
cd aiporto
git remote add origin https://github.com/Evan78s/AIPorto-workbench.git
git add .
git commit -m "Aiporto Workbench 首个版本"
git branch -M main
git push -u origin main
```

3. 提交输出里不应出现 `.env`、`seed*.txt`。如果出现，立刻停止并删掉该文件的跟踪：`git rm --cached <文件名>`。

发布完之后，服务器上的安装与更新方式见 `INSTALL.md`。本机已备好 `.gitignore` 并完成 `git init`，但**没有**配置远程地址、没有提交、也没有推送——推送是外部动作，需要你明确确认。
