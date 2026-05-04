package mchlogcore

import (
	"github.com/gaudiumsoftware/mchlogtoolkitgo/mchlogcorev1"
	"github.com/gaudiumsoftware/mchlogtoolkitgo/mchlogcorev2"
)

// Asserções de tempo de compilação garantindo que cada backend
// satisfaz a interface Transport.
var (
	_ Transport = (*mchlogcorev1.LogType)(nil)
	_ Transport = (*mchlogcorev2.LogType)(nil)
)

// LogVersion identifica qual backend de log está em uso.
type LogVersion int

const (
	// V1 — backend de arquivo, formato com IP e timestamp por hora.
	V1 LogVersion = iota
	// V2 — backend de arquivo, formato simples (um arquivo por subject).
	V2
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
// Centraliza o dispatch para que adicionar uma nova versão (ex.: V3 = rede)
// signifique apenas estender este switch.
func transportFor(v LogVersion) Transport {
	switch v {
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
// a interface Closer (somente backends que precisam de cleanup explícito,
// ex.: V3 sobre UDP). Para backends de arquivo (V1, V2) é no-op.
// Idempotência é responsabilidade do backend.
func (l *LogType) Close() error {
	if c, ok := current.(Closer); ok {
		return c.Close()
	}
	return nil
}

// MchLog é a instância global do facade.
var MchLog LogType

// InitializeMchLog inicializa o backend selecionado com o caminho dado.
// Para backends de arquivo (V1, V2) o path é o diretório base.
func InitializeMchLog(path string) {
	versionName := "V1"
	switch currentVersion {
	case V2:
		versionName = "V2"
		mchlogcorev2.InitializeMchLog(path)
	default:
		mchlogcorev1.InitializeMchLog(path)
	}

	// Primeiro log informa qual versão foi inicializada.
	MchLog.LogSubject("info", map[string]string{"message": "MchLogToolkit initialized", "version": versionName}, nil)
}
