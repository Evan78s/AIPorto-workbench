# 用 GitHub 安装与更新（一页纸）

## 结论

值得。安装从「打包整个文件夹传到服务器」变成一行 `git clone`；更新从「再传一次、人工比对哪些文件变了」变成一行 `git pull`，还能打版本号、出问题退回去。

**必须建私有仓库（Private）。** 这套代码里有计费和租户隔离逻辑，不要放公开仓库。

## 首次安装

在服务器上装 git，然后把项目拉下来：

```bash
sudo apt update && sudo apt install -y git
cd /opt
git clone https://github.com/Evan78s/AIPorto-workbench.git aiport
cd aiport
```

`.env` 不会跟仓库下来（被 `.gitignore` 挡住了）。在服务器上从模板复制一份，再按 `CONFIG-CHECKLIST.md` 填真实值：

```bash
cp .env.example .env
nano .env
```

后面的步骤照 `SERVER-INSTALL.md` 或 `BAOTA-INSTALL.md` 继续——装 Docker、起容器、配域名、录厂商 key，这些一点没变。

## 日常更新（每次都按这个顺序）

```bash
cd /opt/aiport

# 1. 先备份，必做。数据库改坏了还能救回来。
bash scripts/backup.sh

# 2. 拉新代码
git pull

# 3. 看这次有没有新增数据库迁移文件
git diff --name-only HEAD@{1}..HEAD -- migrations/

# 4. 重建容器
docker compose -f docker-compose.prod.yaml up -d --build

# 5. 确认全部 healthy
docker compose -f docker-compose.prod.yaml ps
```

**第 3 步是这个项目最容易翻车的地方。** 迁移 SQL 只在数据库**第一次初始化**时自动跑；已有数据卷不会自动补。如果第 3 步列出了新的 `migrations/00X.sql`，手动执行一次：

```bash
docker compose -f docker-compose.prod.yaml cp "migrations/00X.sql" "postgres:/tmp/00X.sql"
docker compose -f docker-compose.prod.yaml exec -T postgres \
  sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -f /tmp/00X.sql'
```

把 `00X.sql` 换成实际文件名。看到执行完且没有 `ERROR` 就行。脚本只新增表/列/索引，不动已有数据。

另外每次更新后扫一眼 `.env.example` 有没有新增配置项。新配置不会自动进你的 `.env`，要手动补；旧 `.env` 不会被 `git pull` 覆盖。

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
cd aiport
git remote add origin https://github.com/Evan78s/AIPorto-workbench.git
git add .
git commit -m "Aiporto Workbench 首个版本"
git branch -M main
git push -u origin main
```

3. 提交输出里不应出现 `.env`、`seed*.txt`。如果出现，立刻停止并删掉该文件的跟踪：`git rm --cached <文件名>`。

发布完之后，服务器上的安装与更新方式见 `INSTALL.md`。本机已备好 `.gitignore` 并完成 `git init`，但**没有**配置远程地址、没有提交、也没有推送——推送是外部动作，需要你明确确认。
