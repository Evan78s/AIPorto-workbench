# 上线前验收清单（20 条，逐条勾选）

> 用法：逐项实测后把 `[ ]` 改成 `[x]`，并在旁边记录日期与执行人。任何一条未通过，不得上线。

## 配置与密钥

- [ ] **1.** `.env` 已填成自己的值，且 `.env` 未被提交到 git（`git status` 看不到它）。
- [ ] **2.** `API_KEY_PEPPER_B64`、`UPSTREAM_MASTER_KEY_B64` 各自独立生成、均为 32 字节 base64，且已离线备份。
- [ ] **3.** `ADMIN_USER`、`ADMIN_PASSWORD`、`ADMIN_TOTP_SECRET` 已填；验证器 App 已录入同一 TOTP 私钥。
- [ ] **4.** 数据库中搜索上游 key 明文无结果：`docker compose -f docker-compose.prod.yaml exec -T postgres psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "SELECT count(*) FROM upstream_keys WHERE convert_from(encrypted_key,'UTF8') LIKE '%你的明文片段%';"` 应为 0。

## 红线一：上游 key 明文

- [ ] **5.** 任意管理接口响应中搜不到上游 key 明文或 `encrypted_key` 字段（用浏览器 DevTools 的 Network 面板逐个看）。
- [ ] **6.** `./scripts/check-redaction.sh` 六项全 PASS（先 `mkdir -p .toolcache`）。
- [ ] **7.** 容器日志中搜不到上游 key 明文与 `Authorization` 头：`docker compose -f docker-compose.prod.yaml logs | grep -c 'Bearer '` 应为 0。
- [ ] **8.** 前端源码中搜不到任何 `sk-wb-` 开头的真实 key（`grep -R 'sk-wb-[a-f0-9]\{8\}' frontend/src/` 无输出）。

## 红线二：usage_logs 不存 prompt

- [ ] **9.** `\d usage_logs` 中没有 prompt / messages / body 类正文列。
- [ ] **10.** 随机抽查 10 条 `usage_logs`，只有 token 数与元数据，无对话内容。
- [ ] **11.** 下载 `usage.csv` 与 `bill.csv`，表头与字段白名单一致，不含 prompt、key、tenant_id（`go test ./internal/consoleapi -run CSV` 通过）。

## 红线三：写操作审计

- [ ] **12.** 依次执行创建租户、签发 key、改额度、充值、停用 key，然后查 `audit_logs`，五步各有一条记录，actor 为管理员用户名。
- [ ] **13.** 其中任一写的 `detail` 字段不含一次性明文 key、密码或 Authorization（可用 `go test ./internal/consoleapi -run Audit` 复核）。
- [ ] **14.** 故意用错误会话发一次 POST，仍能在 `audit_logs` 看到 status_code=401 的记录。

## 红线四：双因素登录

- [ ] **15.** 不带会话访问 `/api/admin/tenants` 返回 401；只完成第一步（账号密码）仍返回 401。
- [ ] **16.** 错误 TOTP 码返回 401；同一个正确码 30 秒内第二次提交返回 409。
- [ ] **17.** 完成两步后能正常访问后台页面；前端会话在 `sessionStorage`，关闭标签页后需重新登录；代码中已无 Basic Auth。

## 租户隔离、健康与运维

- [ ] **18.** 用 A 租户控制台令牌请求 `/api/console/keys` 与导出 CSV，只返回 A 的数据；手工把请求里的 `tenant_id` 改成 B，结果不变（`go test ./internal/consoleapi` 全绿）。
- [ ] **19.** `curl /healthz`、`/readyz`、`/metrics` 均 200；`docker compose ps` 所有服务 healthy；`promtool check rules` 通过；备份脚本跑通且演练恢复行数一致。
- [ ] **20.** 按 `LOADTEST.md` 跑 200 并发 SSE，errors=0，端到端 p95 差值 < 100ms，并记录到结果模板。

---

**验收人：** ____________  **日期：** ____________  **结论：** ☐ 通过 ☐ 不通过（原因：__________）
