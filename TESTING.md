# Aiporto Workbench MVP 自测清单

以下 8 个场景必须全部验证。`<KEY>` 指种子命令打印的 `CUSTOMER_KEY_PLAINTEXT`，`<KEY_ID>` 指 `api_keys.id`，`gw:tok:<KEY_ID>:<YYYYMMDD>` 中的日期为当天本地日期。

---

## 1. 鉴权：无 key / 错误 key / 正确 key，且数据库无明文

**步骤**
1. 不带 Authorization 请求 `/v1/chat/completions`。
2. 带 `Authorization: Bearer sk-wb-wrongwrongwrong` 请求。
3. 用 `<KEY>` 正常请求一次。
4. 在数据库执行：`SELECT key_hash, key_prefix FROM api_keys WHERE key_hash = '<用 openssl 计算的摘要>'`；并全文搜索明文串。

**预期结果**
1. 401，响应体 `{"error":"unauthorized"}`。
2. 401，与第 1 步一致，不提示"key 不存在"。
3. 200（或上游返回的正常状态码）。
4. `key_hash` 为 64 位十六进制；数据库任意表中都搜不到 `<KEY>` 明文。

---

## 2. 模型别名路由到真实模型

**步骤**
1. 种子时设置 `-model-alias demo-model -model deepseek-chat`。
2. 请求 body 用 `"model":"demo-model"`，在上游侧（或 mock 服务）打印收到的 body。
3. 请求 `"model":"unknown-model"`。

**预期结果**
1. 上游收到的 `model` 是 `deepseek-chat`，而不是 `demo-model`。
2. 请求路径保持 `/v1/chat/completions`。
3. `unknown-model` 返回 404，`no route for model`。

---

## 3. SSE 流式逐块透传与 token 统计

**步骤**
1. 用 `curl -N` 发起 `stream:true` 请求。
2. 观察响应到达方式与时延。
3. 等 2 秒后查 `usage_logs` 最近一行。

**预期结果**
1. 终端逐行打印 `data: {...}`，不是等全部生成完一次性出现。
2. 首块 TTFT 明显小于整体耗时；多块之间有可见间隔。
3. `stream = true`；若上游返回 `usage`，`prompt_tokens` / `completion_tokens` 与上游字段一致；若上游未返回，则两列均大于 0（本地估算兜底）。`ttft_ms > 0` 且 `latency_ms >= ttft_ms`。

---

## 4. RPM 限流

**步骤**
1. 种子时 `-rpm 2`。
2. 1 分钟内连续发 3 个请求。

**预期结果**
1. 前两个请求正常。
2. 第三个返回 429，响应体含 `quota exceeded: rpm`。
3. `Retry-After` 是 1–60 之间的整数，且与 Redis `TTL gw:rpm:<KEY_ID>` 接近。

---

## 5. 并发数限流与释放

**步骤**
1. 种子时 `-concurrency 1`。
2. 用两个终端同时发起两个耗时较长的流式请求（可用慢速 mock 上游）。
3. 第一个结束后立刻再发一个请求。

**预期结果**
1. 两个并发请求中只有一个成功，另一个 429，响应体含 `quota exceeded: concurrency`。
2. 429 请求的预扣被回滚：`gw:rpm:<KEY_ID>` 不会重复计入该请求。
3. 第一个请求结束后，新请求可以成功（ZSET 中的 member 已被 `Settle` 移除）。
4. 手动在 Redis `ZADD gw:conc:<KEY_ID> <很久以前的时间戳> stale-id` 模拟残留，新请求不会被这个残留卡住（1 小时自动清理）。

---

## 6. 单日 token 上限与预扣校正

**步骤**
1. 种子时 `-daily-tokens 100`。
2. 发一个 `max_tokens: 4096` 的请求。
3. 观察请求期间和结束后的 Redis 计数。
4. 删掉 `gw:tok:<KEY_ID>:<YYYYMMDD>` 模拟跨日重置后再请求。

**预期结果**
1. 预扣超过 100 时被 429，响应体含 `quota exceeded: tokens`，且 `Retry-After` 指向当天结束前的秒数。
2. 额度充足时：请求进行中 `GET gw:tok:<KEY_ID>:<YYYYMMDD>` 是预扣值（输入估算 + max_tokens）；请求结束后变为 `usage_logs` 中该请求的 `prompt_tokens + completion_tokens`。
3. 预扣失败后并发与 RPM 都被回滚，不留下"占位泄漏"。
4. 删除日 key 后重新可请求，新 key 自动带次日午夜过期时间。

---

## 7. 加权轮询与失败转移（含流式禁止重试）

**步骤**
1. 同一个 provider 下插入两个 `upstream_keys`，权重分别为 2 和 1（可用 seed 命令跑两次，第二次给不同权重）。
2. 连续发 6 个非流式请求，统计两个上游 key 收到的次数。
3. 让权重高（先被选中）的那个上游返回 500，再发一个非流式请求。
4. 让上游在 SSE 输出 2 个 chunk 后中断，再发一个流式请求。

**预期结果**
1. 6 次请求中两个 key 命中次数接近 2:1（例如 4:2），不是平均分配。
2. 首个上游 500 时自动换第二个 key 重试一次并返回成功；上游总请求次数为 2。
3. 流式已输出后中断：客户端收到已输出的内容后连接结束，**上游请求次数只增加 1**，不发起第二次请求。
4. 重试只发生一次：故意让两个上游都返回 500，网关返回 502，上游总请求次数为 2。

---

## 8. 异步批量落库不阻塞请求

**步骤**
1. 用脚本并发发 200 个请求，记录每个请求的耗时。
2. 请求全部返回后立即查 `SELECT count(*) FROM usage_logs`（记录时点 A）。
3. 等 2 秒后再查一次（记录时点 B）。
4. 压测时把 `METERING_BATCH_SIZE` 调小、队列打满，查看退出日志。

**预期结果**
1. 200 个请求都正常返回，P99 不包含数据库批量写耗时；请求耗时与单请求接近。
2. 时点 A 的计数可能小于 200（异步），时点 B 等于 200。
3. `request_id` 无重复，`tenant_id` / `api_key_id` / `provider_id` 均正确填充。
4. 队列打满时请求依然成功，退出日志中的 `dropped usage records` 计数大于 0，且不出现 panic。
