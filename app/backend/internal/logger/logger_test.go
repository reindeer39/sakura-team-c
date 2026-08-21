package logger

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestNewLogger_TextAndJson(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	// Test JSON logger to file
	cfg := Config{
		Output: logPath,
		Format: "json",
		Level:  "debug",
	}

	logger, cleanup, err := NewLogger(cfg)
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	logger.Info("test json message", "key1", "val1", "num", 42)
	cleanup()

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	var logEntry map[string]any
	if err := json.Unmarshal(data, &logEntry); err != nil {
		t.Fatalf("log is not valid JSON: %v, raw: %s", err, string(data))
	}

	if logEntry["msg"] != "test json message" {
		t.Errorf("expected msg 'test json message', got %v", logEntry["msg"])
	}
	if logEntry["key1"] != "val1" {
		t.Errorf("expected key1 'val1', got %v", logEntry["key1"])
	}
}

func TestNewLogger_LogLevels(t *testing.T) {
	tests := []struct {
		levelStr string
		expected slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"INFO", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"unknown", slog.LevelInfo},
	}

	for _, tt := range tests {
		t.Run(tt.levelStr, func(t *testing.T) {
			got := parseLevel(tt.levelStr)
			if got != tt.expected {
				t.Errorf("parseLevel(%q) = %v; want %v", tt.levelStr, got, tt.expected)
			}
		})
	}
}

func TestNewLogger_OtlpInit(t *testing.T) {
	cfg := Config{
		Output:       "otlp",
		OtlpEndpoint: "localhost:4317",
		OtlpInsecure: true,
		ServiceName:  "test-service",
		Level:        "info",
	}

	logger, cleanup, err := NewLogger(cfg)
	if err != nil {
		t.Fatalf("failed to create otlp logger: %v", err)
	}
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
	// Verify calling log doesn't panic even without a running collector
	logger.Info("test otlp log message", "key", "value")
	cleanup()
}

func TestConfigFromEnv(t *testing.T) {
	os.Setenv("LOG_OUTPUT", "stderr")
	os.Setenv("LOG_FORMAT", "json")
	os.Setenv("LOG_LEVEL", "warn")
	os.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "custom:4317")
	os.Setenv("OTEL_SERVICE_NAME", "custom-service")
	defer func() {
		os.Unsetenv("LOG_OUTPUT")
		os.Unsetenv("LOG_FORMAT")
		os.Unsetenv("LOG_LEVEL")
		os.Unsetenv("OTEL_EXPORTER_OTLP_ENDPOINT")
		os.Unsetenv("OTEL_SERVICE_NAME")
	}()

	cfg := ConfigFromEnv()
	if cfg.Output != "stderr" {
		t.Errorf("expected Output stderr, got %s", cfg.Output)
	}
	if cfg.Format != "json" {
		t.Errorf("expected Format json, got %s", cfg.Format)
	}
	if cfg.Level != "warn" {
		t.Errorf("expected Level warn, got %s", cfg.Level)
	}
	if cfg.OtlpEndpoint != "custom:4317" {
		t.Errorf("expected OtlpEndpoint custom:4317, got %s", cfg.OtlpEndpoint)
	}
	if cfg.ServiceName != "custom-service" {
		t.Errorf("expected ServiceName custom-service, got %s", cfg.ServiceName)
	}
}
