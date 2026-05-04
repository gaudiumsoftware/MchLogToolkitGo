// Package mchlogcorev3 implementa o backend de rede da toolkit.
// Atualmente entrega logs via GELF UDP para o Graylog. Outros protocolos
// (graylog-tcp, syslog, splunk-hec, etc.) podem ser adicionados expondo
// novos valores de Protocol e a implementação correspondente.
package mchlogcorev3

import (
	"errors"
	"os"
	"sync"
)

// Protocol identifica o protocolo de rede usado para enviar logs.
// Adicionar um novo protocolo significa criar uma nova constante e
// estender o dispatch interno; o resto da API pública não muda.
type Protocol string

const (
	// ProtocolGraylogUDP envia logs em formato GELF via UDP.
	ProtocolGraylogUDP Protocol = "graylog-udp"
)

// NetworkConfig agrupa parâmetros de transporte de rede.
// Addr e Source são obrigatórios. Source é fornecido pelo serviço
// consumidor (tipicamente o nome do pod ou uma composição como
// "<service>-<env>-<pod>"); a toolkit não tenta autodetectar.
type NetworkConfig struct {
	// Protocol é o protocolo de rede. Default: ProtocolGraylogUDP.
	Protocol Protocol
	// Addr é o endereço do destino no formato "host:porta". Obrigatório.
	Addr string
	// Source é o valor que será gravado no campo GELF "host"
	// (a coluna "source" no Graylog). Obrigatório, fornecido pelo caller.
	Source string
	// DisableGZIP desabilita a compressão GZIP do GELF UDP.
	// Por padrão (zero value), GZIP fica habilitado.
	DisableGZIP bool
}

var (
	cfgMu      sync.RWMutex
	activeCfg  NetworkConfig
	configured bool
)

// Configure normaliza e armazena a configuração de rede que será usada
// pelo transporte. Aplica defaults a Protocol; valida campos obrigatórios.
// Retorna erro se Addr ou Source estiverem vazios, ou se Protocol for
// desconhecido.
func Configure(cfg NetworkConfig) error {
	if cfg.Addr == "" {
		return errors.New("mchlogcorev3: Addr is required")
	}
	if cfg.Source == "" {
		return errors.New("mchlogcorev3: Source is required (caller-provided)")
	}

	if cfg.Protocol == "" {
		cfg.Protocol = ProtocolGraylogUDP
	}
	switch cfg.Protocol {
	case ProtocolGraylogUDP:
		// suportado
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
// testes e para o transporte ler os parâmetros já normalizados.
// Antes de Configure ser chamado, devolve um NetworkConfig zero-valued.
func ActiveConfig() NetworkConfig {
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
// caso a chamada falhe ou retorne string vazia.
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
	activeCfg = NetworkConfig{}
	configured = false
	cfgMu.Unlock()
}
