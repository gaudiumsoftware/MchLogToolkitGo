package mchlogcorev3

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Graylog2/go-gelf/gelf"
)

// warnWindow é a janela de rate-limit das mensagens de aviso impressas
// no stderr quando o envio falha. Uma falha gera no máximo um aviso
// por janela.
const warnWindow = 60 * time.Second

// destination é a estratégia interna do V3: implementações concretas
// (graylogUDP, fileDestination) atendem este contrato e são selecionadas
// por Protocol em Initialize.
type destination interface {
	LogSubject(subject string, content any, errLog error, ascendStackFrame ...int)
	GetFileNameFromStreamName(subject string) string
	Close() error
}

// LogType é o facade público do V3. Mantém uma estratégia interna
// (file ou rede) escolhida por Protocol e delega todas as chamadas.
// Satisfaz mchlogcore.Transport e mchlogcore.Closer.
type LogType struct {
	mu   sync.RWMutex
	impl destination
}

// MchLog é a instância global do V3. É populada por Initialize.
// Antes de Initialize a estratégia interna é nil; chamadas via
// Transport permanecem seguras (early-return).
var MchLog LogType

// LogSubject delega para a estratégia ativa. Se ainda não houve
// Initialize, é no-op.
func (l *LogType) LogSubject(subject string, content any, errLog error, ascendStackFrame ...int) {
	l.mu.RLock()
	impl := l.impl
	l.mu.RUnlock()
	if impl == nil {
		return
	}
	impl.LogSubject(subject, content, errLog, ascendStackFrame...)
}

// GetFileNameFromStreamName delega para a estratégia ativa. Devolve
// "" antes de Initialize.
func (l *LogType) GetFileNameFromStreamName(subject string) string {
	l.mu.RLock()
	impl := l.impl
	l.mu.RUnlock()
	if impl == nil {
		return ""
	}
	return impl.GetFileNameFromStreamName(subject)
}

// Close fecha a estratégia ativa. Idempotente: chamadas repetidas
// retornam nil. Após Close, LogSubject torna a ser no-op.
func (l *LogType) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.impl == nil {
		return nil
	}
	err := l.impl.Close()
	l.impl = nil
	return err
}

// Initialize prepara o V3 conforme a configuração ativa. O parâmetro
// path tem a mesma forma usada por V1/V2 ("<basePath>/<service>/")
// e o nome do serviço é extraído do último segmento. Configure precisa
// ter sido chamado antes; caso contrário, retorna erro.
func Initialize(path string) error {
	if !IsConfigured() {
		return errors.New("mchlogcorev3: Configure must be called before Initialize")
	}

	service := serviceFromPath(path)
	if service == "" {
		return errors.New("mchlogcorev3: cannot extract service name from path: " + path)
	}

	cfg := ActiveConfig()

	var impl destination
	switch cfg.Protocol {
	case ProtocolFile:
		impl = newFileDestination(path)
	case ProtocolGraylogUDP:
		w, err := gelf.NewWriter(cfg.Addr)
		if err != nil {
			return fmt.Errorf("mchlogcorev3: dial GELF UDP %s: %w", cfg.Addr, err)
		}
		if cfg.DisableGZIP {
			w.CompressionType = gelf.CompressNone
		} else {
			w.CompressionType = gelf.CompressGzip
		}
		impl = &graylogUDP{
			writer:      w,
			cfg:         cfg,
			serviceName: service,
		}
	default:
		return errors.New("mchlogcorev3: unsupported Protocol: " + string(cfg.Protocol))
	}

	MchLog.mu.Lock()
	MchLog.impl = impl
	MchLog.mu.Unlock()
	return nil
}

// graylogUDP envia logs em formato GELF via UDP.
type graylogUDP struct {
	writer      *gelf.Writer
	cfg         DestinationConfig
	serviceName string

	mu       sync.Mutex
	closed   bool
	lastWarn time.Time
}

func (g *graylogUDP) LogSubject(subject string, content any, errLog error, ascendStackFrame ...int) {
	if g == nil || subject == "" {
		return
	}

	msg, err := buildGELFMessage(g.serviceName, subject, content, errLog, g.cfg)
	if err != nil {
		g.warnOnce(err)
		return
	}

	if err := g.writer.WriteMessage(msg); err != nil {
		g.warnOnce(err)
	}

	_ = ascendStackFrame
}

func (g *graylogUDP) GetFileNameFromStreamName(subject string) string {
	if g == nil {
		return ""
	}
	return "udp://" + g.cfg.Addr + "/" + subject
}

func (g *graylogUDP) Close() error {
	if g == nil {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return nil
	}
	g.closed = true
	if g.writer != nil {
		_ = g.writer.Close()
	}
	return nil
}

// warnOnce imprime um aviso no stderr respeitando warnWindow:
// no máximo uma linha por janela quando os envios falham em sequência.
func (g *graylogUDP) warnOnce(err error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now()
	if now.Sub(g.lastWarn) < warnWindow {
		return
	}
	g.lastWarn = now
	fmt.Fprintf(os.Stderr, "mchlogcorev3: GELF UDP send failed: %v\n", err)
}

// serviceFromPath extrai o nome do serviço de um path no formato
// "<basePath>/<service>/" (ou variações com separadores Windows).
func serviceFromPath(path string) string {
	p := strings.TrimRight(filepath.ToSlash(path), "/ ")
	if p == "" {
		return ""
	}
	return filepath.Base(p)
}
