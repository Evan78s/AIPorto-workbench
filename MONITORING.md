# 监控指标清单与告警阈值

## 1. 网关自曝指标（`GET http://127.0.0.1:8080/metrics`）

零依赖实现，Prometheus 文本格式，不含任何请求体、key 或租户信息。

| 指标 | 类型 | 含义 | 建议阈值 | 触发后做什么 |
| --- | --- | --- | --- | --- |
| `wb_requests_total` | counter | 进入网关的请求数 | 仅作分母 | 与 429 率配合看 |
| `wb_quota_rejected_total` | counter | 被 RPM/并发/日 token 拒绝的次数 | 5 分钟增长率 / 总请求率 > 5%，持续 5m | 查租户额度是否被撞穿、是否有异常调用方 |
| `wb_upstream_failures_total` | counter | 上游失败或触发重试的次数 | > 1 次/秒，持续 3m | 查 provider 状态与上游 key 有效性 |
| `wb_quota_errors_total` | counter | Redis 额度服务报错 | > 0，持续 2m | 立即查 Redis；此时请求可能返回 503 |
| `wb_active_streams` | gauge | 当前未结束的流式响应数 | 达到单实例并发上限 80% | 扩容或收紧并发额度 |
| `wb_request_duration_ms` | histogram | 端到端延迟，桶 5/10/25/50/100/250/500/1000/5000ms | p95 > 150ms，持续 5m | 查上游耗时与本机负载 |

查询示例（`http://127.0.0.1:9090`）：

```promql
histogram_quantile(0.95, rate(wb_request_duration_ms_bucket[5m]))
rate(wb_quota_rejected_total[5m]) / clamp_min(rate(wb_requests_total[5m]), 1)
```

## 2. 业务异常告警（运营后台「异常告警」页）

| 告警 | 判定口径 | 阈值 | 数据来源 |
| --- | --- | --- | --- |
| 余额低于阈值 | `v_tenant_balance.balance < 阈值` | 默认 100，按你的收款口径调整 | `v_tenant_balance` |
| 单日用量突增 | 今日 token > 近 7 日有数据日均值的 3 倍 | 3 倍，且均值需 > 1000 才参与判断，避免小样本误报 | `v_usage_cost` |
| 上游 429 激增 | 今日 429 次数 > 昨日同期 3 倍 | 且今日次数 > 10 | `usage_logs.status_code` |

这三条对应 `GET /api/admin/alerts`。要推到企业 IM/邮件时，用 Prometheus 同样口径写 recording rule，或直接定时轮询这个接口，不要新引入付费监控服务。

## 3. 基础设施

| 项 | 采集方式 | 阈值建议 |
| --- | --- | --- |
| 容器存活 | Docker HEALTHCHECK + `up{job="gateway"}` | 不健康持续 1m 告警 |
| CPU / 内存 / 磁盘 | node-exporter | 磁盘 > 80%、内存 > 85% 持续 10m |
| 磁盘写满风险 | 日志已限制 10m × 5；备份目录单独看 | 备份目录剩余空间 < 20GB |
| Postgres / Redis | 网关 `/readyz` + 容器重启次数 | readyz 连续失败即告警 |

## 4. 告警规则文件

`monitoring/alerts.yml` 已内置 5 条规则并由 Prometheus 自动加载。验证语法与加载：

```bash
docker compose -f docker-compose.prod.yaml exec -T prometheus \
  promtool check rules /etc/prometheus/alerts.yml
```

在 Prometheus 页面 `Alerts` 标签能看到规则即生效。阈值需要按你的真实流量调：上线第一周先观察，不要直接拿默认值做寻呼。

## 5. 一条重要边界

`wb_request_duration_ms` 是**端到端**延迟，包含上游模型生成时间。它不能用来直接判断"网关额外延迟"。额外延迟必须按 `LOADTEST.md` 的基线对照法测量。
