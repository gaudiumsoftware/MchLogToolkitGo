package mchlogcorev3

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Graylog2/go-gelf/gelf"
)

// TestLevelToSyslogMapping garante que cada level da toolkit mapeia
// para o severity syslog/GELF correto.
func TestLevelToSyslogMapping(t *testing.T) {
	cases := map[string]int32{
		"fatal": gelf.LOG_CRIT,    // 2
		"error": gelf.LOG_ERR,     // 3
		"warn":  gelf.LOG_WARNING, // 4
		"info":  gelf.LOG_INFO,    // 6
		"debug": gelf.LOG_DEBUG,   // 7
		"test":  gelf.LOG_DEBUG,   // 7 (test trata como debug)
	}
	for lvl, want := range cases {
		t.Run(lvl, func(t *testing.T) {
			if got := levelToSyslog(lvl); got != want {
				t.Errorf("levelToSyslog(%q) = %d, want %d", lvl, got, want)
			}
		})
	}
}

// TestLevelToSyslogUnknownDefaultsToInfo trata level fora do conjunto.
func TestLevelToSyslogUnknownDefaultsToInfo(t *testing.T) {
	if got := levelToSyslog("nonsense"); got != gelf.LOG_INFO {
		t.Errorf("unknown level should default to INFO(6), got %d", got)
	}
}

// TestBuildGELFMessageRequiredFields garante presença e valor dos
// campos obrigatórios do GELF 1.1 a partir de um payload []byte JSON
// que reproduz a saída do formatLog do logger.go.
func TestBuildGELFMessageRequiredFields(t *testing.T) {
	payload := []byte(`{"message":"hello","level":"info","source":"foo.go","line":"42","trace":""}`)
	msg, err := buildGELFMessage("payments-api", "info", payload, nil, BackendConfig{
		Source: "pod-1",
	})
	if err != nil {
		t.Fatalf("buildGELFMessage failed: %v", err)
	}
	if msg.Version != "1.1" {
		t.Errorf("Version = %q, want 1.1", msg.Version)
	}
	if msg.Host != "pod-1" {
		t.Errorf("Host = %q, want pod-1", msg.Host)
	}
	if msg.Short != "hello" {
		t.Errorf("Short = %q, want hello", msg.Short)
	}
	if msg.TimeUnix <= 0 {
		t.Errorf("TimeUnix should be set, got %f", msg.TimeUnix)
	}
	if msg.Level != gelf.LOG_INFO {
		t.Errorf("Level = %d, want %d", msg.Level, gelf.LOG_INFO)
	}
}

// TestBuildGELFMessageCustomFields garante composição correta de
// _application_name, _log_id, _level_name, _file e _line.
func TestBuildGELFMessageCustomFields(t *testing.T) {
	payload := []byte(`{"message":"hi","level":"debug","source":"internal/foo.go","line":"99","trace":"abc"}`)
	msg, err := buildGELFMessage("payments-api", "debug", payload, nil, BackendConfig{
		Source: "pod-1",
	})
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}

	checks := map[string]string{
		"_application_name": "payments-api",
		"_log_id":           "payments-api-mchlog-debug",
		"_level_name":       "debug",
		"_file":             "internal/foo.go",
		"_line":             "99",
		"_trace":            "abc",
	}
	for k, want := range checks {
		got, ok := msg.Extra[k]
		if !ok {
			t.Errorf("missing custom field %q", k)
			continue
		}
		if gs, _ := got.(string); gs != want {
			t.Errorf("Extra[%q] = %v, want %q", k, got, want)
		}
	}
}

// TestBuildGELFMessageWithError garante que um errLog não-nil produz
// _error e mantém os demais campos.
func TestBuildGELFMessageWithError(t *testing.T) {
	payload := []byte(`{"message":"boom","level":"error","source":"x.go","line":"7","trace":""}`)
	msg, err := buildGELFMessage("svc", "error", payload, errors.New("kaboom"), BackendConfig{
		Source: "pod-1",
	})
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	if msg.Level != gelf.LOG_ERR {
		t.Errorf("Level = %d, want %d", msg.Level, gelf.LOG_ERR)
	}
	got, ok := msg.Extra["_error"]
	if !ok {
		t.Fatalf("missing _error")
	}
	if s, _ := got.(string); !strings.Contains(s, "kaboom") {
		t.Errorf("_error = %v, want contains kaboom", got)
	}
}

// TestBuildGELFMessageAcceptsMap garante que content como map também
// funciona (caso da mensagem de init disparada pelo facade).
func TestBuildGELFMessageAcceptsMap(t *testing.T) {
	content := map[string]string{
		"message": "MchLogToolkit initialized",
		"version": "V3",
	}
	msg, err := buildGELFMessage("svc", "info", content, nil, BackendConfig{Source: "pod-1"})
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	if msg.Short != "MchLogToolkit initialized" {
		t.Errorf("Short = %q", msg.Short)
	}
	if v, _ := msg.Extra["_version"].(string); v != "V3" {
		t.Errorf("Extra[_version] = %v, want V3", msg.Extra["_version"])
	}
}

// TestBuildGELFMessageMissingMessage garante que payload sem campo
// "message" usa string vazia em Short e não falha.
func TestBuildGELFMessageMissingMessage(t *testing.T) {
	payload := []byte(`{"level":"info"}`)
	msg, err := buildGELFMessage("svc", "info", payload, nil, BackendConfig{Source: "pod-1"})
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	if msg.Short != "" {
		t.Errorf("Short = %q, want empty", msg.Short)
	}
}

// TestBuildGELFMessageInvalidJSONReturnsError garante que payload
// malformado produz erro em vez de panic.
func TestBuildGELFMessageInvalidJSONReturnsError(t *testing.T) {
	payload := []byte(`{not json`)
	if _, err := buildGELFMessage("svc", "info", payload, nil, BackendConfig{Source: "pod-1"}); err == nil {
		t.Fatalf("expected error on invalid JSON")
	}
}

// TestBuildGELFMessageSerializable confirma que a mensagem produzida
// serializa em JSON válido com Extra inline (formato exigido pelo GELF).
func TestBuildGELFMessageSerializable(t *testing.T) {
	payload := []byte(`{"message":"x","level":"info","source":"a.go","line":"1","trace":""}`)
	msg, err := buildGELFMessage("svc", "info", payload, nil, BackendConfig{Source: "pod-1"})
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	// MarshalJSONBuf é o método usado pelo gelf.Writer ao enviar.
	// Consumimos via json.Marshal padrão para o teste — aqui só
	// verificamos que os campos identificáveis são serializáveis.
	if _, err := json.Marshal(msg); err != nil {
		t.Fatalf("Message not serializable: %v", err)
	}
}
