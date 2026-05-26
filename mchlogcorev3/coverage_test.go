package mchlogcorev3

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Graylog2/go-gelf/gelf"
)

func deadline2s() time.Time    { return time.Now().Add(2 * time.Second) }
func deadline100ms() time.Time { return time.Now().Add(100 * time.Millisecond) }

// TestDatagramLevelMappingAllLevels percorre cada level da toolkit e
// verifica que o datagrama enviado carrega o severity numérico correto
// e o _level_name textual correspondente.
func TestDatagramLevelMappingAllLevels(t *testing.T) {
	cases := []struct {
		level      string
		wantSyslog int32
	}{
		{"fatal", gelf.LOG_CRIT},
		{"error", gelf.LOG_ERR},
		{"warn", gelf.LOG_WARNING},
		{"info", gelf.LOG_INFO},
		{"debug", gelf.LOG_DEBUG},
		{"test", gelf.LOG_DEBUG},
	}

	for _, tc := range cases {
		t.Run(tc.level, func(t *testing.T) {
			t.Cleanup(resetConfig)
			addr, conn := listenUDP(t)
			defer conn.Close()

			if err := Configure(DestinationConfig{Network: &NetworkConfig{Type: NetworkGraylogUDP, Addr: addr, Source: "pod-1"}}); err != nil {
				t.Fatalf("Configure: %v", err)
			}
			if err := Initialize("/applog/svc/"); err != nil {
				t.Fatalf("Initialize: %v", err)
			}
			t.Cleanup(func() { _ = MchLog.Close() })

			payload := []byte(`{"message":"x","level":"` + tc.level + `","file":"a.go","line":"1","trace":""}`)
			MchLog.LogSubject(tc.level, payload, nil)

			raw := readDatagram(t, conn)
			var got map[string]any
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatalf("invalid GELF JSON: %v", err)
			}
			if lvl, _ := got["level"].(float64); int32(lvl) != tc.wantSyslog {
				t.Errorf("level=%v want %d", got["level"], tc.wantSyslog)
			}
			if got["_level_name"] != tc.level {
				t.Errorf("_level_name=%v want %q", got["_level_name"], tc.level)
			}
			if got["_log_id"] != "svc-mchlog-"+tc.level {
				t.Errorf("_log_id=%v want svc-mchlog-%s", got["_log_id"], tc.level)
			}
		})
	}
}

