// Package mchlogcorev3 é o backend unificado da toolkit. Suporta múltiplos
// protocolos selecionados por BackendConfig.Protocol:
//
//   - ProtocolFile: grava em arquivo no mesmo layout do mchlogcorev2
//     (<basePath>/<service>/<level>/<level>.log) e mesma JSON shape.
//   - ProtocolGraylogUDP: envia em formato GELF via UDP para o Graylog.
//
// Novos protocolos (graylog-tcp, syslog, splunk-hec, etc.) podem ser
// adicionados expondo novos valores de Protocol e a implementação
// correspondente; a API pública não muda.
package mchlogcorev3

import (
	"errors"
	"os"
	"sync"
)

// Protocol identifica o backend efetivo usado para persistir/enviar logs.
type Protocol string

const (
	// ProtocolFile grava logs em arquivo. Layout e JSON shape são os
	// mesmos do mchlogcorev2; o caller controla o caminho via
	// Logger.SetPath (ou usa o default /applog/).
	ProtocolFile Protocol = "file"

	// ProtocolGraylogUDP envia logs em formato GELF via UDP.
	ProtocolGraylogUDP Protocol = "graylog-udp"
)

// BackendConfig agrupa todos os parâmetros aceitos pelo V3. Os campos
// relevantes dependem de Protocol — campos de outros protocolos são
// ignorados pela validação.
type BackendConfig struct {
	// Protocol seleciona o backend. Default: ProtocolFile.
	Protocol Protocol

	// Addr é o endereço do destino no formato "host:porta".
	// Obrigatório quando Protocol = ProtocolGraylogUDP.
	Addr string

	// Source é o valor gravado no campo GELF "host" (coluna "source"
	// no Graylog). Obrigatório quando Protocol = ProtocolGraylogUDP.
	// Fornecido pelo serviço (a toolkit não autodetecta).
	Source string

	// DisableGZIP desabilita a compressão GZIP do GELF UDP. Default
	// (zero value) = GZIP habilitado. Aplica apenas a ProtocolGraylogUDP.
	DisableGZIP bool
}

var (
	cfgMu      sync.RWMutex
	activeCfg  BackendConfig
	configured bool
)

// Configure normaliza e armazena a configuração que será usada pelo
// backend. Aplica default a Protocol e valida os campos obrigatórios
// para o protocolo selecionado.
func Configure(cfg BackendConfig) error {
	if cfg.Protocol == "" {
		cfg.Protocol = ProtocolFile
	}

	switch cfg.Protocol {
	case ProtocolFile:
		// arquivo: nada obrigatório aqui; o path vem via Logger.SetPath
		// e o nome do serviço via NewLogger.
	case ProtocolGraylogUDP:
		if cfg.Addr == "" {
			return errors.New("mchlogcorev3: Addr is required for ProtocolGraylogUDP")
		}
		if cfg.Source == "" {
			return errors.New("mchlogcorev3: Source is required for ProtocolGraylogUDP (caller-provided)")
		}
	default:
		return errors.New("mchlogcorev3: unknown Protocol: " + string(cfg.Protocol))
	}

	cfgMu.Lock()
	activeCfg = cfg
	configured = true
	cfgMu.Unlock()
	return nil
}

// ActiveConfig retorna uma cópia da configuração ativa. Útil para
// testes e para o backend ler os parâmetros já normalizados.
// Antes de Configure ser chamado, devolve um BackendConfig zero-valued.
func ActiveConfig() BackendConfig {
	cfgMu.RLock()
	defer cfgMu.RUnlock()
	return activeCfg
}

// IsConfigured indica se Configure já foi chamado com sucesso.
func IsConfigured() bool {
	cfgMu.RLock()
	defer cfgMu.RUnlock()
	return configured
}

// DefaultSource é um helper para callers que não querem compor o Source
// manualmente. Devolve o hostname do sistema (os.Hostname) ou "unknown"
// caso a chamada falhe ou retorne string vazia. Útil apenas para
// ProtocolGraylogUDP.
func DefaultSource() string {
	if h, err := os.Hostname(); err == nil && h != "" {
		return h
	}
	return "unknown"
}

// resetConfig limpa o estado de configuração. Usado apenas em testes
// (não exportado).
func resetConfig() {
	cfgMu.Lock()
	activeCfg = BackendConfig{}
	configured = false
	cfgMu.Unlock()
}
