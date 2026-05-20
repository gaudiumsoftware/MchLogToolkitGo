// Package mchlogcorev3 é o destino unificado da toolkit. O V3 sempre
// roteia chamadas LogSubject por um routerDestination que combina dois
// impls: file (sempre presente) e network (opcional).
//
// Regra de roteamento (exata, case-sensitive):
//
//   - subjects "level-like" da toolkit (test, debug, info, warn, error,
//     fatal) → network impl, se configurado; caso contrário, file.
//   - subjects estendidos via DestinationConfig.NetworkSubjects → mesma
//     regra dos level-like.
//   - qualquer outro subject (eventos de domínio, ex.: historico_posicao_taxi,
//     log_posicao_alterada, etc.) → file impl.
//
// O file impl usa o mesmo layout do mchlogcorev2
// (<basePath>/<service>/<subject>/<subject>.log) e a mesma JSON shape.
//
// Atualmente o único tipo de network suportado é Graylog UDP (GELF).
// Novos transportes (graylog-tcp, syslog, splunk-hec, etc.) podem ser
// adicionados expondo novos valores de NetworkType e a implementação
// correspondente; a API pública (LogSubject) não muda.
package mchlogcorev3

import (
	"errors"
	"os"
	"sync"
)

// NetworkType identifica o transporte do impl de rede.
type NetworkType string

const (
	// NetworkGraylogUDP envia logs em formato GELF via UDP.
	NetworkGraylogUDP NetworkType = "graylog-udp"
)

// NetworkConfig descreve o destino de rede opcional. Quando presente em
// DestinationConfig, subjects "level-like" (e os explicitamente listados
// em NetworkSubjects) são enviados por aqui em vez de gravados em disco.
type NetworkConfig struct {
	// Type seleciona o transporte. Hoje só NetworkGraylogUDP.
	Type NetworkType

	// Addr é o endereço do destino no formato "host:porta".
	Addr string

	// Source é o valor gravado no campo GELF "host" (coluna "source"
	// no Graylog). Fornecido pelo serviço (a toolkit não autodetecta).
	Source string

	// DisableGZIP desabilita a compressão GZIP do GELF UDP. Default
	// (zero value) = GZIP habilitado.
	DisableGZIP bool
}

// DestinationConfig agrupa os parâmetros aceitos pelo V3.
//
// Zero value (Network==nil, NetworkSubjects==nil) configura o V3 em modo
// "file-only": todos os subjects vão para arquivo no mesmo layout do V2.
type DestinationConfig struct {
	// Network é opcional. Quando nil, todos os subjects vão para arquivo.
	// Quando definido, subjects level-like (e os listados em
	// NetworkSubjects) vão por aqui; o restante continua em arquivo.
	Network *NetworkConfig

	// NetworkSubjects estende a whitelist default de subjects roteados
	// para o network impl. A whitelist default é o conjunto fixo de
	// levels da toolkit (test, debug, info, warn, error, fatal).
	// Match é exato e case-sensitive. Strings vazias são ignoradas.
	//
	// Só faz sentido com Network != nil. Configure rejeita o contrário.
	NetworkSubjects []string
}

var (
	cfgMu      sync.RWMutex
	activeCfg  DestinationConfig
	configured bool
)

// Configure normaliza e armazena a configuração que será usada pelo
// destino. Valida os campos obrigatórios para o transporte de rede
// selecionado (quando presente).
//
// A NetworkConfig recebida é copiada antes do armazenamento, então o
// caller pode mutar/descartar a struct após o retorno.
func Configure(cfg DestinationConfig) error {
	if cfg.Network != nil {
		netCfg := *cfg.Network
		switch netCfg.Type {
		case "":
			return errors.New("mchlogcorev3: Network.Type is required when Network is set")
		case NetworkGraylogUDP:
			if netCfg.Addr == "" {
				return errors.New("mchlogcorev3: Network.Addr is required for NetworkGraylogUDP")
			}
			if netCfg.Source == "" {
				return errors.New("mchlogcorev3: Network.Source is required for NetworkGraylogUDP (caller-provided)")
			}
		default:
			return errors.New("mchlogcorev3: unknown Network.Type: " + string(netCfg.Type))
		}
		cfg.Network = &netCfg
	} else if len(cfg.NetworkSubjects) > 0 {
		return errors.New("mchlogcorev3: NetworkSubjects requires Network to be set")
	}

	if len(cfg.NetworkSubjects) > 0 {
		// Copia o slice para isolar mutações posteriores no caller.
		dup := make([]string, len(cfg.NetworkSubjects))
		copy(dup, cfg.NetworkSubjects)
		cfg.NetworkSubjects = dup
	}

	cfgMu.Lock()
	activeCfg = cfg
	configured = true
	cfgMu.Unlock()
	return nil
}

// ActiveConfig retorna uma cópia da configuração ativa. Útil para
// testes e para o destino ler os parâmetros já normalizados.
// Antes de Configure ser chamado, devolve um DestinationConfig zero-valued.
//
// Atenção: o ponteiro Network é compartilhado com a cópia interna.
// Callers que mutarem *ActiveConfig().Network corromperão o estado;
// trate-o como read-only.
func ActiveConfig() DestinationConfig {
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
// NetworkGraylogUDP.
func DefaultSource() string {
	if h, err := os.Hostname(); err == nil && h != "" {
		return h
	}
	return "unknown"
}

// resetConfig limpa o estado de configuração. Usado apenas em testes
// (não exportado).
//
// Os testes do pacote NÃO usam t.Parallel: activeCfg, MchLog.impl e o
// global de mchlogcorev2 são compartilhados, então rodar testes em
// paralelo causaria interferência. Cada teste chama
// t.Cleanup(resetConfig) para deixar o estado pronto para o próximo.
func resetConfig() {
	cfgMu.Lock()
	activeCfg = DestinationConfig{}
	configured = false
	cfgMu.Unlock()
}
