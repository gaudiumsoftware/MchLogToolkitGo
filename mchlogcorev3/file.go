package mchlogcorev3

import (
	"github.com/gaudiumsoftware/mchlogtoolkitgo/mchlogcorev2"
)

// fileDestination é a estratégia de arquivo do V3. Internamente delega
// para mchlogcorev2.MchLog, preservando integralmente o layout
// (<basePath>/<service>/<level>/<level>.log) e a JSON shape do V2.
//
// É um wrapper fino de propósito: permite ao V3 oferecer "mesmo
// comportamento do V2" sem duplicar código, abrindo o caminho para
// que serviços migrem de V2 para V3 sem alterar a operação. Quando
// V1/V2 forem removidos no futuro, a lógica do V2 pode ser inlineada
// aqui sem mudar a API pública do V3.
type fileDestination struct {
	inner *mchlogcorev2.LogType
}

// newFileDestination inicializa o V2 subjacente com o path recebido e
// devolve um wrapper pronto para uso.
func newFileDestination(path string) *fileDestination {
	mchlogcorev2.InitializeMchLog(path)
	return &fileDestination{inner: &mchlogcorev2.MchLog}
}

func (f *fileDestination) LogSubject(subject string, content any, errLog error, ascendStackFrame ...int) {
	if f == nil || f.inner == nil {
		return
	}
	f.inner.LogSubject(subject, content, errLog, ascendStackFrame...)
}

func (f *fileDestination) GetFileNameFromStreamName(subject string) string {
	if f == nil || f.inner == nil {
		return ""
	}
	return f.inner.GetFileNameFromStreamName(subject)
}

// Close é no-op porque mchlogcorev2.LogType não expõe Close (decisão
// prévia de não alterar V1/V2). O sistema operacional libera os FDs
// no encerramento do processo, comportamento idêntico ao uso direto
// de mchlogcorev2.
func (f *fileDestination) Close() error {
	return nil
}
