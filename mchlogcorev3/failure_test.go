package mchlogcorev3

import (
	"bytes"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// captureStderr substitui os.Stderr por uma pipe enquanto fn roda e
// devolve tudo que foi escrito.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	original := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w

	var (
		buf bytes.Buffer
		wg  sync.WaitGroup
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(&buf, r)
	}()

	fn()

	_ = w.Close()
	wg.Wait()
	os.Stderr = original
	return buf.String()
}

// TestSendFailureDoesNotPanic garante que um content inválido (que faz
// buildGELFMessage falhar) não panica e silencia o erro para o caller.
func TestSendFailureDoesNotPanic(t *testing.T) {
	t.Cleanup(resetConfig)
	addr, conn := listenUDP(t)
	defer conn.Close()

	if err := Configure(DestinationConfig{Protocol: ProtocolGraylogUDP, Addr: addr, Source: "pod-1"}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := Initialize("/applog/svc/"); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	t.Cleanup(func() { _ = MchLog.Close() })

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("LogSubject panicked: %v", r)
		}
	}()
	// 123 (int) é tipo não suportado por contentToMap.
	MchLog.LogSubject("info", 123, nil)
}

// TestRateLimitedWarnOneLinePerWindow garante que 100 falhas seguidas
// dentro da janela de warnWindow produzem exatamente uma linha em
// stderr. O caminho exercitado aqui é a falha em buildGELFMessage
// (content type não suportado); a falha em writer.WriteMessage é
// coberta separadamente por TestWriterWriteMessageFailureWarns.
func TestRateLimitedWarnOneLinePerWindow(t *testing.T) {
	t.Cleanup(resetConfig)
	addr, conn := listenUDP(t)
	defer conn.Close()

	if err := Configure(DestinationConfig{Protocol: ProtocolGraylogUDP, Addr: addr, Source: "pod-1"}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := Initialize("/applog/svc/"); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	t.Cleanup(func() { _ = MchLog.Close() })

	got := captureStderr(t, func() {
		for i := 0; i < 100; i++ {
			MchLog.LogSubject("info", 123, nil) // builder fails
		}
	})

	count := strings.Count(got, "GELF UDP send failed")
	if count != 1 {
		t.Errorf("expected 1 warn line in window, got %d. stderr:\n%s", count, got)
	}
}

// TestRateLimitedWarnEmitsAgainAfterWindow garante que após a janela
// expirar, um novo aviso é emitido.
func TestRateLimitedWarnEmitsAgainAfterWindow(t *testing.T) {
	t.Cleanup(resetConfig)
	addr, conn := listenUDP(t)
	defer conn.Close()

	if err := Configure(DestinationConfig{Protocol: ProtocolGraylogUDP, Addr: addr, Source: "pod-1"}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := Initialize("/applog/svc/"); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	t.Cleanup(func() { _ = MchLog.Close() })

	got := captureStderr(t, func() {
		MchLog.LogSubject("info", 123, nil) // 1ª falha → warn
		// força janela a "expirar" zerando lastWarn no backend interno
		// (mesmo pacote, acesso a campo unexported permitido).
		MchLog.mu.RLock()
		impl := MchLog.impl
		MchLog.mu.RUnlock()
		g, ok := impl.(*graylogUDP)
		if !ok {
			t.Fatalf("expected *graylogUDP, got %T", impl)
		}
		g.mu.Lock()
		g.lastWarn = time.Time{}
		g.mu.Unlock()
		MchLog.LogSubject("info", 123, nil) // 2ª falha → novo warn
	})

	if c := strings.Count(got, "GELF UDP send failed"); c != 2 {
		t.Errorf("expected 2 warn lines after window reset, got %d. stderr:\n%s", c, got)
	}
}

// TestNotConfiguredErrorMessage garante que o erro de Initialize sem
// Configure prévio menciona Configure (mensagem orientativa).
func TestNotConfiguredErrorMessage(t *testing.T) {
	t.Cleanup(resetConfig)
	resetConfig()

	err := Initialize("/applog/svc/")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "Configure") {
		t.Errorf("error should mention Configure: %q", err.Error())
	}
}

// TestInitializeBadServicePath garante que um path do qual não dá para
// extrair nome de serviço resulta em erro claro.
func TestInitializeBadServicePath(t *testing.T) {
	t.Cleanup(resetConfig)

	if err := Configure(DestinationConfig{Protocol: ProtocolGraylogUDP, Addr: "127.0.0.1:1", Source: "pod-1"}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := Initialize(""); err == nil {
		t.Fatalf("expected error for empty path")
	}
}

// TestInitializeDialFailureReturnsError cobre o caminho em que
// gelf.NewWriter falha (Addr malformado, sem porta) e Initialize
// devolve o erro embrulhado em "dial GELF UDP %s: %w".
func TestInitializeDialFailureReturnsError(t *testing.T) {
	t.Cleanup(resetConfig)

	if err := Configure(DestinationConfig{
		Protocol: ProtocolGraylogUDP,
		Addr:     "no-port-no-colon",
		Source:   "pod-1",
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	err := Initialize("/applog/svc/")
	if err == nil {
		t.Fatalf("expected dial error for malformed Addr")
	}
	if !strings.Contains(err.Error(), "dial GELF UDP") {
		t.Errorf("error should mention dial: %q", err.Error())
	}
}

// TestWriterWriteMessageFailureWarns garante que uma falha em
// writer.WriteMessage (não em buildGELFMessage) também aciona
// warnOnce. Forçamos a falha fechando o socket subjacente do
// gelf.Writer antes de chamar LogSubject — o WriteMessage tenta
// escrever em conexão fechada e devolve erro.
func TestWriterWriteMessageFailureWarns(t *testing.T) {
	t.Cleanup(resetConfig)

	addr, conn := listenUDP(t)
	defer conn.Close()

	if err := Configure(DestinationConfig{Protocol: ProtocolGraylogUDP, Addr: addr, Source: "pod-1"}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := Initialize("/applog/svc/"); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	g, ok := MchLog.impl.(*graylogUDP)
	if !ok {
		t.Fatalf("expected *graylogUDP, got %T", MchLog.impl)
	}

	// Fecha o writer subjacente sem mexer no flag g.closed (replica a
	// situação em que a conexão UDP foi perdida, mas o transporte ainda
	// está ativo do ponto de vista do facade).
	if err := g.writer.Close(); err != nil {
		t.Fatalf("writer.Close: %v", err)
	}

	got := captureStderr(t, func() {
		MchLog.LogSubject("info", []byte(`{"message":"after close"}`), nil)
	})
	if !strings.Contains(got, "GELF UDP send failed") {
		t.Errorf("expected warn line on writer failure; stderr:\n%s", got)
	}
}

// TestLogTypeLogSubjectBeforeInitialize garante que o facade do V3
// (mchlogcorev3.MchLog) é seguro de usar antes de Initialize:
// LogSubject vira no-op e GetFileNameFromStreamName devolve "".
func TestLogTypeLogSubjectBeforeInitialize(t *testing.T) {
	t.Cleanup(resetConfig)
	resetConfig()

	// Garante que MchLog.impl está nil (pode ter sido populado por
	// um teste anterior se a ordem mudar).
	MchLog.mu.Lock()
	MchLog.impl = nil
	MchLog.mu.Unlock()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("LogSubject before Initialize panicked: %v", r)
		}
	}()
	MchLog.LogSubject("info", []byte(`{"message":"x"}`), nil) // no-op

	if got := MchLog.GetFileNameFromStreamName("info"); got != "" {
		t.Errorf("GetFileNameFromStreamName before Initialize = %q, want empty", got)
	}
	if err := MchLog.Close(); err != nil {
		t.Errorf("Close before Initialize = %v, want nil", err)
	}
}
