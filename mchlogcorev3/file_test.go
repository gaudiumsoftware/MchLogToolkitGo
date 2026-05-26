package mchlogcorev3

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFileOnlyWritesV2LayoutAndShape garante que sem Network configurado
// o V3 grava o log no caminho <basePath>/<service>/<subject>/<subject>.log
// (mesmo layout do V2) e usa a mesma JSON shape do V2.
func TestFileOnlyWritesV2LayoutAndShape(t *testing.T) {
	t.Cleanup(resetConfig)

	dir := t.TempDir()
	if err := Configure(DestinationConfig{}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	servicePath := filepath.Join(dir, "payments-api") + string(filepath.Separator)
	if err := Initialize(servicePath); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	t.Cleanup(func() { _ = MchLog.Close() })

	payload := []byte(`{"message":"hello","level":"info","file":"x.go","line":"1","trace":""}`)
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
	if got["file"] != "x.go" {
		t.Errorf("file=%v", got["file"])
	}
	if _, hasTimestamp := got["timestamp"]; !hasTimestamp {
		t.Errorf("missing timestamp")
	}
}

// TestFileErrorPrefixesSubject garante que erros vão para
// pasta err_<level>/, mantendo o comportamento do V2.
func TestFileErrorPrefixesSubject(t *testing.T) {
	t.Cleanup(resetConfig)

	dir := t.TempDir()
	if err := Configure(DestinationConfig{}); err != nil {
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

// TestFileOnlyGetFileNameFromStreamName devolve caminho real de
// arquivo (delegando ao V2) quando não há network configurado.
func TestFileOnlyGetFileNameFromStreamName(t *testing.T) {
	t.Cleanup(resetConfig)

	dir := t.TempDir()
	if err := Configure(DestinationConfig{}); err != nil {
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

// TestFileCloseIsNoOp documenta que Close é no-op para o file impl
// (V2 subjacente não expõe Close — decisão prévia). Idempotente.
func TestFileCloseIsNoOp(t *testing.T) {
	t.Cleanup(resetConfig)

	dir := t.TempDir()
	if err := Configure(DestinationConfig{}); err != nil {
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
