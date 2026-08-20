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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	appdb "sakuravel/internal/db"
	"sakuravel/internal/seed"
	"sakuravel/internal/testutil"
)

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
}

type reqSpec struct {
	name   string
	method string
	path   string
	isAuth bool
	body   []byte
}

func runParallelBenchmark(
	t *testing.T,
	ts *httptest.Server,
	authCookie *http.Cookie,
	concurrency int,
	totalRequests int,
) parallelResult {
	specs := []reqSpec{
		{name: "タイムライン(フォロー中)", method: "GET", path: "/posts?feed=following", isAuth: true},
		{name: "タイムライン(最新)", method: "GET", path: "/posts?feed=latest", isAuth: true},
		{name: "タイムライン(おすすめ)", method: "GET", path: "/posts?feed=recommended", isAuth: true},
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
	durations := make([]time.Duration, 0, totalRequests)
	var durMu sync.Mutex

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
					continue
				}

				if spec.isAuth && authCookie != nil {
					req.AddCookie(authCookie)
				}

				reqStart := time.Now()
				resp, err := client.Do(req)
				elapsed := time.Since(reqStart)

				if err != nil || resp.StatusCode >= 400 {
					atomic.AddInt64(&errorCount, 1)
				} else {
					atomic.AddInt64(&successCount, 1)
				}
				if resp != nil {
					testutil.CloseBody(resp)
				}

				durMu.Lock()
				durations = append(durations, elapsed)
				durMu.Unlock()
			}
		}()
	}

	wg.Wait()
	totalDuration := time.Since(start)

	sort.Slice(durations, func(i, j int) bool {
		return durations[i] < durations[j]
	})

	var totalDurSum time.Duration
	for _, d := range durations {
		totalDurSum += d
	}

	res := parallelResult{
		totalRequests: totalRequests,
		successCount:  successCount,
		errorCount:    errorCount,
		totalDuration: totalDuration,
		rps:           float64(successCount) / totalDuration.Seconds(),
	}

	if len(durations) > 0 {
		res.minDuration = durations[0]
		res.maxDuration = durations[len(durations)-1]
		res.avgDuration = totalDurSum / time.Duration(len(durations))
		res.p50Duration = durations[int(float64(len(durations))*0.50)]
		res.p95Duration = durations[int(float64(len(durations))*0.95)]
		p99Idx := int(float64(len(durations)) * 0.99)
		if p99Idx >= len(durations) {
			p99Idx = len(durations) - 1
		}
		res.p99Duration = durations[p99Idx]
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
	defer db.Close()

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

	// 結果出力
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
}
