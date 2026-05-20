package mchlogcore

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net"
	"testing"
	"time"

	"github.com/gaudiumsoftware/mchlogtoolkitgo/mchlogcorev3"
)

func listenUDP(t *testing.T) (string, net.PacketConn) {
	t.Helper()
	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen UDP: %v", err)
	}
	return conn.LocalAddr().String(), conn
}

func readDatagram(t *testing.T, conn net.PacketConn) []byte {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 64*1024)
	n, _, err := conn.ReadFrom(buf)
	if err != nil {
		t.Fatalf("ReadFrom: %v", err)
	}
	data := buf[:n]
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

// TestSetVersionV3DispatchesToGraylog cobre o caminho completo:
// Configure → SetVersion(V3) → InitializeMchLog → MchLog.LogSubject
// gera datagrama GELF no listener mock com _application_name correto.
func TestSetVersionV3DispatchesToGraylog(t *testing.T) {
	t.Cleanup(func() { SetVersion(V1) })

	addr, conn := listenUDP(t)
	defer conn.Close()

	if err := mchlogcorev3.Configure(mchlogcorev3.DestinationConfig{
		Network: &mchlogcorev3.NetworkConfig{
			Type:   mchlogcorev3.NetworkGraylogUDP,
			Addr:   addr,
			Source: "pod-1",
		},
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	SetVersion(V3)
	InitializeMchLog("/applog/payments-api/")
	t.Cleanup(func() { _ = MchLog.Close() })

	// O primeiro datagrama é a mensagem de "MchLogToolkit initialized".
	first := readDatagram(t, conn)
	var initMsg map[string]any
	if err := json.Unmarshal(first, &initMsg); err != nil {
		t.Fatalf("init datagram invalid JSON: %v\nraw=%s", err, first)
	}
	if got := initMsg["short_message"]; got != "MchLogToolkit initialized" {
		t.Errorf("init short_message = %v", got)
	}
	if got := initMsg["_version"]; got != "V3" {
		t.Errorf("init _version = %v want V3", got)
	}

	// Mensagem de aplicação.
	MchLog.LogSubject("info", []byte(`{"message":"hello","level":"info","source":"x.go","line":"1","trace":""}`), nil)
	raw := readDatagram(t, conn)

	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("invalid GELF JSON: %v", err)
	}
	if got["short_message"] != "hello" {
		t.Errorf("short_message=%v", got["short_message"])
	}
	if got["host"] != "pod-1" {
		t.Errorf("host=%v", got["host"])
	}
	if got["_application_name"] != "payments-api" {
		t.Errorf("_application_name=%v", got["_application_name"])
	}
	if got["_log_id"] != "payments-api-mchlog-info" {
		t.Errorf("_log_id=%v", got["_log_id"])
	}
}
