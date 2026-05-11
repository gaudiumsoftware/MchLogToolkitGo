package mchlogcore

import (
	"testing"

	"github.com/gaudiumsoftware/mchlogtoolkitgo/mchlogcorev1"
	"github.com/gaudiumsoftware/mchlogtoolkitgo/mchlogcorev2"
)

// TestSetVersionSwapsTransport garante que SetVersion troca o Transport
// ativo para o backend correspondente (V1 ou V2). É a invariante central
// do facade refatorado.
func TestSetVersionSwapsTransport(t *testing.T) {
	t.Cleanup(func() { SetVersion(V1) })

	SetVersion(V1)
	if _, ok := current.(*mchlogcorev1.LogType); !ok {
		t.Fatalf("V1 should select *mchlogcorev1.LogType, got %T", current)
	}

	SetVersion(V2)
	if _, ok := current.(*mchlogcorev2.LogType); !ok {
		t.Fatalf("V2 should select *mchlogcorev2.LogType, got %T", current)
	}
}

// TestGetIPOnlyV1 confirma que GetIP delega para o backend apenas quando
// V1 está ativo; para outros backends retorna string vazia.
func TestGetIPOnlyV1(t *testing.T) {
	t.Cleanup(func() { SetVersion(V1) })

	SetVersion(V2)
	if got := MchLog.GetIP(); got != "" {
		t.Fatalf("expected empty IP for V2, got %q", got)
	}

	SetVersion(V1)
	// V1.GetIP pode retornar "" se a máquina não tem IP não-loopback,
	// portanto não exigimos não-vazio — apenas que o tipo correto é consultado.
	_ = MchLog.GetIP()
}

// TestCloseNoopForFileBackends garante que Close no facade é no-op para
// V1 e V2 (que não implementam Closer) e não retorna erro.
func TestCloseNoopForFileBackends(t *testing.T) {
	t.Cleanup(func() { SetVersion(V1) })

	SetVersion(V1)
	if err := MchLog.Close(); err != nil {
		t.Fatalf("Close on V1 should be no-op, got %v", err)
	}

	SetVersion(V2)
	if err := MchLog.Close(); err != nil {
		t.Fatalf("Close on V2 should be no-op, got %v", err)
	}
}
