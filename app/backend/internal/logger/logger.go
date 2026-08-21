package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

type Config struct {
	Output       string
	Format       string
	Level        string
	OtlpEndpoint string
	OtlpInsecure bool
	ServiceName  string
}

func ConfigFromEnv() Config {
	output := strings.TrimSpace(os.Getenv("LOG_OUTPUT"))
	if output == "" {
		output = "stdout"
	}

	format := strings.ToLower(strings.TrimSpace(os.Getenv("LOG_FORMAT")))
	if format == "" {
		format = "text"
	}

	level := strings.ToLower(strings.TrimSpace(os.Getenv("LOG_LEVEL")))
	if level == "" {
		level = "info"
	}

	otlpEndpoint := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	if otlpEndpoint == "" {
		otlpEndpoint = "localhost:4317"
	}

	otlpInsecure := true
	if insecureStr := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_INSECURE")); insecureStr != "" {
		otlpInsecure = insecureStr == "true" || insecureStr == "1"
	}

	serviceName := strings.TrimSpace(os.Getenv("OTEL_SERVICE_NAME"))
	if serviceName == "" {
		serviceName = "sakuravel-api"
	}

	return Config{
		Output:       output,
		Format:       format,
		Level:        level,
		OtlpEndpoint: otlpEndpoint,
		OtlpInsecure: otlpInsecure,
		ServiceName:  serviceName,
	}
}

func parseLevel(levelStr string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(levelStr)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	case "info":
		fallthrough
	default:
		return slog.LevelInfo
	}
}

func NewLogger(cfg Config) (*slog.Logger, func(), error) {
	logLevel := parseLevel(cfg.Level)
	outputLower := strings.ToLower(cfg.Output)

	if outputLower == "otlp" || outputLower == "grpc" {
		return newOtlpLogger(cfg, logLevel)
	}

	var writer io.Writer
	cleanup := func() {}

	switch outputLower {
	case "stdout", "":
		writer = os.Stdout
	case "stderr":
		writer = os.Stderr
	default:
		dir := filepath.Dir(cfg.Output)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, nil, fmt.Errorf("failed to create log directory: %w", err)
		}
		file, err := os.OpenFile(cfg.Output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to open log file %s: %w", cfg.Output, err)
		}
		writer = file
		cleanup = func() {
			_ = file.Sync()
			_ = file.Close()
		}
	}

	opts := &slog.HandlerOptions{
		Level: logLevel,
	}

	var handler slog.Handler
	if strings.ToLower(cfg.Format) == "json" {
		handler = slog.NewJSONHandler(writer, opts)
	} else {
		handler = slog.NewTextHandler(writer, opts)
	}

	logger := slog.New(handler)
	return logger, cleanup, nil
}

func newOtlpLogger(cfg Config, logLevel slog.Level) (*slog.Logger, func(), error) {
	ctx := context.Background()

	var grpcOpts []otlploggrpc.Option
	if cfg.OtlpEndpoint != "" {
		grpcOpts = append(grpcOpts, otlploggrpc.WithEndpoint(cfg.OtlpEndpoint))
	}
	if cfg.OtlpInsecure {
		grpcOpts = append(grpcOpts, otlploggrpc.WithInsecure())
	}

	exporter, err := otlploggrpc.New(ctx, grpcOpts...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create otlp log exporter: %w", err)
	}

	res, err := resource.New(
		ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(cfg.ServiceName),
		),
		resource.WithProcess(),
		resource.WithOS(),
		resource.WithHost(),
	)
	if err != nil {
		res = resource.Default()
	}

	processor := sdklog.NewBatchProcessor(exporter)
	loggerProvider := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(processor),
		sdklog.WithResource(res),
	)

	cleanup := func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = loggerProvider.Shutdown(shutdownCtx)
	}

	logger := otelslog.NewLogger(
		cfg.ServiceName,
		otelslog.WithLoggerProvider(loggerProvider),
	)

	return logger, cleanup, nil
}

// SetupFromEnv reads configuration from environment variables, sets the default slog logger, and returns a cleanup function.
func SetupFromEnv() func() {
	cfg := ConfigFromEnv()
	logger, cleanup, err := NewLogger(cfg)
	if err != nil {
		// Fallback to standard error logger
		slog.Default().Error("Failed to initialize configured logger, falling back to default", "error", err)
		return func() {}
	}

	slog.SetDefault(logger)
	return cleanup
}
