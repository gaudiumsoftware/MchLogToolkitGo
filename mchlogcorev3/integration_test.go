//go:build integration

// Integration tests envia logs reais para um Graylog acessível via UDP.
// É opt-in via build tag para não rodar no CI padrão. Requer a env
// GRAYLOG_TEST_ADDR (ex.: "localhost:12201").
//
// Exemplo de execução local com docker:
//   docker run -d --name graylog-test -p 12201:12201/udp graylog/graylog:5.0
//   GRAYLOG_TEST_ADDR=localhost:12201 go test -tags=integration -v ./mchlogcorev3/...

package mchlogcorev3

import (
	"os"
	"testing"
)

func TestIntegrationSendsToRealGraylog(t *testing.T) {
	addr := os.Getenv("GRAYLOG_TEST_ADDR")
	if addr == "" {
		t.Skip("GRAYLOG_TEST_ADDR not set; skipping integration test")
	}

	t.Cleanup(resetConfig)

	if err := Configure(BackendConfig{
		Protocol: ProtocolGraylogUDP,
		Addr:     addr,
		Source:   "mchlog-integration-test",
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := Initialize("/applog/mchlog-test/"); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	t.Cleanup(func() { _ = MchLog.Close() })

	payload := []byte(`{"message":"integration smoke","level":"info","source":"integration_test.go","line":"1","trace":""}`)
	MchLog.LogSubject("info", payload, nil)
	// UDP é fire-and-forget; a ausência de panic/erro no caller já é
	// confirmação suficiente. A presença real no Graylog deve ser
	// inspecionada manualmente na UI.
}
