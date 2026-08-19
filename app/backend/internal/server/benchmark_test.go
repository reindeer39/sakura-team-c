package server_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"sakuravel/internal/seed"
	"sakuravel/internal/testutil"
)

type endpointBenchmarkTarget struct {
	name       string
	method     string
	path       string
	authClient *http.Client
	body       []byte
}

type benchmarkStat struct {
	name       string
	method     string
	path       string
	statusCode int
	count      int
	min        time.Duration
	max        time.Duration
	avg        time.Duration
	p50        time.Duration
	p95        time.Duration
}

func TestBenchmark_HeavyEndpoints(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping benchmark in short mode")
	}

	scale := 1
	if s := os.Getenv("BENCH_SCALE"); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			scale = v
		}
	}

	iterations := 10
	if it := os.Getenv("BENCH_ITERATIONS"); it != "" {
		if v, err := strconv.Atoi(it); err == nil && v > 0 {
			iterations = v
		}
	}

	db := testutil.SetupTestDB(t)
	ts, _ := testutil.SetupTestServer(t, db)

	t.Logf("シードデータを投入中 (scale=%d)...", scale)
	seedRes, err := seed.InsertSeedData(db, scale)
	if err != nil {
		t.Fatalf("シード投入に失敗しました: %v", err)
	}
	t.Logf("シードデータ投入完了 (所要時間: %.2f秒)", seedRes.Duration.Seconds())

	// 認証付きクライアント (user00001@example.com / password)
	jar, _ := cookiejar.New(nil)
	authClient := &http.Client{Jar: jar}

	loginPayload, _ := json.Marshal(map[string]string{
		"email":    "user00001@example.com",
		"password": "password",
	})
	loginResp, err := authClient.Post(ts.URL+"/login", "application/json", bytes.NewReader(loginPayload))
	if err != nil {
		t.Fatalf("ベンチマーク用ログイン失敗: %v", err)
	}
	defer testutil.CloseBody(loginResp)
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("ログインステータス不正: %d", loginResp.StatusCode)
	}

	// 未認証クライアント
	anonClient := &http.Client{}

	// 対象ユーザーIDと投稿IDの選定
	var targetUserID int64 = 1
	if len(seedRes.UserIDs) > 0 {
		targetUserID = seedRes.UserIDs[0]
	}
	var anotherUserID int64 = 2
	if len(seedRes.UserIDs) > 1 {
		anotherUserID = seedRes.UserIDs[1]
	}

	var normalPostID int64 = 1
	if len(seedRes.PostIDs) > 0 {
		normalPostID = seedRes.PostIDs[0]
	}

	// スレッド計測用に対象の投稿IDを選択（返信が紐づいている親投稿IDを優先して取得）
	var threadPostID int64
	err = db.QueryRow(`
		SELECT DISTINCT parent_post_id FROM posts WHERE parent_post_id IS NOT NULL LIMIT 1
	`).Scan(&threadPostID)
	if err != nil || threadPostID == 0 {
		threadPostID = normalPostID
	}

	// ページネーション用カーソルの取得 (最新投稿のIDなど)
	var cursorPostID int64
	err = db.QueryRow(`SELECT id FROM posts ORDER BY created_at DESC, id DESC LIMIT 1 OFFSET 20`).Scan(&cursorPostID)
	if err != nil {
		cursorPostID = normalPostID
	}

	targets := []endpointBenchmarkTarget{
		{
			name:       "1. フォロー中タイムライン",
			method:     "GET",
			path:       "/posts?feed=following",
			authClient: authClient,
		},
		{
			name:       "2. おすすめタイムライン",
			method:     "GET",
			path:       "/posts?feed=recommended",
			authClient: authClient,
		},
		{
			name:       "3. 最新タイムライン",
			method:     "GET",
			path:       "/posts?feed=latest",
			authClient: authClient,
		},
		{
			name:       "4. 最新タイムライン(ページ2)",
			method:     "GET",
			path:       fmt.Sprintf("/posts?feed=latest&limit=20&cursor=%d", cursorPostID),
			authClient: authClient,
		},
		{
			name:       "5. ユーザー投稿一覧",
			method:     "GET",
			path:       fmt.Sprintf("/users/%d/posts", targetUserID),
			authClient: anonClient,
		},
		{
			name:       "6. スレッドツリー取得",
			method:     "GET",
			path:       fmt.Sprintf("/posts/%d/thread", threadPostID),
			authClient: anonClient,
		},
		{
			name:       "7. 投稿単体取得",
			method:     "GET",
			path:       fmt.Sprintf("/posts/%d", normalPostID),
			authClient: anonClient,
		},
		{
			name:       "8. 投稿いいね一覧",
			method:     "GET",
			path:       fmt.Sprintf("/posts/%d/likes", normalPostID),
			authClient: anonClient,
		},
		{
			name:       "9. トレンド一覧",
			method:     "GET",
			path:       "/trending",
			authClient: anonClient,
		},
		{
			name:       "10. 投稿検索 (キーワードヒット)",
			method:     "GET",
			path:       "/search?q=さくら&type=posts",
			authClient: anonClient,
		},
		{
			name:       "11. 投稿検索 (0件ヒット)",
			method:     "GET",
			path:       "/search?q=nonexistentquery999&type=posts",
			authClient: anonClient,
		},
		{
			name:       "12. ユーザー検索",
			method:     "GET",
			path:       "/search?q=user&type=users",
			authClient: anonClient,
		},
		{
			name:       "13. 自分のプロフィール",
			method:     "GET",
			path:       "/me",
			authClient: authClient,
		},
		{
			name:       "14. ユーザープロフィール(足跡記録)",
			method:     "GET",
			path:       fmt.Sprintf("/profile/%d", anotherUserID),
			authClient: authClient,
		},
		{
			name:       "15. プロフィール更新 (PUT)",
			method:     "PUT",
			path:       "/profile",
			authClient: authClient,
			body:       []byte(`{"display_name":"更新ユーザー","bio":"ベンチマーク更新テスト"}`),
		},
		{
			name:       "16. フォロワー一覧",
			method:     "GET",
			path:       fmt.Sprintf("/users/%d/followers", targetUserID),
			authClient: anonClient,
		},
		{
			name:       "17. フォロー中一覧",
			method:     "GET",
			path:       fmt.Sprintf("/users/%d/following", targetUserID),
			authClient: anonClient,
		},
		{
			name:       "18. 足跡一覧",
			method:     "GET",
			path:       "/me/footprints",
			authClient: authClient,
		},
		{
			name:       "19. 通知一覧",
			method:     "GET",
			path:       "/notifications",
			authClient: authClient,
		},
		{
			name:       "20. 未読通知数",
			method:     "GET",
			path:       "/notifications/unread_count",
			authClient: authClient,
		},
		{
			name:       "21. 通知既読化 (POST)",
			method:     "POST",
			path:       "/notifications/read",
			authClient: authClient,
		},
		{
			name:       "22. いいね作成 (POST)",
			method:     "POST",
			path:       "/likes",
			authClient: authClient,
			body:       []byte(fmt.Sprintf(`{"post_id":%d}`, normalPostID)),
		},
		{
			name:       "23. いいね解除 (DELETE)",
			method:     "DELETE",
			path:       fmt.Sprintf("/likes/%d", normalPostID),
			authClient: authClient,
		},
		{
			name:       "24. リポスト作成 (POST)",
			method:     "POST",
			path:       "/reposts",
			authClient: authClient,
			body:       []byte(fmt.Sprintf(`{"post_id":%d}`, normalPostID)),
		},
		{
			name:       "25. リポスト解除 (DELETE)",
			method:     "DELETE",
			path:       fmt.Sprintf("/reposts/%d", normalPostID),
			authClient: authClient,
		},
		{
			name:       "26. ユーザーフォロー (POST)",
			method:     "POST",
			path:       fmt.Sprintf("/users/%d/follow", anotherUserID),
			authClient: authClient,
		},
		{
			name:       "27. ユーザーフォロー解除 (DELETE)",
			method:     "DELETE",
			path:       fmt.Sprintf("/users/%d/follow", anotherUserID),
			authClient: authClient,
		},
		{
			name:       "28. 投稿作成 (POST)",
			method:     "POST",
			path:       "/posts",
			authClient: authClient,
			body:       []byte(`{"content":"ベンチマーク新規投稿 #test"}`),
		},
		{
			name:       "29. 返信作成 (POST)",
			method:     "POST",
			path:       "/replies",
			authClient: authClient,
			body:       []byte(fmt.Sprintf(`{"post_id":%d,"content":"ベンチマーク返信"}`, normalPostID)),
		},
	}

	fmt.Println()
	fmt.Printf("========================================================================================\n")
	fmt.Printf(" [ベンチマーク] 重いエンドポイント応答時間計測 (Scale: %d, 試行回数: %d回)\n", scale, iterations)
	fmt.Printf("========================================================================================\n")

	stats := make([]benchmarkStat, 0, len(targets))

	for _, target := range targets {
		reqURL := ts.URL + target.path

		// ウォームアップ（初回クエリコンパイル・バッファロード等）
		var warmupBody *bytes.Reader
		if target.body != nil {
			warmupBody = bytes.NewReader(target.body)
		} else {
			warmupBody = bytes.NewReader(nil)
		}
		warmupReq, err := http.NewRequest(target.method, reqURL, warmupBody)
		if err != nil {
			t.Fatalf("ウォームアップリクエスト作成失敗 (%s): %v", target.name, err)
		}
		if target.body != nil {
			warmupReq.Header.Set("Content-Type", "application/json")
		}
		warmupResp, err := target.authClient.Do(warmupReq)
		if err != nil {
			t.Fatalf("ウォームアップリクエスト実行失敗 (%s): %v", target.name, err)
		}
		if warmupResp.StatusCode < 200 || warmupResp.StatusCode >= 300 {
			t.Fatalf("[%s] ウォームアップリクエスト異常ステータス: %d", target.name, warmupResp.StatusCode)
		}
		testutil.CloseBody(warmupResp)

		durations := make([]time.Duration, 0, iterations)
		var lastStatusCode int

		for i := 0; i < iterations; i++ {
			var bodyReader *bytes.Reader
			if target.body != nil {
				bodyReader = bytes.NewReader(target.body)
			} else {
				bodyReader = bytes.NewReader(nil)
			}
			req, err := http.NewRequest(target.method, reqURL, bodyReader)
			if err != nil {
				t.Fatalf("リクエスト作成失敗: %v", err)
			}
			if target.body != nil {
				req.Header.Set("Content-Type", "application/json")
			}

			start := time.Now()
			resp, err := target.authClient.Do(req)
			elapsed := time.Since(start)

			if err != nil {
				t.Fatalf("[%s] リクエスト失敗 (回数: %d): %v", target.name, i+1, err)
			}
			lastStatusCode = resp.StatusCode
			testutil.CloseBody(resp)

			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				t.Fatalf("[%s] 予期しないステータスコード (回数: %d): %d", target.name, i+1, resp.StatusCode)
			}

			durations = append(durations, elapsed)
		}

		sort.Slice(durations, func(i, j int) bool {
			return durations[i] < durations[j]
		})

		var totalDur time.Duration
		for _, d := range durations {
			totalDur += d
		}

		p50Index := int(float64(len(durations)) * 0.50)
		p95Index := int(float64(len(durations)) * 0.95)
		if p95Index >= len(durations) {
			p95Index = len(durations) - 1
		}

		stat := benchmarkStat{
			name:       target.name,
			method:     target.method,
			path:       target.path,
			statusCode: lastStatusCode,
			count:      iterations,
			min:        durations[0],
			max:        durations[len(durations)-1],
			avg:        totalDur / time.Duration(iterations),
			p50:        durations[p50Index],
			p95:        durations[p95Index],
		}
		stats = append(stats, stat)
	}

	// 計測結果テーブルの表示
	var report strings.Builder
	report.WriteString("\n")
	fmt.Fprintf(&report, "%-26s | %-6s | %-28s | %-6s | %-9s | %-9s | %-9s | %-9s | %-9s\n",
		"Endpoint Name", "Method", "Path", "Status", "Min", "Avg", "P50", "P95", "Max")
	report.WriteString(strings.Repeat("-", 125) + "\n")

	for _, s := range stats {
		fmt.Fprintf(&report, "%-26s | %-6s | %-28s | %-6d | %-9s | %-9s | %-9s | %-9s | %-9s\n",
			s.name,
			s.method,
			s.path,
			s.statusCode,
			formatDuration(s.min),
			formatDuration(s.avg),
			formatDuration(s.p50),
			formatDuration(s.p95),
			formatDuration(s.max),
		)
	}
	report.WriteString(strings.Repeat("-", 125) + "\n")

	fmt.Print(report.String())
	t.Log(report.String())

	writeStepSummary(stats, scale, iterations, seedRes.Duration)
}

