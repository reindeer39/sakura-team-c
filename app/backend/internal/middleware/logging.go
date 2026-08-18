package middleware

import (
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type responseWriter struct {
	http.ResponseWriter
	status int
	size   int64
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.size += int64(n)
	return n, err
}

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
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rw, r)

		duration := time.Since(start)
		durationMs := duration.Milliseconds()

		attrs := []any{
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.status,
			"duration_ms", durationMs,
			"size", rw.size,
		}

		// SSEなどのストリーミングエンドポイントはスローログ判定から除外
		isStream := strings.HasSuffix(r.URL.Path, "/stream")

		if rw.status >= http.StatusInternalServerError {
			slog.Error("http request error", attrs...)
		} else if !isStream && duration >= slowRequestThreshold {
			attrs = append(attrs, "threshold_ms", slowRequestThreshold.Milliseconds())
			slog.Warn("slow http request", attrs...)
		} else {
			slog.Debug("http request", attrs...)
		}
	})
}
