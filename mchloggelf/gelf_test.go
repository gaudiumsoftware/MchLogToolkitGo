package mchloggelf

import (
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"
)

func TestLevelToSyslog(t *testing.T) {
	tests := []struct {
		level    string
		expected int
	}{
		{"fatal", SyslogEmergency},
		{"error", SyslogError},
		{"warn", SyslogWarning},
		{"info", SyslogInformational},
		{"debug", SyslogDebug},
		{"test", SyslogDebug},
		{"unknown", SyslogInformational},
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			got := LevelToSyslog(tt.level)
			if got != tt.expected {
				t.Errorf("LevelToSyslog(%q) = %d, want %d", tt.level, got, tt.expected)
			}
		})
	}
}

func TestNewGELFMessageFromBytes(t *testing.T) {
	content := []byte(`{"message":"test message","level":"info","source":"/app/main.go","line":"42","trace":""}`)

	msg, err := NewGELFMessage("info", content, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if msg.Version != "1.1" {
		t.Errorf("version = %q, want %q", msg.Version, "1.1")
	}

	hostname, _ := os.Hostname()
	if msg.Host != hostname {
		t.Errorf("host = %q, want %q", msg.Host, hostname)
	}

	if msg.ShortMessage != "test message" {
		t.Errorf("short_message = %q, want %q", msg.ShortMessage, "test message")
	}

	if msg.Level != SyslogInformational {
		t.Errorf("level = %d, want %d", msg.Level, SyslogInformational)
	}

	if msg.Extra["source"] != "/app/main.go" {
		t.Errorf("extra[source] = %v, want %q", msg.Extra["source"], "/app/main.go")
	}

	if msg.Extra["line"] != "42" {
		t.Errorf("extra[line] = %v, want %q", msg.Extra["line"], "42")
	}

	// Timestamp should be recent
	now := float64(time.Now().UnixNano()) / 1e9
	if msg.Timestamp < now-5 || msg.Timestamp > now+1 {
		t.Errorf("timestamp %f is not recent (now=%f)", msg.Timestamp, now)
	}
}

func TestNewGELFMessageFromMap(t *testing.T) {
	content := map[string]string{
		"message": "map message",
		"level":   "debug",
		"source":  "/app/handler.go",
	}

	msg, err := NewGELFMessage("debug", content, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if msg.ShortMessage != "map message" {
		t.Errorf("short_message = %q, want %q", msg.ShortMessage, "map message")
	}

	if msg.Level != SyslogDebug {
		t.Errorf("level = %d, want %d", msg.Level, SyslogDebug)
	}
}

func TestNewGELFMessageWithError(t *testing.T) {
	content := []byte(`{"message":"something failed"}`)
	errLog := errors.New("connection refused")

	msg, err := NewGELFMessage("error", content, errLog)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if msg.Extra["error"] != "connection refused" {
		t.Errorf("extra[error] = %v, want %q", msg.Extra["error"], "connection refused")
	}

	if msg.Level != SyslogError {
		t.Errorf("level = %d, want %d", msg.Level, SyslogError)
	}
}

func TestNewGELFMessageEmptyMessage(t *testing.T) {
	content := []byte(`{"level":"info"}`)

	msg, err := NewGELFMessage("info", content, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if msg.ShortMessage == "" {
		t.Error("short_message should never be empty")
	}
}

func TestGELFMessageMarshalJSON(t *testing.T) {
	msg := &GELFMessage{
		Version:      "1.1",
		Host:         "testhost",
		ShortMessage: "test",
		Timestamp:    1234567890.123,
		Level:        SyslogInformational,
		Extra: map[string]any{
			"source":  "/app/main.go",
			"service": "my-service",
		},
	}

	data, err := msg.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	// Standard fields
	if result["version"] != "1.1" {
		t.Errorf("version = %v, want %q", result["version"], "1.1")
	}
	if result["host"] != "testhost" {
		t.Errorf("host = %v, want %q", result["host"], "testhost")
	}
	if result["short_message"] != "test" {
		t.Errorf("short_message = %v, want %q", result["short_message"], "test")
	}

	// Extra fields should have underscore prefix
	if result["_source"] != "/app/main.go" {
		t.Errorf("_source = %v, want %q", result["_source"], "/app/main.go")
	}
	if result["_service"] != "my-service" {
		t.Errorf("_service = %v, want %q", result["_service"], "my-service")
	}

	// Original keys without underscore should not exist
	if _, ok := result["source"]; ok {
		t.Error("field 'source' should not exist (should be '_source')")
	}
}

func TestGELFMessageMarshalJSONSkipsID(t *testing.T) {
	msg := &GELFMessage{
		Version:      "1.1",
		Host:         "testhost",
		ShortMessage: "test",
		Timestamp:    1234567890.123,
		Level:        SyslogInformational,
		Extra: map[string]any{
			"id": "should-be-skipped",
		},
	}

	data, err := msg.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if _, ok := result["_id"]; ok {
		t.Error("_id field should be skipped per GELF spec")
	}
}
