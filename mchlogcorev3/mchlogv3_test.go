package mchlogcorev3

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net"
	"testing"
	"time"
)

// listenUDP cria um listener UDP em porta efêmera e devolve endereço
// (host:porta) e a conexão para leitura.
func listenUDP(t *testing.T) (string, net.PacketConn) {
	t.Helper()
	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen UDP: %v", err)
	}
	return conn.LocalAddr().String(), conn
}

// readDatagram lê um datagrama do listener, com timeout. Retorna os
// bytes brutos. Se o conteúdo estiver gzipado, descomprime.
func readDatagram(t *testing.T, conn net.PacketConn) []byte {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 64*1024)
	n, _, err := conn.ReadFrom(buf)
	if err != nil {
		t.Fatalf("ReadFrom: %v", err)
	}
	data := buf[:n]
	// gzip magic
	if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
		gr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("gzip reader: %v", err)
		}
		defer gr.Close()
		out, err := io.ReadAll(gr)
		if err != nil {
			t.Fatalf("gzip read: %v", err)
		}
		return out
	}
	return data
}

// TestGraylogUDPSendsValidGELF mostra que o transporte UDP entrega um
// datagrama em formato GELF 1.1 ao destino, com short_message, host,
// level e o custom field _application_name corretos.
func TestGraylogUDPSendsValidGELF(t *testing.T) {
	t.Cleanup(resetConfig)

	addr, conn := listenUDP(t)
	defer conn.Close()

	if err := Configure(DestinationConfig{Network: &NetworkConfig{
		Type:   NetworkGraylogUDP,
		Addr:   addr,
		Source: "pod-1",
	}}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := Initialize("/applog/payments-api/"); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	t.Cleanup(func() { _ = MchLog.Close() })

	payload := []byte(`{"message":"hello","level":"info","source":"x.go","line":"1","trace":""}`)
	MchLog.LogSubject("info", payload, nil)

	raw := readDatagram(t, conn)

	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("invalid GELF JSON: %v\nraw=%s", err, string(raw))
	}
	if got["version"] != "1.1" {
		t.Errorf("version=%v want 1.1", got["version"])
	}
	if got["host"] != "pod-1" {
		t.Errorf("host=%v want pod-1", got["host"])
	}
	if got["short_message"] != "hello" {
		t.Errorf("short_message=%v want hello", got["short_message"])
	}
	if got["_application_name"] != "payments-api" {
		t.Errorf("_application_name=%v want payments-api", got["_application_name"])
	}
	if got["_log_id"] != "payments-api-mchlog-info" {
		t.Errorf("_log_id=%v want payments-api-mchlog-info", got["_log_id"])
	}
	// level (syslog) chega como número JSON
	if lvl, ok := got["level"].(float64); !ok || int(lvl) != 6 {
		t.Errorf("level=%v want 6", got["level"])
	}
}

// TestGraylogUDPGetFileNameFromStreamName devolve descritor lógico
// quando V3 está inicializado (não há arquivo real).
func TestGraylogUDPGetFileNameFromStreamName(t *testing.T) {
	t.Cleanup(resetConfig)

	addr, conn := listenUDP(t)
	defer conn.Close()

	if err := Configure(DestinationConfig{Network: &NetworkConfig{Type: NetworkGraylogUDP, Addr: addr, Source: "pod-1"}}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := Initialize("/applog/payments-api/"); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	t.Cleanup(func() { _ = MchLog.Close() })

	got := MchLog.GetFileNameFromStreamName("info")
	want := "udp://" + addr + "/info"
	if got != want {
		t.Errorf("descriptor = %q want %q", got, want)
	}
}

// TestGraylogUDPCloseIdempotent garante que Close pode ser chamado
// múltiplas vezes sem erro.
func TestGraylogUDPCloseIdempotent(t *testing.T) {
	t.Cleanup(resetConfig)

	addr, conn := listenUDP(t)
	defer conn.Close()

	if err := Configure(DestinationConfig{Network: &NetworkConfig{Type: NetworkGraylogUDP, Addr: addr, Source: "pod-1"}}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := Initialize("/applog/svc/"); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	if err := MchLog.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := MchLog.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

// TestGraylogUDPInitializeRequiresConfigure garante que Initialize
// sem Configure prévio retorna erro claro.
func TestGraylogUDPInitializeRequiresConfigure(t *testing.T) {
	t.Cleanup(resetConfig)
	resetConfig()

	if err := Initialize("/applog/svc/"); err == nil {
		t.Fatalf("Initialize without Configure should error")
	}
}

// TestInitializeReentryClosesPrevious garante que chamar Initialize
// duas vezes fecha o destino anterior antes de instalar o novo
// (evita vazamento do socket UDP do graylogUDP).
func TestInitializeReentryClosesPrevious(t *testing.T) {
	t.Cleanup(resetConfig)

	addr, conn := listenUDP(t)
	defer conn.Close()

	if err := Configure(DestinationConfig{Network: &NetworkConfig{
		Type:   NetworkGraylogUDP,
		Addr:   addr,
		Source: "pod-1",
	}}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := Initialize("/applog/svc-a/"); err != nil {
		t.Fatalf("first Initialize: %v", err)
	}

	first := currentGraylogUDP(t)

	if err := Initialize("/applog/svc-b/"); err != nil {
		t.Fatalf("second Initialize: %v", err)
	}

	first.mu.Lock()
	closed := first.closed
	first.mu.Unlock()
	if !closed {
		t.Errorf("first impl should have been Closed by second Initialize")
	}

	t.Cleanup(func() { _ = MchLog.Close() })
}
