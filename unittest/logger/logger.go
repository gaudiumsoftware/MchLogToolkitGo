// Package logger expõe uma instância única do logger da toolkit para ser
// compartilhada pelos testes unitários do repositório.
//
// Ele espelha o pacote `infra/logger` dos microsserviços que consomem a
// MchLogToolkit, de modo que o helper `unittest/util.go` seja o mesmo em
// todos os repositórios da Gaudium.
//
// Embora este arquivo não tenha o sufixo `_test`, ele não entra no código de
// produção enquanto nenhum arquivo de produção o importar.
package logger

import (
	"testing"

	"github.com/gaudiumsoftware/mchlogtoolkitgo"
)

// ServiceName é o nome do serviço usado pelo logger dos testes.
// Os logs são gravados em `<mchlogtoolkitgo.DebugPath>/<ServiceName>/`.
const ServiceName = "unittest"

// Level é o nível de log usado por InitLoggerMock.
// Deve ser definido antes da chamada de InitLoggerMock.
var Level string

// Logger é a instância compartilhada pelos testes, populada por InitLoggerMock.
var Logger *mchlogtoolkitgo.Logger

// InitLoggerMock inicializa o logger dos testes gravando em disco no diretório
// de debug, para que os logs possam ser conferidos com `unittest.CompareLogs`.
func InitLoggerMock(t *testing.T) {
	t.Helper()

	l, err := mchlogtoolkitgo.NewLogger(ServiceName, Level)
	if err != nil {
		t.Fatalf("Error creating the logger mock: %v", err)
	}

	l.SetPath(mchlogtoolkitgo.DebugPath)
	l.Initialize()

	Logger = l
}
