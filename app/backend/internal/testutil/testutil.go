package testutil

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	appdb "sakuravel/internal/db"
	"sakuravel/internal/handler"
	"sakuravel/internal/middleware"
	"sakuravel/internal/realtime"
	"sakuravel/internal/server"

	_ "github.com/go-sql-driver/mysql"
)

func CloseBody(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
}

func SetupTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "sakuravel:password@tcp(localhost:3307)/sakuravel?parseTime=true&charset=utf8mb4&multiStatements=true"
	} else if !strings.Contains(dsn, "multiStatements=true") {
		if strings.Contains(dsn, "?") {
			dsn += "&multiStatements=true"
		} else {
			dsn += "?multiStatements=true"
		}
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Minute)

	// 疎通確認（CI環境などの起動遅延・瞬断に備えてリトライする）
	var pingErr error
	for i := 0; i < 20; i++ {
		pingErr = db.Ping()
		if pingErr == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if pingErr != nil {
		t.Fatalf("failed to ping test db (%s): %v (Is test DB container running on port 3307?)", dsn, pingErr)
	}

	ResetAndMigrateDB(t, db)

	t.Cleanup(func() {
		_ = db.Close()
	})

	return db
}

// ResetAndMigrateDB はデータベースを初期化し、golang-migrate を使ってスキーマを適用する
func ResetAndMigrateDB(t *testing.T, db *sql.DB) {
	t.Helper()

	// 1. 既存テーブルの全削除と再作成
	_, err := db.Exec(`
		DROP DATABASE IF EXISTS sakuravel;
		CREATE DATABASE sakuravel;
		USE sakuravel;
	`)
	if err != nil {
		t.Fatalf("failed to reset database: %v", err)
	}

	// 2. golang-migrate によるマイグレーション実行
	if err := appdb.RunMigrationsWithDB(db); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}
}

func SetupTestServer(t *testing.T, db *sql.DB) (*httptest.Server, *handler.Handler) {
	t.Helper()

	h := &handler.Handler{
		DB:            db,
		CookieSecure:  false,
		Notifications: realtime.NewHub(),
		Threads:       realtime.NewHub(),
	}
	auth := &middleware.Auth{DB: db}

	router := server.NewRouter(h, auth)
	ts := httptest.NewServer(router)

	t.Cleanup(func() {
		ts.Close()
	})

	return ts, h
}
