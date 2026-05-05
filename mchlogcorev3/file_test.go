package mchlogcorev3

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestProtocolFileWritesV2LayoutAndShape garante que com ProtocolFile o
// V3 grava o log no caminho <basePath>/<service>/<level>/<level>.log
// (mesmo layout do V2) e usa a mesma JSON shape do V2.
func TestProtocolFileWritesV2LayoutAndShape(t *testing.T) {
	t.Cleanup(resetConfig)

	dir := t.TempDir()
	if err := Configure(BackendConfig{Protocol: ProtocolFile}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	servicePath := filepath.Join(dir, "payments-api") + string(filepath.Separator)
	if err := Initialize(servicePath); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	t.Cleanup(func() { _ = MchLog.Close() })

	payload := []byte(`{"message":"hello","level":"info","source":"x.go","line":"1","trace":""}`)
	MchLog.LogSubject("info", payload, nil)

	expected := MchLog.GetFileNameFromStreamName("info")
	if expected == "" {
		t.Fatalf("expected non-empty file path")
	}
	if !strings.Contains(expected, filepath.Join("info", "info.log")) {
		t.Errorf("expected V2 layout (.../info/info.log), got %q", expected)
	}

	data, err := os.ReadFile(expected)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	// Cada linha é um JSON (zerolog grava 1 evento por linha).
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 0 {
		t.Fatalf("no log lines written")
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &got); err != nil {
		t.Fatalf("invalid JSON line: %v\nline=%s", err, lines[len(lines)-1])
	}
	// Mesma shape do V2 atual: campos sem prefixo "_".
	if got["message"] != "hello" {
		t.Errorf("message=%v want hello", got["message"])
	}
	if got["level"] != "info" {
		t.Errorf("level=%v want info", got["level"])
	}
	if got["source"] != "x.go" {
		t.Errorf("source=%v", got["source"])
	}
	if _, hasTimestamp := got["timestamp"]; !hasTimestamp {
		t.Errorf("missing timestamp")
	}
}

// TestProtocolFileErrorPrefixesSubject garante que erros vão para
// pasta err_<level>/, mantendo o comportamento do V2.
func TestProtocolFileErrorPrefixesSubject(t *testing.T) {
	t.Cleanup(resetConfig)

	dir := t.TempDir()
	if err := Configure(BackendConfig{Protocol: ProtocolFile}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	servicePath := filepath.Join(dir, "svc") + string(filepath.Separator)
	if err := Initialize(servicePath); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	t.Cleanup(func() { _ = MchLog.Close() })

	payload := []byte(`{"message":"boom","level":"error"}`)
	MchLog.LogSubject("error", payload, errForTest("kaboom"))

	errPath := filepath.Join(dir, "svc", "err_error", "err_error.log")
	if _, err := os.Stat(errPath); err != nil {
		t.Fatalf("expected err_error file at %q: %v", errPath, err)
	}
}

// TestProtocolFileGetFileNameFromStreamName devolve caminho real de
// arquivo (delegando ao V2).
func TestProtocolFileGetFileNameFromStreamName(t *testing.T) {
	t.Cleanup(resetConfig)

	dir := t.TempDir()
	if err := Configure(BackendConfig{Protocol: ProtocolFile}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	servicePath := filepath.Join(dir, "svc") + string(filepath.Separator)
	if err := Initialize(servicePath); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	t.Cleanup(func() { _ = MchLog.Close() })

	got := MchLog.GetFileNameFromStreamName("info")
	want := filepath.Join(dir, "svc", "info", "info.log")
	if got != want {
		t.Errorf("path = %q want %q", got, want)
	}
}

// TestProtocolFileCloseIsNoOp documenta que Close é no-op para V3-file
// (V2 subjacente não expõe Close — decisão prévia). Idempotente.
func TestProtocolFileCloseIsNoOp(t *testing.T) {
	t.Cleanup(resetConfig)

	dir := t.TempDir()
	if err := Configure(BackendConfig{Protocol: ProtocolFile}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	servicePath := filepath.Join(dir, "svc") + string(filepath.Separator)
	if err := Initialize(servicePath); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := MchLog.Close(); err != nil {
		t.Errorf("Close error: %v", err)
	}
	// Idempotência:
	if err := MchLog.Close(); err != nil {
		t.Errorf("second Close error: %v", err)
	}
}

// errForTest é um helper conciso para construir um error.
type fakeErr string

func (e fakeErr) Error() string { return string(e) }

func errForTest(msg string) error { return fakeErr(msg) }
