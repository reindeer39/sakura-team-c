package server_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	appdb "sakuravel/internal/db"
	"sakuravel/internal/seed"
	"sakuravel/internal/testutil"
)

type endpointStat struct {
	name      string
	method    string
	path      string
	count     int
	errors    int
	min       time.Duration
	max       time.Duration
	avg       time.Duration
	p50       time.Duration
	p95       time.Duration
	durations []time.Duration
}

type parallelResult struct {
	totalRequests int
	successCount  int64
	errorCount    int64
	totalDuration time.Duration
	rps           float64
	minDuration   time.Duration
	maxDuration   time.Duration
	avgDuration   time.Duration
	p50Duration   time.Duration
	p95Duration   time.Duration
	p99Duration   time.Duration
	endpointStats []endpointStat
}

type reqSpec struct {
	name   string
	method string
	path   string
	isAuth bool
	body   []byte
}

type reqResult struct {
	spec    reqSpec
	elapsed time.Duration
	err     bool
}

func runParallelBenchmark(
	t *testing.T,
	ts *httptest.Server,
	authCookie *http.Cookie,
	concurrency int,
	totalRequests int,
) parallelResult {
	specs := []reqSpec{
		{name: "フォロー中TL", method: "GET", path: "/posts?feed=following", isAuth: true},
		{name: "最新TL", method: "GET", path: "/posts?feed=latest", isAuth: true},
		{name: "おすすめTL", method: "GET", path: "/posts?feed=recommended", isAuth: true},
		{name: "スレッド取得", method: "GET", path: "/posts/1/thread", isAuth: false},
		{name: "投稿単体取得", method: "GET", path: "/posts/1", isAuth: false},
		{name: "ユーザー投稿一覧", method: "GET", path: "/users/1/posts", isAuth: false},
		{name: "自分のプロフィール", method: "GET", path: "/me", isAuth: true},
		{name: "トレンド一覧", method: "GET", path: "/trending", isAuth: false},
		{name: "投稿検索", method: "GET", path: "/search?q=さくら&type=posts", isAuth: false},
		{name: "未読通知数", method: "GET", path: "/notifications/unread_count", isAuth: true},
	}

	reqChan := make(chan reqSpec, totalRequests)
	for i := 0; i < totalRequests; i++ {
		reqChan <- specs[i%len(specs)]
	}
	close(reqChan)

	var successCount int64
	var errorCount int64
	allDurations := make([]time.Duration, 0, totalRequests)
	results := make([]reqResult, 0, totalRequests)
	var resMu sync.Mutex

	var wg sync.WaitGroup
	start := time.Now()

	for c := 0; c < concurrency; c++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client := &http.Client{
				Timeout: 10 * time.Second,
				Transport: &http.Transport{
					MaxIdleConnsPerHost: concurrency,
				},
			}

			for spec := range reqChan {
				var bodyReader *bytes.Reader
				if spec.body != nil {
					bodyReader = bytes.NewReader(spec.body)
				} else {
					bodyReader = bytes.NewReader(nil)
				}

				req, err := http.NewRequest(spec.method, ts.URL+spec.path, bodyReader)
				if err != nil {
					atomic.AddInt64(&errorCount, 1)
					resMu.Lock()
					results = append(results, reqResult{spec: spec, elapsed: 0, err: true})
					resMu.Unlock()
					continue
				}

				if spec.isAuth && authCookie != nil {
					req.AddCookie(authCookie)
				}

				reqStart := time.Now()
				resp, err := client.Do(req)
				elapsed := time.Since(reqStart)

				isErr := err != nil || (resp != nil && resp.StatusCode >= 400)
				if isErr {
					atomic.AddInt64(&errorCount, 1)
				} else {
					atomic.AddInt64(&successCount, 1)
				}
				if resp != nil {
					testutil.CloseBody(resp)
				}

				resMu.Lock()
				allDurations = append(allDurations, elapsed)
				results = append(results, reqResult{spec: spec, elapsed: elapsed, err: isErr})
				resMu.Unlock()
			}
		}()
	}

	wg.Wait()
	totalDuration := time.Since(start)

	// 全体統計の算出
	sort.Slice(allDurations, func(i, j int) bool {
		return allDurations[i] < allDurations[j]
	})

	var totalDurSum time.Duration
	for _, d := range allDurations {
		totalDurSum += d
	}

	res := parallelResult{
		totalRequests: totalRequests,
		successCount:  successCount,
		errorCount:    errorCount,
		totalDuration: totalDuration,
		rps:           float64(successCount) / totalDuration.Seconds(),
	}

	if len(allDurations) > 0 {
		res.minDuration = allDurations[0]
		res.maxDuration = allDurations[len(allDurations)-1]
		res.avgDuration = totalDurSum / time.Duration(len(allDurations))
		res.p50Duration = allDurations[int(float64(len(allDurations))*0.50)]
		res.p95Duration = allDurations[int(float64(len(allDurations))*0.95)]
		p99Idx := int(float64(len(allDurations)) * 0.99)
		if p99Idx >= len(allDurations) {
			p99Idx = len(allDurations) - 1
		}
		res.p99Duration = allDurations[p99Idx]
	}

	// エンドポイント別統計の算出
	epMap := make(map[string]*endpointStat)
	for _, s := range specs {
		epMap[s.name] = &endpointStat{
			name:      s.name,
			method:    s.method,
			path:      s.path,
			durations: make([]time.Duration, 0),
		}
	}

	for _, r := range results {
		stat, ok := epMap[r.spec.name]
		if !ok {
			continue
		}
		stat.count++
		if r.err {
			stat.errors++
		}
		stat.durations = append(stat.durations, r.elapsed)
	}

	for _, s := range specs {
		stat := epMap[s.name]
		if len(stat.durations) == 0 {
			continue
		}
		sort.Slice(stat.durations, func(i, j int) bool {
			return stat.durations[i] < stat.durations[j]
		})

		var sum time.Duration
		for _, d := range stat.durations {
			sum += d
		}
		stat.min = stat.durations[0]
		stat.max = stat.durations[len(stat.durations)-1]
		stat.avg = sum / time.Duration(len(stat.durations))
		stat.p50 = stat.durations[int(float64(len(stat.durations))*0.50)]
		stat.p95 = stat.durations[int(float64(len(stat.durations))*0.95)]
		res.endpointStats = append(res.endpointStats, *stat)
	}

	return res
}