// TestDatagramGZIPDisabled garante que com DisableGZIP=true o datagrama
// chega como JSON bruto (sem magic bytes de gzip).
func TestDatagramGZIPDisabled(t *testing.T) {
	t.Cleanup(resetConfig)
	addr, conn := listenUDP(t)
	defer conn.Close()

	if err := Configure(DestinationConfig{Network: &NetworkConfig{Type: NetworkGraylogUDP, Addr: addr, Source: "pod-1", DisableGZIP: true}}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := Initialize("/applog/svc/"); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	t.Cleanup(func() { _ = MchLog.Close() })

	MchLog.LogSubject("info", []byte(`{"message":"plain"}`), nil)

	// readDatagram já descomprime se gzip; aqui consultamos os primeiros
	// bytes via ReadFrom direto antes da descompressão para conferir.
	// Como readDatagram não dá acesso aos bytes brutos, testamos via
	// um listener próprio.
	addr2, conn2 := listenUDP(t)
	defer conn2.Close()
	if err := Configure(DestinationConfig{Network: &NetworkConfig{Type: NetworkGraylogUDP, Addr: addr2, Source: "pod-1", DisableGZIP: true}}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	_ = MchLog.Close()
	if err := Initialize("/applog/svc/"); err != nil {
		t.Fatalf("Initialize2: %v", err)
	}
	MchLog.LogSubject("info", []byte(`{"message":"plain"}`), nil)

	buf := make([]byte, 4096)
	_ = conn2.SetReadDeadline(deadline2s())
	n, _, err := conn2.ReadFrom(buf)
	if err != nil {
		t.Fatalf("ReadFrom: %v", err)
	}
	data := buf[:n]
	if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
		t.Fatalf("expected uncompressed payload, got gzip magic")
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("uncompressed payload should be JSON: %v\nraw=%s", err, string(data))
	}
	if got["short_message"] != "plain" {
		t.Errorf("short_message=%v", got["short_message"])
	}
}

// TestDatagramGZIPEnabled garante que GZIP default produz datagrama
// com magic bytes 0x1f 0x8b.
func TestDatagramGZIPEnabled(t *testing.T) {
	t.Cleanup(resetConfig)
	addr, conn := listenUDP(t)
	defer conn.Close()

	if err := Configure(DestinationConfig{Network: &NetworkConfig{Type: NetworkGraylogUDP, Addr: addr, Source: "pod-1"}}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := Initialize("/applog/svc/"); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	t.Cleanup(func() { _ = MchLog.Close() })

	MchLog.LogSubject("info", []byte(`{"message":"compressed"}`), nil)

	buf := make([]byte, 4096)
	_ = conn.SetReadDeadline(deadline2s())
	n, _, err := conn.ReadFrom(buf)
	if err != nil {
		t.Fatalf("ReadFrom: %v", err)
	}
	data := buf[:n]
	if !(len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b) {
		t.Fatalf("expected gzip magic, got %x", data[:min(len(data), 4)])
	}
}

// TestLogSubjectEmptySubjectIgnored garante que subject vazio não
// dispara envio (early return previne datagrama vazio).
func TestLogSubjectEmptySubjectIgnored(t *testing.T) {
	t.Cleanup(resetConfig)
	addr, conn := listenUDP(t)
	defer conn.Close()

	if err := Configure(DestinationConfig{Network: &NetworkConfig{Type: NetworkGraylogUDP, Addr: addr, Source: "pod-1"}}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := Initialize("/applog/svc/"); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	t.Cleanup(func() { _ = MchLog.Close() })

	MchLog.LogSubject("", []byte(`{"message":"x"}`), nil)

	_ = conn.SetReadDeadline(deadline100ms())
	buf := make([]byte, 4096)
	if _, _, err := conn.ReadFrom(buf); err == nil {
		t.Fatalf("empty subject should not produce a datagram")
	}
}

// TestNilReceiverSafety mostra que chamadas em ponteiro nil (caso V3
// dispatch antes de Initialize) não panicam.
func TestNilReceiverSafety(t *testing.T) {
	var g *graylogUDP
	g.LogSubject("info", []byte(`{"message":"x"}`), nil) // não deve panicar
	if got := g.GetFileNameFromStreamName("info"); got != "" {
		t.Errorf("nil GetFileNameFromStreamName=%q want empty", got)
	}
	if err := g.Close(); err != nil {
		t.Errorf("nil Close=%v want nil", err)
	}
}

// TestContentToMapStringJSON exercita a branch de string contendo JSON.
func TestContentToMapStringJSON(t *testing.T) {
	got, err := contentToMap(`{"k":"v","n":1}`)
	if err != nil {
		t.Fatalf("string JSON: %v", err)
	}
	if got["k"] != "v" {
		t.Errorf("k=%v", got["k"])
	}
}

// TestContentToMapReflectFallback exercita o fallback via reflect para
// mapas com value type estático.
func TestContentToMapReflectFallback(t *testing.T) {
	got, err := contentToMap(map[string]int{"n": 42})
	if err != nil {
		t.Fatalf("reflect fallback: %v", err)
	}
	if v, _ := got["n"].(int); v != 42 {
		t.Errorf("n=%v", got["n"])
	}
}

// TestContentToMapUnsupportedType garante erro claro para tipos fora
// do contrato (ex.: int).
func TestContentToMapUnsupportedType(t *testing.T) {
	if _, err := contentToMap(123); err == nil {
		t.Fatalf("expected error for unsupported type")
	}
}

// TestContentToMapNil garante erro para nil content.
func TestContentToMapNil(t *testing.T) {
	if _, err := contentToMap(nil); err == nil {
		t.Fatalf("expected error for nil content")
	}
}

// TestServiceFromPathVariants cobre formatos comuns de path e
// rejeita paths degenerados ("./", ".", "..", roots Windows).
func TestServiceFromPathVariants(t *testing.T) {
	cases := map[string]string{
		"/applog/payments-api/": "payments-api",
		"/applog/svc":           "svc",
		"./applog/svc/":         "svc",
		"":                      "",
		"/":                     "",
		"./":                    "",
		".":                     "",
		"..":                    "",
		"C:/":                   "",
		"C:\\":                  "",
	}
	for in, want := range cases {
		if got := serviceFromPath(in); got != want {
			t.Errorf("serviceFromPath(%q)=%q want %q", in, got, want)
		}
	}
}

