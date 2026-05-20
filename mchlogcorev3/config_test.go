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

// TestConfigureZeroValueIsFileOnly garante que sem Network configurado
// o V3 entra em modo file-only (caminho de menor surpresa para callers
// migrando de V2).
func TestConfigureZeroValueIsFileOnly(t *testing.T) {
	t.Cleanup(resetConfig)

	if err := Configure(DestinationConfig{}); err != nil {
		t.Fatalf("Configure with empty config should be valid for file-only: %v", err)
	}
	if got := ActiveConfig().Network; got != nil {
		t.Errorf("expected Network==nil, got %+v", got)
	}
}

// TestConfigureNetworkRequiresType garante que NetworkConfig sem Type
// é rejeitado.
func TestConfigureNetworkRequiresType(t *testing.T) {
	t.Cleanup(resetConfig)

	err := Configure(DestinationConfig{Network: &NetworkConfig{
		Addr:   "graylog.dev:12201",
		Source: "svc-x",
	}})
	if err == nil {
		t.Fatalf("Configure should reject empty Network.Type")
	}
}

// TestConfigureGraylogUDPRequiresAddr.
func TestConfigureGraylogUDPRequiresAddr(t *testing.T) {
	t.Cleanup(resetConfig)

	err := Configure(DestinationConfig{Network: &NetworkConfig{
		Type:   NetworkGraylogUDP,
		Source: "svc-x",
	}})
	if err == nil {
		t.Fatalf("Configure should reject empty Addr for NetworkGraylogUDP")
	}
}

// TestConfigureGraylogUDPRequiresSource.
func TestConfigureGraylogUDPRequiresSource(t *testing.T) {
	t.Cleanup(resetConfig)

	err := Configure(DestinationConfig{Network: &NetworkConfig{
		Type: NetworkGraylogUDP,
		Addr: "graylog.dev:12201",
	}})
	if err == nil {
		t.Fatalf("Configure should reject empty Source for NetworkGraylogUDP")
	}
}

// TestConfigureGraylogUDPHappy garante que o caminho válido aceita config.
func TestConfigureGraylogUDPHappy(t *testing.T) {
	t.Cleanup(resetConfig)

	if err := Configure(DestinationConfig{Network: &NetworkConfig{
		Type:   NetworkGraylogUDP,
		Addr:   "graylog.dev:12201",
		Source: "svc-x",
	}}); err != nil {
		t.Fatalf("Configure failed: %v", err)
	}
	got := ActiveConfig()
	if got.Network == nil {
		t.Fatalf("expected Network!=nil")
	}
	if got.Network.Type != NetworkGraylogUDP {
		t.Errorf("Type = %q", got.Network.Type)
	}
	if got.Network.DisableGZIP {
		t.Errorf("GZIP must be enabled by default (DisableGZIP=false)")
	}
	if got.Network.Addr != "graylog.dev:12201" {
		t.Errorf("Addr = %q", got.Network.Addr)
	}
}

// TestConfigureDisableGZIPRespected garante que callers podem desabilitar
// gzip explicitamente.
func TestConfigureDisableGZIPRespected(t *testing.T) {
	t.Cleanup(resetConfig)

	if err := Configure(DestinationConfig{Network: &NetworkConfig{
		Type:        NetworkGraylogUDP,
		Addr:        "graylog.dev:12201",
		Source:      "svc-x",
		DisableGZIP: true,
	}}); err != nil {
		t.Fatalf("Configure failed: %v", err)
	}
	if !ActiveConfig().Network.DisableGZIP {
		t.Errorf("DisableGZIP=true was overwritten")
	}
}

// TestConfigureRejectsUnknownNetworkType garante futuro-proofing para
// quando outros transportes forem adicionados.
func TestConfigureRejectsUnknownNetworkType(t *testing.T) {
	t.Cleanup(resetConfig)

	err := Configure(DestinationConfig{Network: &NetworkConfig{
		Type:   "graylog-tcp",
		Addr:   "graylog.dev:12201",
		Source: "svc-x",
	}})
	if err == nil {
		t.Fatalf("Configure should reject unknown Network.Type")
	}
}

// TestConfigureCopiesNetworkConfig garante que mutações do caller após
// Configure não afetam o estado armazenado.
func TestConfigureCopiesNetworkConfig(t *testing.T) {
	t.Cleanup(resetConfig)

	cfg := &NetworkConfig{
		Type:   NetworkGraylogUDP,
		Addr:   "graylog.dev:12201",
		Source: "svc-x",
	}
	if err := Configure(DestinationConfig{Network: cfg}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	cfg.Addr = "evil.dev:1"

	if got := ActiveConfig().Network.Addr; got != "graylog.dev:12201" {
		t.Errorf("internal config mutated by caller, Addr=%q", got)
	}
}

// TestConfigureNetworkSubjectsRequireNetwork garante que listar subjects
// extras sem destino de rede é erro claro.
func TestConfigureNetworkSubjectsRequireNetwork(t *testing.T) {
	t.Cleanup(resetConfig)

	err := Configure(DestinationConfig{NetworkSubjects: []string{"foo"}})
	if err == nil {
		t.Fatalf("Configure should reject NetworkSubjects without Network")
	}
}

// TestConfigureNetworkSubjectsCopied garante que mutações no slice
// passado pelo caller não afetam o estado armazenado.
func TestConfigureNetworkSubjectsCopied(t *testing.T) {
	t.Cleanup(resetConfig)

	subs := []string{"historico_posicao_taxi"}
	if err := Configure(DestinationConfig{
		Network: &NetworkConfig{
			Type:   NetworkGraylogUDP,
			Addr:   "graylog.dev:12201",
			Source: "svc-x",
		},
		NetworkSubjects: subs,
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	subs[0] = "tampered"

	got := ActiveConfig().NetworkSubjects
	if len(got) != 1 || got[0] != "historico_posicao_taxi" {
		t.Errorf("internal NetworkSubjects mutated by caller: %v", got)
	}
}

// TestActiveConfigBeforeConfigure garante que consultar ActiveConfig
// antes de Configure não panica e devolve config zero-valued.
func TestActiveConfigBeforeConfigure(t *testing.T) {
	t.Cleanup(resetConfig)
	resetConfig()

	got := ActiveConfig()
	if got.Network != nil {
		t.Errorf("ActiveConfig before Configure should be zero-valued, got Network=%+v", got.Network)
	}
	if len(got.NetworkSubjects) != 0 {
		t.Errorf("ActiveConfig before Configure should be zero-valued, got NetworkSubjects=%v", got.NetworkSubjects)
	}
	if IsConfigured() {
		t.Errorf("IsConfigured should be false before Configure")
	}
}
