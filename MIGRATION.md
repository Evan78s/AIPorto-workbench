# Aiporto 智港 · 服务器迁移与全量备份

## 一句话结论

**最快捷的正确做法：在旧服务器执行 `bash scripts/snapshot.sh`，得到一个包含全部数据的单文件；新服务器 `git clone` 后执行 `bash scripts/migrate.sh <快照文件>`。**

> 重要：不要直接 `scp` 整个项目目录就以为迁移完了。PostgreSQL 数据存放在 Docker 命名卷 `pgdata` 里，**不在项目目录中**，直接拷目录会丢掉全部业务数据。

---

## 1. 必须带走的东西

| 类别 | 位置 | 丢了会怎样 |
| --- | --- | --- |
| PostgreSQL 业务数据 | Docker 卷 `pgdata` | 租户、用户、套餐、订单、API Key 哈希、用量记录全部丢失 |
| `.env` 中的技术密钥 | 项目根目录 `.env` | `UPSTREAM_MASTER_KEY_B64` 丢失 → 所有厂商 key 无法解密；`API_KEY_PEPPER_B64` 丢失 → 所有客户 key 失效；`ADMIN_TOTP_SECRET` 丢失 → 管理员无法登录 |
| 支付与邮件配置 | 同一个 `.env` | 支付宝/微信私钥、SMTP 授权码丢失 → 支付与注册发信中断 |
| 代码版本 | git commit | 新旧代码与数据库结构不匹配可能导致接口异常 |
| Redis 数据 | Docker 卷 `redisdata` | 仅限流计数与缓存丢失，重启后自动重建，不影响业务数据 |

`snapshot.sh` 会把以上全部打进一个 `.tar.gz`。宝塔的反代配置、SSL 证书、DNS 解析**不在快照里**，需要在新服务器重新配置。

---

## 2. 最快捷的全量备份（旧服务器）

```bash
cd /www/docker/aiporto
bash scripts/snapshot.sh
```

输出类似：

```text
完成：/www/docker/aiporto/snapshots/aiporto-full-20261128_130000.tar.gz
```

脚本做了 6 件事：

1. `pg_dump -Fc` 导出 PostgreSQL 全量数据并用 `pg_restore -l` 校验完整性；
2. 触发 Redis `SAVE` 并导出 RDB；
3. 复制 `.env`、compose 文件、Dockerfile、migrations、前后端源码；
4. 写入 `MANIFEST.txt`（生成时间、git 版本）与 `SHA256SUMS`；
5. 打包为 `aiporto-full-时间戳.tar.gz`，权限 `600`；
6. 自动清理 30 天前的旧快照（`KEEP_DAYS=60` 可改）。

### 异地保存（强烈建议）

同机快照挡不住机器损坏。立刻推到另一台机器或对象存储：

```bash
scp /www/docker/aiporto/snapshots/aiporto-full-20261128_130000.tar.gz \
    root@备份服务器IP:/backup/
```

也可以放进 crontab 每天自动快照 + 同步：

```bash
crontab -e
# 每天 03:00 全量快照，03:30 推送到备份机
0  3 * * * cd /www/docker/aiporto && bash scripts/snapshot.sh >> backups/snapshot.log 2>&1
30 3 * * * rsync -avz /www/docker/aiporto/snapshots/ root@备份服务器IP:/backup/
```

日常轻量备份仍可继续用 `scripts/backup.sh`（只备数据库、保留 30 天）；`snapshot.sh` 用于迁移和灾备。

---

## 3. 迁移到新服务器

### 第 1 步：新服务器装好基础环境

```bash
mkdir -p /www/docker
git clone https://github.com/Evan78s/AIPorto-workbench.git /www/docker/aiporto
cd /www/docker/aiporto
```

> 不需要先跑 `install.sh`：迁移脚本会直接起容器并覆盖 `.env`。如果已经跑过也没关系，旧 `.env` 会自动另存为 `.env.pre-migrate-时间戳`。

### 第 2 步：上传快照

```bash
scp aiporto-full-20261128_130000.tar.gz root@新服务器IP:/www/docker/aiporto/
```

### 第 3 步：一条命令恢复

```bash
cd /www/docker/aiporto
bash scripts/migrate.sh aiporto-full-20261128_130000.tar.gz
```

过程会要求输入 `YES` 确认，然后依次：校验 SHA256 → 保护现有 `.env` → 恢复代码与 `.env` → 恢复 PostgreSQL → 恢复 Redis → 重建全部容器 → 检查 `/healthz` 与 `/readyz`。

### 第 4 步：新服务器补齐外部配置

快照不包含这些，必须在宝塔/网关侧重做：

1. 域名解析：`gw.aiporto.net`、`admin.aiporto.net`、`user.aiporto.net` 指向新服务器 IP；
2. 宝塔新建三个站点并配置反向代理：
   - `admin.aiporto.net` → `127.0.0.1:8081`
   - `gw.aiporto.net` → `127.0.0.1:8080`
   - `user.aiporto.net` → `127.0.0.1:8082`
   - 三个反代配置文件里都要有 `proxy_buffering off;`（SSE 流式必须）
3. 每个站点申请 SSL 证书、强制 HTTPS；
4. 安全组/防火墙只放行 `80/443`，不要暴露 `8080/8081/9090`；
5. 支付平台回调白名单：如果新服务器出口 IP 变了，去支付宝/微信商户平台更新回调域名或 IP 白名单。

### 第 5 步：验证后再切流量

```bash
curl -fsS http://127.0.0.1:8080/healthz
curl -fsS http://127.0.0.1:8080/readyz
bash scripts/diagnose-plans.sh
```

再逐项检查：

- [ ] 管理后台能用原来的 TOTP 验证码登录（验证 `.env` 迁移成功）
- [ ] 厂商管理页能看到原来的厂商和 key 提示（验证主密钥迁移成功）
- [ ] 用户端 `user.aiporto.net` 能看到原来的套餐，且能登录
- [ ] 用一个已有客户 key 调 `/v1/chat/completions`，确认鉴权与计费正常
- [ ] 支付回调地址能正常访问
- [ ] 全部通过后，再把 DNS 切到新服务器；旧服务器先保持运行 1～2 天

---

## 4. 常见坑

**Q：迁移后厂商 key 全部失效？**
A：`.env` 里的 `UPSTREAM_MASTER_KEY_B64` 没带过去。只能用旧 `.env` 恢复，或重新录入全部厂商 key。

**Q：迁移后客户 key 全部报鉴权失败？**
A：`API_KEY_PEPPER_B64` 没带过去。key 只存哈希，pepper 变了无法推导，只能重新签发客户 key。

**Q：迁移后管理员验证码对不上？**
A：`ADMIN_TOTP_SECRET` 没带过去。用旧 `.env` 恢复，或在 Authy/Aegis 里删掉旧条目重新扫码。

**Q：迁移后用户端显示“暂无可售套餐”？**
A：先跑 `bash scripts/diagnose-plans.sh`；通常是数据库没恢复成功，或 `.env` 没重建导致连错库。

**Q：只想备份、不迁移，需要这么麻烦吗？**
A：不需要。日常用 `bash scripts/backup.sh` 即可；只有换服务器、灾难恢复、重大升级前才用 `snapshot.sh`。

---

## 5. 回滚到旧服务器

切换 DNS 后发现问题，把域名解析改回旧服务器 IP 即可。旧服务器在确认新站稳定前**不要关机、不要删数据**。如果新服务器已经产生新订单，需要用新服务器的快照反向执行一次 `migrate.sh`，把新增数据带回旧服务器。