// TestBenchmark_ParallelAccess はアプリケーション設定 (appdb.New) そのままのDBインスタンスを使用して
// 並行アクセス時のスループット・応答時間を計測します。
func TestBenchmark_ParallelAccess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping parallel benchmark in short mode")
	}

	scale := 1
	if s := os.Getenv("BENCH_SCALE"); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			scale = v
		}
	}

	concurrency := 20
	if c := os.Getenv("BENCH_CONCURRENCY"); c != "" {
		if v, err := strconv.Atoi(c); err == nil && v > 0 {
			concurrency = v
		}
	}

	totalRequests := 100
	if r := os.Getenv("BENCH_REQUESTS"); r != "" {
		if v, err := strconv.Atoi(r); err == nil && v > 0 {
			totalRequests = v
		}
	}

	// DBスキーマの初期化・マイグレーション
	_ = testutil.SetupTestDB(t)

	// アプリ本体の設定ロジック (appdb.New) でDBインスタンスを生成して検証
	db := appdb.New()
	defer func() {
		_ = db.Close()
	}()

	t.Logf("シードデータを投入中 (scale=%d)...", scale)
	seedRes, err := seed.InsertSeedData(db, scale)
	if err != nil {
		t.Fatalf("シード投入に失敗しました: %v", err)
	}
	t.Logf("シードデータ投入完了 (所要時間: %.2f秒)", seedRes.Duration.Seconds())

	ts, _ := testutil.SetupTestServer(t, db)

	// 認証用ログインを実行してセッションCookieを取得
	jar, _ := cookiejar.New(nil)
	loginClient := &http.Client{Jar: jar}
	loginPayload, _ := json.Marshal(map[string]string{
		"email":    "user00001@example.com",
		"password": "password",
	})
	loginResp, err := loginClient.Post(ts.URL+"/login", "application/json", bytes.NewReader(loginPayload))
	if err != nil {
		t.Fatalf("ログイン失敗: %v", err)
	}
	testutil.CloseBody(loginResp)
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("ログインステータス不正: %d", loginResp.StatusCode)
	}

	var sessionCookie *http.Cookie
	for _, cookie := range loginResp.Cookies() {
		if cookie.Name == "session_id" {
			sessionCookie = cookie
			break
		}
	}

	res := runParallelBenchmark(t, ts, sessionCookie, concurrency, totalRequests)

	// コンソール出力
	fmt.Println()
	fmt.Printf("=========================================================================================================\n")
	fmt.Printf(" [並行アクセスベンチマーク結果 (appdb.New アプリ設定使用)] 並行度: %d, 総リクエスト数: %d\n", concurrency, totalRequests)
	fmt.Printf("=========================================================================================================\n")
	fmt.Printf("%-32s | %-12s | %-12s | %-12s | %-12s | %-12s | %-12s\n",
		"成功数 / 失敗数", "RPS (req/s)", "平均応答時間", "P50", "P95", "P99", "総所要時間")
	fmt.Printf("---------------------------------------------------------------------------------------------------------\n")
	fmt.Printf("%-30s | %10.2f/s | %12s | %12s | %12s | %12s | %12s\n",
		fmt.Sprintf("%d 成功 / %d 失敗", res.successCount, res.errorCount),
		res.rps,
		formatDuration(res.avgDuration),
		formatDuration(res.p50Duration),
		formatDuration(res.p95Duration),
		formatDuration(res.p99Duration),
		formatDuration(res.totalDuration),
	)
	fmt.Printf("=========================================================================================================\n")

	fmt.Println()
	fmt.Printf("%-24s | %-6s | %-28s | %-6s | %-8s | %-8s | %-8s | %-8s | %-8s\n",
		"Endpoint Name", "Method", "Path", "Reqs", "Errors", "Min", "Avg", "P50", "P95")
	fmt.Println(strings.Repeat("-", 125))
	for _, ep := range res.endpointStats {
		fmt.Printf("%-24s | %-6s | %-28s | %-6d | %-8d | %-8s | %-8s | %-8s | %-8s\n",
			ep.name,
			ep.method,
			ep.path,
			ep.count,
			ep.errors,
			formatDuration(ep.min),
			formatDuration(ep.avg),
			formatDuration(ep.p50),
			formatDuration(ep.p95),
		)
	}
	fmt.Println(strings.Repeat("-", 125))

	// GitHub Actions Step Summary へ出力
	writeParallelStepSummary(res, scale, concurrency, totalRequests, seedRes.Duration)
}