func writeStepSummary(stats []benchmarkStat, scale int, iterations int, seedDuration time.Duration) {
	summaryPath := os.Getenv("GITHUB_STEP_SUMMARY")
	if summaryPath == "" {
		summaryPath = os.Getenv("BENCH_SUMMARY_FILE")
	}
	if summaryPath == "" {
		return
	}

	var md strings.Builder
	fmt.Fprintf(&md, "## Backend api benchmark Results\n\n")
	fmt.Fprintf(&md, "- **Scale**: `%d` (Seed Duration: `%.2fs`)\n", scale, seedDuration.Seconds())
	fmt.Fprintf(&md, "- **Iterations**: `%d` times per endpoint\n\n", iterations)
	fmt.Fprintf(&md, "| Endpoint Name | Method | Path | Status | Min | Avg | P50 | P95 | Max |\n")
	fmt.Fprintf(&md, "| :--- | :---: | :--- | :---: | :---: | :---: | :---: | :---: | :---: |\n")

	for _, s := range stats {
		fmt.Fprintf(&md, "| %s | `%s` | `%s` | `%d` | %s | %s | %s | %s | %s |\n",
			s.name,
			s.method,
			s.path,
			s.statusCode,
			formatDuration(s.min),
			formatDuration(s.avg),
			formatDuration(s.p50),
			formatDuration(s.p95),
			formatDuration(s.max),
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

func formatDuration(d time.Duration) string {
	if d < time.Millisecond {
		return fmt.Sprintf("%.2fµs", float64(d.Microseconds()))
	}
	return fmt.Sprintf("%.2fms", float64(d.Microseconds())/1000.0)
}
