package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"

	appdb "sakuravel/internal/db"
	"sakuravel/internal/handler"
	"sakuravel/internal/middleware"
	"sakuravel/internal/realtime"
	"sakuravel/internal/server"
)

func main() {
	if err := appdb.RunMigrations(); err != nil {
		slog.Error("Failed to run migrations", "error", err)
		os.Exit(1)
	}
	db := appdb.New()
	defer func() {
		if err := db.Close(); err != nil {
			slog.Error("failed to close db", "error", err)
		}
	}()

	h := &handler.Handler{
		DB:            db,
		CookieSecure:  os.Getenv("COOKIE_SECURE") == "true",
		Notifications: realtime.NewHub(),
		Threads:       realtime.NewHub(),
	}
	auth := &middleware.Auth{DB: db}

	mux := http.NewServeMux()

	//  ミドルウェア
	middlewares := middleware.LoggingMiddleware(middleware.CorsMiddleware(server.NewRouter(h, auth)))
	mux.Handle("/", middlewares)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	slog.Info(fmt.Sprintf("starting server on :%s", port))

	err := http.ListenAndServe(":"+port, mux)

	if err != nil {
		slog.Error("Failed to start server", "error", err, "port", port)
		os.Exit(1)
	}

}
