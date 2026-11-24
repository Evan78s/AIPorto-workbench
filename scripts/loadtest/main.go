// 并发流式压测：记录 TTFB 与端到端耗时，计算 p50/p95/p99。
// 环境变量 WB_TEST_API_KEY 由使用者提供，代码里不内置任何 key。
// 用法：WB_TEST_API_KEY=... go run ./scripts/loadtest -target http://127.0.0.1:8080 -n 200
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(p/100*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func main() {
	target := flag.String("target", "http://127.0.0.1:8080", "gateway or mock base URL")
	n := flag.Int("n", 200, "concurrent requests")
	model := flag.String("model", "demo-model", "model alias")
	timeout := flag.Duration("timeout", 90*time.Second, "per-request timeout")
	flag.Parse()

	key := os.Getenv("WB_TEST_API_KEY")
	if key == "" {
		log.Fatal("WB_TEST_API_KEY is required (use your own test key)")
	}

	client := &http.Client{Timeout: *timeout}
	latency := make([]time.Duration, *n)
	ttfb := make([]time.Duration, *n)
	var errors atomic.Int64
	var wg sync.WaitGroup

	startedAll := time.Now()
	for i := 0; i < *n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			body := fmt.Sprintf(`{"model":%q,"stream":true,"messages":[{"role":"user","content":"hi"}]}`, *model)
			req, err := http.NewRequest(http.MethodPost, strings.TrimRight(*target, "/")+"/v1/chat/completions", strings.NewReader(body))
			if err != nil {
				errors.Add(1)
				return
			}
			req.Header.Set("Authorization", "Bearer "+key)
			req.Header.Set("Content-Type", "application/json")

			start := time.Now()
			resp, err := client.Do(req)
			if err != nil {
				errors.Add(1)
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				errors.Add(1)
				return
			}
			buf := make([]byte, 4096)
			first := true
			for {
				nr, readErr := resp.Body.Read(buf)
				if nr > 0 && first {
					ttfb[i] = time.Since(start)
					first = false
				}
				if readErr != nil {
					if readErr != io.EOF {
						errors.Add(1)
					}
					break
				}
			}
			latency[i] = time.Since(start)
		}(i)
	}
	wg.Wait()

	lat := make([]time.Duration, 0, *n)
	ttf := make([]time.Duration, 0, *n)
	for i := 0; i < *n; i++ {
		if latency[i] > 0 {
			lat = append(lat, latency[i])
			ttf = append(ttf, ttfb[i])
		}
	}
	sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
	sort.Slice(ttf, func(i, j int) bool { return ttf[i] < ttf[j] })

	fmt.Printf("target=%s requests=%d ok=%d errors=%d wall=%s\n",
		*target, *n, len(lat), errors.Load(), time.Since(startedAll).Round(time.Millisecond))
	fmt.Printf("end_to_end  p50=%s p95=%s p99=%s\n",
		percentile(lat, 50), percentile(lat, 95), percentile(lat, 99))
	fmt.Printf("ttfb        p50=%s p95=%s p99=%s\n",
		percentile(ttf, 50), percentile(ttf, 95), percentile(ttf, 99))
}
