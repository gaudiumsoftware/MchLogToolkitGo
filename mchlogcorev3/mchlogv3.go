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
// no stderr quando o envio para o Graylog falha. Uma falha gera no
// máximo um aviso por janela.
const warnWindow = 60 * time.Second

// graylogUDP implementa a interface mchlogcore.Transport (e Closer)
// enviando logs em formato GELF via UDP para um servidor Graylog.
type graylogUDP struct {
	writer      *gelf.Writer
	cfg         NetworkConfig
	serviceName string

	mu       sync.Mutex
	closed   bool
	lastWarn time.Time
}

// MchLog é a instância global do transporte V3. É populada por
// Initialize. Antes disso, é nil.
var MchLog *graylogUDP

// Initialize prepara o transporte V3 para uso. O parâmetro path tem
// a mesma forma usada por V1/V2 ("<basePath>/<service>/") e o nome do
// serviço é extraído do último segmento. Configure precisa ter sido
// chamado antes; caso contrário, retorna erro.
func Initialize(path string) error {
	if !IsConfigured() {
		return errors.New("mchlogcorev3: Configure must be called before Initialize")
	}

	service := serviceFromPath(path)
	if service == "" {
		return errors.New("mchlogcorev3: cannot extract service name from path: " + path)
	}

	cfg := ActiveConfig()

	w, err := gelf.NewWriter(cfg.Addr)
	if err != nil {
		return fmt.Errorf("mchlogcorev3: dial GELF UDP %s: %w", cfg.Addr, err)
	}
	if cfg.DisableGZIP {
		w.CompressionType = gelf.CompressNone
	} else {
		w.CompressionType = gelf.CompressGzip
	}

	MchLog = &graylogUDP{
		writer:      w,
		cfg:         cfg,
		serviceName: service,
	}
	return nil
}

// LogSubject monta a mensagem GELF e envia via UDP. Falhas no envio
// são registradas via warnOnce e silenciadas para o caller.
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

	// ascendStackFrame é mantido na assinatura por contrato com o
	// facade; o caller pode usar para skip no zerolog. V3 já popula
	// _file/_line a partir do payload, então é silenciosamente ignorado.
	_ = ascendStackFrame
}

// GetFileNameFromStreamName devolve um descritor lógico do "fluxo" de
// log, no formato "udp://<addr>/<subject>". Usado apenas para
// observabilidade e compatibilidade com testes existentes.
func (g *graylogUDP) GetFileNameFromStreamName(subject string) string {
	if g == nil {
		return ""
	}
	return "udp://" + g.cfg.Addr + "/" + subject
}

// Close fecha o writer GELF. Idempotente.
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
