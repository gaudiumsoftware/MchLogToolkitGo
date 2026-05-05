package mchlogcore

import (
	"fmt"
	"os"

	"github.com/gaudiumsoftware/mchlogtoolkitgo/mchlogcorev1"
	"github.com/gaudiumsoftware/mchlogtoolkitgo/mchlogcorev2"
	"github.com/gaudiumsoftware/mchlogtoolkitgo/mchlogcorev3"
)

// Asserções de tempo de compilação garantindo que cada destino
// satisfaz a interface Transport (e Closer, quando aplicável).
var (
	_ Transport = (*mchlogcorev1.LogType)(nil)
	_ Transport = (*mchlogcorev2.LogType)(nil)
	_ Transport = (*mchlogcorev3.LogType)(nil)
	_ Closer    = (*mchlogcorev3.LogType)(nil)
)

// LogVersion identifica qual destino de log está em uso.
type LogVersion int

const (
	// V1 — destino de arquivo, formato com IP e timestamp por hora.
	V1 LogVersion = iota
	// V2 — destino de arquivo, formato simples (um arquivo por subject).
	V2
	// V3 — destino unificado. Suporta arquivo (mesmo layout do V2) e
	// rede (GELF UDP) selecionados via mchlogcorev3.DestinationConfig.Protocol.
	// Outros protocolos (graylog-tcp, syslog, splunk-hec, ...) podem ser
	// adicionados sem bumpar o enum.
	V3
)

var (
	currentVersion = V1
	// current aponta para o Transport efetivamente em uso. É populado
	// por SetVersion (e, na primeira chamada, por init).
	current Transport
)

func init() {
	current = transportFor(currentVersion)
}

// transportFor mapeia uma LogVersion para o Transport correspondente.
// Centraliza o dispatch: adicionar uma nova versão significa estender
// apenas este switch.
//
// Para V3, devolvemos &mchlogcorev3.MchLog. O facade interno do V3
// (LogType) tolera estado pré-Initialize (early-return em LogSubject /
// GetFileNameFromStreamName / Close), então chamadas antes de
// InitializeMchLog não panicam.
func transportFor(v LogVersion) Transport {
	switch v {
	case V3:
		return &mchlogcorev3.MchLog
	case V2:
		return &mchlogcorev2.MchLog
	default:
		return &mchlogcorev1.MchLog
	}
}

// SetVersion escolhe qual versão (V1 ou V2) será usada.
// Deve ser chamado antes de InitializeMchLog.
func SetVersion(v LogVersion) {
	currentVersion = v
	current = transportFor(v)
}

// LogType é o facade que delega ao Transport ativo.
type LogType struct{}

// LogSubject delega para o transporte ativo.
func (l *LogType) LogSubject(subject string, content any, errLog error, ascendStackFrame ...int) {
	current.LogSubject(subject, content, errLog, ascendStackFrame...)
}

// GetFileNameFromStreamName delega para o transporte ativo.
func (l *LogType) GetFileNameFromStreamName(subject string) string {
	return current.GetFileNameFromStreamName(subject)
}

// GetIP retorna o IP onde o log está rodando, quando o transporte ativo
// expõe essa informação (V1). Para outros transportes retorna "".
func (l *LogType) GetIP() string {
	if v1, ok := current.(*mchlogcorev1.LogType); ok {
		return v1.GetIP()
	}
	return ""
}

// Close libera recursos do transporte ativo, quando ele implementa
// a interface Closer (somente destinos que precisam de cleanup explícito,
// ex.: V3 sobre UDP). Para destinos de arquivo (V1, V2) é no-op.
// Idempotência é responsabilidade do destino.
func (l *LogType) Close() error {
	if c, ok := current.(Closer); ok {
		return c.Close()
	}
	return nil
}

// MchLog é a instância global do facade.
var MchLog LogType

// InitializeMchLog inicializa o destino selecionado com o caminho dado.
// Em todos os destinos o path tem a forma "<basePath>/<service>/":
//   - V1, V2 e V3-ProtocolFile usam o caminho como diretório base de arquivos.
//   - V3-ProtocolGraylogUDP usa o último segmento apenas para extrair
//     o nome do serviço; o destino real é cfg.Addr.
func InitializeMchLog(path string) {
	var versionName string
	var initErr error

	switch currentVersion {
	case V3:
		versionName = "V3"
		initErr = mchlogcorev3.Initialize(path)
	case V2:
		versionName = "V2"
		mchlogcorev2.InitializeMchLog(path)
	default:
		versionName = "V1"
		mchlogcorev1.InitializeMchLog(path)
	}

	// Re-vincula current ao transporte agora inicializado. Necessário
	// para V3, cuja global passou de nil para o ponteiro válido.
	current = transportFor(currentVersion)

	if initErr != nil {
		fmt.Fprintf(os.Stderr, "mchlogcore: %s initialization failed: %v\n", versionName, initErr)
		return
	}

	// Primeiro log informa qual versão foi inicializada.
	MchLog.LogSubject("info", map[string]string{"message": "MchLogToolkit initialized", "version": versionName}, nil)
}
