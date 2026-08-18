package middleware

import (
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

var (
	slowRequestThreshold = 200 * time.Millisecond
)

func init() {
	if thresholdStr := os.Getenv("SLOW_REQUEST_THRESHOLD_MS"); thresholdStr != "" {
		if ms, err := strconv.Atoi(thresholdStr); err == nil && ms > 0 {
			slowRequestThreshold = time.Duration(ms) * time.Millisecond
		}
	}
}

func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		next.ServeHTTP(w, r)

		duration := time.Since(start)
		durationMs := duration.Milliseconds()

		attrs := []any{
			"method", r.Method,
			"path", r.URL.Path,
			"duration_ms", durationMs,
		}

		// SSEなどのストリーミングエンドポイントはスローログ判定から除外
		isStream := strings.HasSuffix(r.URL.Path, "/stream")

		if !isStream && duration >= slowRequestThreshold {
			attrs = append(attrs, "threshold_ms", slowRequestThreshold.Milliseconds())
			slog.Warn("slow http request", attrs...)
		} else {
			slog.Debug("http request", attrs...)
		}
	})
}
