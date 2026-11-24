# 压测方案：单实例 200 并发 SSE，p95 额外延迟 < 100ms

## 1. 口径定义

**额外延迟 = 同一台机器、同样 200 并发、同样 chunk 参数下，经网关的端到端 p95 − 直连 mock 上游的端到端 p95。**

- 端到端：从发出请求到读完整个响应体。
- TTFB：收到第一个字节的时间，反映首 token 前的开销。
- mock 上游不带鉴权、不查库，代表"上游理想基线"。这样算出的差值才是网关自己加的开销。

## 2. 准备

1. 启动本地 mock（纯 Go，无外部依赖）：

   ```bash
   go run ./scripts/mock-upstream -addr :18080 -chunks 20 -delay 20ms
   ```

2. 用你自己创建的一个测试 key，把它的 rpm/concurrency 调到足够大（例如 10000 / 500），否则 200 并发会被限流，测出来的是 429 不是延迟。代码不会生成假 key；脚本只要求 `WB_TEST_API_KEY` 非空。

3. 让网关的某个模型别名指向 mock：provider 的 `-provider-base-url` 填 `http://127.0.0.1:18080`（不要带 `/v1`，客户端路径会原样拼接）。

4. 预热：先发几十个请求，避免首次解析路由/DB 连接建立计入冷启动。

## 3. 跑两轮

```bash
# A. 基线：直连 mock（mock 不校验 key，但脚本要求环境变量非空）
WB_TEST_API_KEY=<你自己的测试key> \
  go run ./scripts/loadtest -target http://127.0.0.1:18080 -n 200

# B. 经网关
WB_TEST_API_KEY=<你自己的测试key> \
  go run ./scripts/loadtest -target http://127.0.0.1:8080 -n 200 -model <你的模型别名>
```

每轮跑 3 次取 p95 的中位数，避免单次抖动。输出示例格式：

```
target=... requests=200 ok=200 errors=0 wall=1.2s
end_to_end  p50=...ms p95=...ms p99=...ms
ttfb        p50=...ms p95=...ms p99=...ms
```

## 4. 判定标准

| 项 | 通过标准 |
| --- | --- |
| 错误数 | 0（无 401/429/502、无读流错误） |
| 端到端 p95 差值 | **< 100ms**（本轮目标） |
| TTFB p95 差值 | < 50ms（更严格的可选目标） |
| p99 | 记录留档，不作为硬门槛 |

## 5. 不达标时按顺序排查

1. `errors` 非 0：先看是不是 401（key 错）或 429（额度没调大）。
2. 差值大但错误为 0：查 `/metrics` 的 `wb_quota_errors_total` 是否增长（Redis 远了或慢）。
3. 查 `docker compose logs gateway | grep 'metering flush failed'`：批量落库失败虽不阻塞请求，但持续失败说明数据库有问题。
4. 用 `curl -N` 确认 SSE 是逐块到达；如果一次性返回，检查 nginx `proxy_buffering off` 与网关的逐块 Flush。
5. 查容器 CPU/内存与 Go 进程 goroutine 数；200 个长连接对单实例 Go 通常余量充足。
6. 确认压测客户端和网关在同一台机器、走回环地址，别把网络延迟算进网关开销。

## 6. 结果记录模板

```
日期：
机器配置（CPU/内存/系统）：
代码版本 / 镜像 tag：
mock 参数：chunks=20 delay=20ms
基线 p95（三次中位数）：
网关 p95（三次中位数）：
额外延迟：
TTFB 差值：
结论：通过 / 不通过
```
