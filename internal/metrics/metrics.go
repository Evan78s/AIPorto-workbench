// Package metrics 提供零依赖的 Prometheus 文本端点。
// 只暴露聚合计数与直方图，不包含任何请求体、key 或租户数据。
package metrics

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"time"
)

var (
	Requests         atomic.Uint64
	Rejected429      atomic.Uint64
	UpstreamFailures atomic.Uint64
	QuotaErrors      atomic.Uint64
	ActiveStreams    atomic.Int64
	LatencySumMS     atomic.Uint64
	LatencyCount     atomic.Uint64
)

var buckets = []float64{5, 10, 25, 50, 100, 250, 500, 1000, 5000}
var bucketCounts [9]atomic.Uint64

func Observe(d time.Duration) {
	ms := uint64(d.Milliseconds())
	LatencySumMS.Add(ms)
	LatencyCount.Add(1)
	for i, b := range buckets {
		if ms <= uint64(b) {
			bucketCounts[i].Add(1)
			return
		}
	}
}

func Handler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	fmt.Fprintln(w, "# HELP wb_requests_total Gateway requests seen.")
	fmt.Fprintln(w, "# TYPE wb_requests_total counter")
	fmt.Fprintf(w, "wb_requests_total %d\n", Requests.Load())
	fmt.Fprintln(w, "# HELP wb_quota_rejected_total Requests rejected by rate/quota limits.")
	fmt.Fprintln(w, "# TYPE wb_quota_rejected_total counter")
	fmt.Fprintf(w, "wb_quota_rejected_total %d\n", Rejected429.Load())
	fmt.Fprintln(w, "# HELP wb_upstream_failures_total Upstream attempts that failed or were retried.")
	fmt.Fprintln(w, "# TYPE wb_upstream_failures_total counter")
	fmt.Fprintf(w, "wb_upstream_failures_total %d\n", UpstreamFailures.Load())
	fmt.Fprintln(w, "# HELP wb_quota_errors_total Quota service (Redis) errors.")
	fmt.Fprintln(w, "# TYPE wb_quota_errors_total counter")
	fmt.Fprintf(w, "wb_quota_errors_total %d\n", QuotaErrors.Load())
	fmt.Fprintln(w, "# HELP wb_active_streams Currently open streaming responses.")
	fmt.Fprintln(w, "# TYPE wb_active_streams gauge")
	fmt.Fprintf(w, "wb_active_streams %d\n", ActiveStreams.Load())

	fmt.Fprintln(w, "# HELP wb_request_duration_ms End-to-end request latency in milliseconds.")
	fmt.Fprintln(w, "# TYPE wb_request_duration_ms histogram")
	for i, b := range buckets {
		fmt.Fprintf(w, "wb_request_duration_ms_bucket{le=\"%s\"} %d\n",
			fmt.Sprintf("%.0f", b), bucketCounts[i].Load())
	}
	fmt.Fprintf(w, "wb_request_duration_ms_bucket{le=\"+Inf\"} %d\n", LatencyCount.Load())
	fmt.Fprintf(w, "wb_request_duration_ms_sum %d\n", LatencySumMS.Load())
	fmt.Fprintf(w, "wb_request_duration_ms_count %d\n", LatencyCount.Load())
}
