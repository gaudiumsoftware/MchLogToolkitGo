package mchlogcore

// Transport é a estratégia que efetivamente persiste ou envia os logs.
// Cada backend (arquivo V1, arquivo V2, rede V3, ...) implementa esta
// interface e é selecionado pelo facade através de SetVersion.
//
// A interface é declarada aqui no pacote do facade. Os pacotes de backend
// não precisam importá-la: como Go usa interfaces estruturais, basta que
// os métodos coincidam. Isso evita ciclo de import entre mchlogcore e os
// pacotes mchlogcoreV*.
type Transport interface {
	// LogSubject persiste/envia o conteúdo de log.
	// Mesma assinatura usada pelos backends V1 e V2.
	LogSubject(subject string, content any, errLog error, ascendStackFrame ...int)

	// GetFileNameFromStreamName devolve o caminho do arquivo (V1/V2)
	// ou um descritor lógico (ex.: "udp://host:port/<subject>") para
	// transportes de rede. Mantida por compatibilidade com testes existentes.
	GetFileNameFromStreamName(subject string) string
}

// Closer é uma interface opcional: backends que mantêm recursos
// que precisam ser liberados explicitamente (ex.: conexões de rede)
// implementam Close. O facade chama via type assertion, então
// backends que não precisam (V1, V2) não são forçados a implementar.
type Closer interface {
	// Close libera recursos do transporte.
	// Deve ser idempotente.
	Close() error
}
