package mchlogcorev3

// defaultLevelSubjects são os subjects "level-like" emitidos pelo Logger
// da toolkit (Test/Debug/Info/Warn/Error/Fatal). São sempre roteados
// para o network impl quando este está configurado. Subjects fora desta
// lista (eventos de domínio) vão para o file impl.
//
// Match é case-sensitive: o Logger interno sempre emite o level em
// lowercase, então `LogSubject("INFO", ...)` cai em file (não bate na
// whitelist). Para estender, use DestinationConfig.NetworkSubjects.
var defaultLevelSubjects = []string{
	"test", "debug", "info", "warn", "error", "fatal",
}

// routerDestination roteia chamadas LogSubject por subject:
//
//   - subjects em whitelist  → network impl (se configurado)
//   - qualquer outro subject → file impl
//
// File impl é sempre presente; network impl é opcional. Quando network
// é nil, todos os subjects vão para file — comportamento idêntico ao
// V2.
//
// A whitelist é construída em newRouterDestination e é read-only após
// isso, então lookups não precisam de lock.
type routerDestination struct {
	file      destination
	network   destination
	whitelist map[string]struct{}
}

// newRouterDestination compõe a whitelist (defaults ∪ extra) e devolve
// o router. file deve ser não-nil; network pode ser nil.
func newRouterDestination(file, network destination, extra []string) *routerDestination {
	wl := make(map[string]struct{}, len(defaultLevelSubjects)+len(extra))
	for _, s := range defaultLevelSubjects {
		wl[s] = struct{}{}
	}
	for _, s := range extra {
		if s == "" {
			continue
		}
		wl[s] = struct{}{}
	}
	return &routerDestination{
		file:      file,
		network:   network,
		whitelist: wl,
	}
}

// shouldNetwork devolve true quando o subject deve ser enviado pelo
// network impl. Falso quando network==nil ou subject está fora da
// whitelist.
func (r *routerDestination) shouldNetwork(subject string) bool {
	if r == nil || r.network == nil {
		return false
	}
	_, ok := r.whitelist[subject]
	return ok
}

// LogSubject roteia para network ou file conforme a whitelist. Subject
// vazio é no-op (preserva guard prévio do graylogUDP e evita criar
// arquivos com nome vazio no fileDestination).
func (r *routerDestination) LogSubject(subject string, content any, errLog error, ascendStackFrame ...int) {
	if r == nil || subject == "" {
		return
	}
	if r.shouldNetwork(subject) {
		r.network.LogSubject(subject, content, errLog, ascendStackFrame...)
		return
	}
	r.file.LogSubject(subject, content, errLog, ascendStackFrame...)
}

// GetFileNameFromStreamName devolve o descritor do impl que receberia
// um LogSubject para o subject informado. Para subjects roteados ao
// network, é "udp://<addr>/<subject>" (devolvido pelo graylogUDP).
// Para os demais, é o caminho real do arquivo no disco (devolvido pelo
// fileDestination / V2).
func (r *routerDestination) GetFileNameFromStreamName(subject string) string {
	if r == nil || subject == "" {
		return ""
	}
	if r.shouldNetwork(subject) {
		return r.network.GetFileNameFromStreamName(subject)
	}
	return r.file.GetFileNameFromStreamName(subject)
}

// Close fecha ambos os impls. Retorna o primeiro erro encontrado, mas
// continua fechando o outro impl mesmo se um deles falhar. Idempotente
// na prática: graylogUDP.Close e fileDestination.Close já são idempotentes,
// então chamadas repetidas via LogType.Close (que zera MchLog.impl) ou
// re-entrada não disparam erros.
func (r *routerDestination) Close() error {
	if r == nil {
		return nil
	}
	var firstErr error
	if r.network != nil {
		if err := r.network.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if r.file != nil {
		if err := r.file.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
