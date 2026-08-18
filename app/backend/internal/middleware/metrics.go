package middleware

import (
	"context"
	"log"
	"net/http"
	"time"
)

type ctxKey string

const endpointKey ctxKey = "endpoint"

func MetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		endpointName := r.Method + " " + r.URL.Path
		ctx := context.WithValue(r.Context(), endpointKey, endpointName)
		next.ServeHTTP(w, r.WithContext(ctx))
		duration := time.Since(start)
		if duration > 200*time.Millisecond {
			log.Printf("[API_SLOW_LOG] %s took %s", endpointName, duration)
		}
	})
}
