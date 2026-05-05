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

// TestConfigureDefaultProtocolIsFile garante que sem Protocol explícito
// o default aplicado é ProtocolFile (caminho de menor surpresa para
// callers migrando de V2).
func TestConfigureDefaultProtocolIsFile(t *testing.T) {
	t.Cleanup(resetConfig)

	if err := Configure(DestinationConfig{}); err != nil {
		t.Fatalf("Configure with empty config should be valid for file: %v", err)
	}
	if got := ActiveConfig().Protocol; got != ProtocolFile {
		t.Errorf("default Protocol = %q, want %q", got, ProtocolFile)
	}
}

// TestConfigureFileNoRequiredFields garante que ProtocolFile não exige
// Addr nem Source (essas são exclusivas do GraylogUDP).
func TestConfigureFileNoRequiredFields(t *testing.T) {
	t.Cleanup(resetConfig)

	if err := Configure(DestinationConfig{Protocol: ProtocolFile}); err != nil {
		t.Fatalf("Configure should accept file with no fields: %v", err)
	}
}

// TestConfigureGraylogUDPRequiresAddr.
func TestConfigureGraylogUDPRequiresAddr(t *testing.T) {
	t.Cleanup(resetConfig)

	err := Configure(DestinationConfig{Protocol: ProtocolGraylogUDP, Source: "svc-x"})
	if err == nil {
		t.Fatalf("Configure should reject empty Addr for ProtocolGraylogUDP")
	}
}

// TestConfigureGraylogUDPRequiresSource.
func TestConfigureGraylogUDPRequiresSource(t *testing.T) {
	t.Cleanup(resetConfig)

	err := Configure(DestinationConfig{Protocol: ProtocolGraylogUDP, Addr: "graylog.dev:12201"})
	if err == nil {
		t.Fatalf("Configure should reject empty Source for ProtocolGraylogUDP")
	}
}

// TestConfigureGraylogUDPHappy garante que o caminho válido aceita config.
func TestConfigureGraylogUDPHappy(t *testing.T) {
	t.Cleanup(resetConfig)

	if err := Configure(DestinationConfig{
		Protocol: ProtocolGraylogUDP,
		Addr:     "graylog.dev:12201",
		Source:   "svc-x",
	}); err != nil {
		t.Fatalf("Configure failed: %v", err)
	}
	got := ActiveConfig()
	if got.Protocol != ProtocolGraylogUDP {
		t.Errorf("Protocol = %q", got.Protocol)
	}
	if got.DisableGZIP {
		t.Errorf("GZIP must be enabled by default (DisableGZIP=false)")
	}
	if got.Addr != "graylog.dev:12201" {
		t.Errorf("Addr = %q", got.Addr)
	}
}

// TestConfigureDisableGZIPRespected garante que callers podem desabilitar
// gzip explicitamente.
func TestConfigureDisableGZIPRespected(t *testing.T) {
	t.Cleanup(resetConfig)

	if err := Configure(DestinationConfig{
		Protocol:    ProtocolGraylogUDP,
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

// TestConfigureRejectsUnknownProtocol garante futuro-proofing para
// quando outros protocolos forem adicionados.
func TestConfigureRejectsUnknownProtocol(t *testing.T) {
	t.Cleanup(resetConfig)

	err := Configure(DestinationConfig{Protocol: "graylog-tcp"})
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
	if IsConfigured() {
		t.Errorf("IsConfigured should be false before Configure")
	}
}
