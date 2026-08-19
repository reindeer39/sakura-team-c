package middleware

import (
	"net/http"
	"os"
)

// CorsMiddleware は CORS ヘッダーを設定するミドルウェア
func CorsMiddleware(next http.Handler) http.Handler {
	allowedOrigin := os.Getenv("ALLOWED_ORIGIN")
	if allowedOrigin == "" {
		allowedOrigin = "http://localhost:3000"
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
