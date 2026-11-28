# 备份与恢复（每日备份 / 保留 30 天 / 演练恢复）

> **换服务器或灾难恢复请看 `MIGRATION.md`**：`bash scripts/snapshot.sh` 一键打包数据库 + Redis + `.env` + 代码，新服务器 `bash scripts/migrate.sh <快照文件>` 一键还原。本文下面的 `backup.sh` 只备份数据库，适合日常轻量备份。

## 1. 每日自动备份

`scripts/backup.sh` 做四件事：用 `pg_dump -Fc` 导出 custom format → 用 `pg_restore -l` 校验备份可读 → gzip 压缩 → 删除超过 30 天的旧文件。备份落在项目目录下的 `backups/wb_YYYYMMDD_HHMMSS.dump.gz`。

在宿主机加一条 crontab（时间按你的时区调整）：

```bash
crontab -e
# 每天 02:00 备份，日志追加到 backups/cron.log
0 2 * * * cd /绝对路径/aiporto && ./scripts/backup.sh >> backups/cron.log 2>&1
```

先手工跑一次确认通：

```bash
mkdir -p backups .toolcache
./scripts/backup.sh
ls -lh backups/
```

想改保留天数：`KEEP_DAYS=45 ./scripts/backup.sh`。

## 2. 异地副本（强烈建议，仍用开源工具）

同机备份挡不住机器损坏。用 `rsync` 推到另一台服务器：

```bash
rsync -avz --delete ./backups/ 备份服务器用户@备份服务器:/绝对路径/wb-backups/
```

把这条也放进 crontab，排在备份之后 30 分钟。

## 3. 每月演练恢复（不碰生产库）

`scripts/restore-drill.sh` 会新建临时库 `wb_restore_drill`，把备份恢复进去，打印 tenants / usage_logs / audit_logs 三张表的行数，然后删除临时库。生产库完全不受影响。

```bash
./scripts/restore-drill.sh backups/wb_20261123_020000.dump.gz
```

把打印出的行数与备份当天记录对照；数量明显不符就说明备份有问题。建议每月固定日期演练一次，并把输出留在运维记录里。

## 4. 真正需要恢复时（会覆盖生产数据）

**⚠️ 此操作非常危险，可能导致不可逆的数据丢失！** 执行前必须确认：已经停止写入（或接受最近几分钟数据丢失）、已经有一份当前状态的备份、已经通知相关租户。

```bash
# 1) 先把当前状态再备一份，以防恢复方向选错
./scripts/backup.sh

# 2) 恢复指定备份；脚本会要求手工输入 YES 确认
./scripts/restore.sh backups/wb_20261123_020000.dump.gz

# 3) 立刻验证
curl -fsS http://127.0.0.1:8080/readyz
```

恢复使用 `--clean --if-exists`，会先删后建表；因此**不要在业务高峰执行**。恢复后检查 `audit_logs` 最新记录时间是否符合预期。

## 5. 恢复清单（每次都要过一遍）

- [ ] 确认备份文件完整：`docker compose -f docker-compose.prod.yaml exec -T postgres sh -c 'pg_restore -l' < 备份文件 > /dev/null`
- [ ] 确认已有当前状态的额外备份
- [ ] 确认已通知租户/已停止写入
- [ ] 演练恢复到临时库并核对行数
- [ ] 正式恢复
- [ ] `readyz` 返回 ok
- [ ] 抽查租户列表和最近一小时用量
- [ ] 记录恢复原因、时间点、执行人