func writeParallelStepSummary(
	res parallelResult,
	scale int,
	concurrency int,
	totalRequests int,
	seedDuration time.Duration,
) {
	summaryPath := os.Getenv("GITHUB_STEP_SUMMARY")
	if summaryPath == "" {
		summaryPath = os.Getenv("BENCH_SUMMARY_FILE")
	}
	if summaryPath == "" {
		return
	}

	var md strings.Builder
	fmt.Fprintf(&md, "## Parallel Benchmark Results (appdb.New)\n\n")
	fmt.Fprintf(&md, "- **Scale**: `%d` (Seed Duration: `%.2fs`)\n", scale, seedDuration.Seconds())
	fmt.Fprintf(&md, "- **Concurrency**: `%d` workers\n", concurrency)
	fmt.Fprintf(&md, "- **Total Requests**: `%d` requests\n", totalRequests)
	fmt.Fprintf(&md, "- **Throughput**: **`%.2f req/s`** (Total Time: `%s`)\n", res.rps, formatDuration(res.totalDuration))
	fmt.Fprintf(&md, "- **Success / Errors**: `%d / %d`\n\n", res.successCount, res.errorCount)

	fmt.Fprintf(&md, "### Overall Latency Summary\n\n")
	fmt.Fprintf(&md, "| Metric | Value |\n")
	fmt.Fprintf(&md, "| :--- | :---: |\n")
	fmt.Fprintf(&md, "| Min Latency | `%s` |\n", formatDuration(res.minDuration))
	fmt.Fprintf(&md, "| Avg Latency | `%s` |\n", formatDuration(res.avgDuration))
	fmt.Fprintf(&md, "| P50 Latency | `%s` |\n", formatDuration(res.p50Duration))
	fmt.Fprintf(&md, "| P95 Latency | `%s` |\n", formatDuration(res.p95Duration))
	fmt.Fprintf(&md, "| P99 Latency | `%s` |\n", formatDuration(res.p99Duration))
	fmt.Fprintf(&md, "| Max Latency | `%s` |\n\n", formatDuration(res.maxDuration))

	fmt.Fprintf(&md, "### Endpoint Breakdown\n\n")
	fmt.Fprintf(&md, "| Endpoint Name | Method | Path | Requests | Errors | Min | Avg | P50 | P95 |\n")
	fmt.Fprintf(&md, "| :--- | :---: | :--- | :---: | :---: | :---: | :---: | :---: | :---: |\n")
	for _, ep := range res.endpointStats {
		fmt.Fprintf(&md, "| %s | `%s` | `%s` | `%d` | `%d` | %s | %s | %s | %s |\n",
			ep.name,
			ep.method,
			ep.path,
			ep.count,
			ep.errors,
			formatDuration(ep.min),
			formatDuration(ep.avg),
			formatDuration(ep.p50),
			formatDuration(ep.p95),
		)
	}
	md.WriteString("\n")

	f, err := os.OpenFile(summaryPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("Warning: failed to write to step summary file (%s): %v\n", summaryPath, err)
		return
	}
	defer func() {
		_ = f.Close()
	}()

	if _, err := f.WriteString(md.String()); err != nil {
		fmt.Printf("Warning: failed to append to step summary file (%s): %v\n", summaryPath, err)
	}
}
