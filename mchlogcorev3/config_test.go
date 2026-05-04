package mchlogcorev3

import (
	"testing"
)

// TestDefaultSource garante que o helper devolve uma string não vazia
// (tipicamente o hostname ou "unknown" caso o sistema não retorne hostname).
func TestDefaultSource(t *testing.T) {
	got := DefaultSource()
	if got == "" {
		t.Fatalf("DefaultSource should never return empty string")
	}
}

// TestConfigureAppliesDefaults garante que Protocol recebe valor default
// quando não especificado e que GZIP fica habilitado por padrão. Source e
// Addr não são auto-preenchidos.
func TestConfigureAppliesDefaults(t *testing.T) {
	t.Cleanup(resetConfig)

	cfg := NetworkConfig{
		Addr:   "graylog.dev:12201",
		Source: "svc-x",
	}
	if err := Configure(cfg); err != nil {
		t.Fatalf("Configure failed: %v", err)
	}

	got := ActiveConfig()
	if got.Protocol != ProtocolGraylogUDP {
		t.Errorf("Protocol default mismatch: got %q want %q", got.Protocol, ProtocolGraylogUDP)
	}
	if got.DisableGZIP {
		t.Errorf("GZIP must be enabled by default (DisableGZIP=false)")
	}
	if got.Source != "svc-x" {
		t.Errorf("Source must not be auto-filled: got %q", got.Source)
	}
	if got.Addr != "graylog.dev:12201" {
		t.Errorf("Addr mismatch: got %q", got.Addr)
	}
}

// TestConfigureDisableGZIPRespected garante que callers podem desabilitar
// gzip explicitamente.
func TestConfigureDisableGZIPRespected(t *testing.T) {
	t.Cleanup(resetConfig)

	if err := Configure(NetworkConfig{
		Addr:        "graylog.dev:12201",
		Source:      "svc-x",
		DisableGZIP: true,
	}); err != nil {
		t.Fatalf("Configure failed: %v", err)
	}
	if !ActiveConfig().DisableGZIP {
		t.Errorf("DisableGZIP=true was overwritten")
	}
}

// TestConfigureRejectsEmptyAddr garante validação obrigatória de Addr.
func TestConfigureRejectsEmptyAddr(t *testing.T) {
	t.Cleanup(resetConfig)

	err := Configure(NetworkConfig{Source: "svc-x"})
	if err == nil {
		t.Fatalf("Configure should reject empty Addr")
	}
}

// TestConfigureRejectsEmptySource garante validação obrigatória de Source.
// Source é caller-provided por design (toolkit não autodetecta).
func TestConfigureRejectsEmptySource(t *testing.T) {
	t.Cleanup(resetConfig)

	err := Configure(NetworkConfig{Addr: "graylog.dev:12201"})
	if err == nil {
		t.Fatalf("Configure should reject empty Source")
	}
}

// TestConfigureRejectsUnknownProtocol garante que protocolos não suportados
// retornem erro (futuro-proofing para quando outros protocolos forem
// adicionados).
func TestConfigureRejectsUnknownProtocol(t *testing.T) {
	t.Cleanup(resetConfig)

	err := Configure(NetworkConfig{
		Protocol: "graylog-tcp",
		Addr:     "graylog.dev:12201",
		Source:   "svc-x",
	})
	if err == nil {
		t.Fatalf("Configure should reject unknown protocol")
	}
}

// TestActiveConfigBeforeConfigure garante que consultar ActiveConfig
// antes de Configure não panica e devolve config zero-valued.
func TestActiveConfigBeforeConfigure(t *testing.T) {
	t.Cleanup(resetConfig)
	resetConfig()

	got := ActiveConfig()
	if got.Addr != "" || got.Source != "" {
		t.Errorf("ActiveConfig before Configure should be zero-valued, got %+v", got)
	}
}
