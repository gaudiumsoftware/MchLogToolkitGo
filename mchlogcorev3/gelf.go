package mchlogcorev3

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/Graylog2/go-gelf/gelf"
)

// handledPayloadKeys são as chaves do payload que o builder já trata
// explicitamente (Short, _file, _line, _trace) e portanto devem ser
// puladas no fan-out genérico de Extra.
//
// Observação: campos GELF top-level (version, host, short_message,
// timestamp, level, facility) NÃO são listados aqui porque o GELF spec
// distingue top-level "x" de custom field "_x" — então uma chave
// "version" no payload pode virar "_version" em Extra sem conflito.
var handledPayloadKeys = map[string]struct{}{
	"message": {},
	"source":  {},
	"line":    {},
	"trace":   {},
}

// levelToSyslog converte um level da toolkit ("debug", "info", ...) para
// o severity numérico do GELF (que segue syslog: 0..7).
// Levels desconhecidos são tratados como info (6).
func levelToSyslog(level string) int32 {
	switch level {
	case "fatal":
		return gelf.LOG_CRIT // 2
	case "error":
		return gelf.LOG_ERR // 3
	case "warn":
		return gelf.LOG_WARNING // 4
	case "info":
		return gelf.LOG_INFO // 6
	case "debug", "test":
		return gelf.LOG_DEBUG // 7
	default:
		return gelf.LOG_INFO
	}
}

// buildGELFMessage monta um *gelf.Message a partir do payload recebido
// pelo Transport. Aceita content como []byte JSON, string JSON, ou
// map[string]any / map[string]string.
//
// Convenções de campos:
//   - Short = chave "message" do payload (ou string vazia se ausente)
//   - Host  = cfg.Source
//   - Level = mapping syslog do parâmetro level
//   - Extra:
//   - _application_name = serviceName
//   - _log_id           = "<serviceName>-mchlog-<level>"
//   - _level_name       = level
//   - _file             = chave "source" do payload (renomeada para evitar
//     colidir com a coluna "source" do Graylog)
//   - _line             = chave "line"
//   - _trace            = chave "trace"
//   - demais chaves     = prefixadas com "_" (a menos que reservadas)
//   - _error            = errLog.Error() quando errLog != nil
func buildGELFMessage(serviceName, level string, content any, errLog error, cfg NetworkConfig) (*gelf.Message, error) {
	fields, err := contentToMap(content)
	if err != nil {
		return nil, err
	}

	msg := &gelf.Message{
		Version:  "1.1",
		Host:     cfg.Source,
		TimeUnix: float64(time.Now().UTC().UnixNano()) / float64(time.Second),
		Level:    levelToSyslog(level),
		Extra:    make(map[string]any),
	}

	// Short = "message" do payload, se presente.
	if v, ok := fields["message"]; ok {
		if s, ok := v.(string); ok {
			msg.Short = s
		} else {
			msg.Short = fmt.Sprintf("%v", v)
		}
	}

	// Custom fields fixos (sempre presentes).
	msg.Extra["_application_name"] = serviceName
	msg.Extra["_log_id"] = serviceName + "-mchlog-" + level
	msg.Extra["_level_name"] = level

	// Renomeio de source → _file para evitar colisão com a coluna
	// "source" do Graylog (que vem de Host).
	if v, ok := fields["source"]; ok {
		msg.Extra["_file"] = stringify(v)
	}
	if v, ok := fields["line"]; ok {
		msg.Extra["_line"] = stringify(v)
	}
	if v, ok := fields["trace"]; ok {
		msg.Extra["_trace"] = stringify(v)
	}

	// Demais chaves do payload viram custom fields prefixados com "_".
	// Pula as que já foram tratadas explicitamente (message → Short;
	// source/line/trace → _file/_line/_trace).
	for k, v := range fields {
		if _, handled := handledPayloadKeys[k]; handled {
			continue
		}
		msg.Extra["_"+k] = v
	}

	// Erro associado.
	if errLog != nil {
		msg.Extra["_error"] = errLog.Error()
	}

	return msg, nil
}

// contentToMap converte os formatos de content suportados (mapas, slice
// de bytes contendo JSON, string contendo JSON) para map[string]any.
func contentToMap(content any) (map[string]any, error) {
	if content == nil {
		return nil, errors.New("mchlogcorev3: nil content")
	}

	switch v := content.(type) {
	case map[string]any:
		return v, nil
	case map[string]string:
		out := make(map[string]any, len(v))
		for k, s := range v {
			out[k] = s
		}
		return out, nil
	case []byte:
		var out map[string]any
		if err := json.Unmarshal(v, &out); err != nil {
			return nil, err
		}
		return out, nil
	case string:
		var out map[string]any
		if err := json.Unmarshal([]byte(v), &out); err != nil {
			return nil, err
		}
		return out, nil
	}

	// Fallback via reflect para mapas com value type estático que não
	// caíram nos casos acima (ex.: map[string]int).
	rv := reflect.ValueOf(content)
	if rv.Kind() == reflect.Map && rv.Type().Key().Kind() == reflect.String {
		out := make(map[string]any, rv.Len())
		iter := rv.MapRange()
		for iter.Next() {
			out[iter.Key().String()] = iter.Value().Interface()
		}
		return out, nil
	}

	return nil, fmt.Errorf("mchlogcorev3: unsupported content type %T", content)
}

// stringify converte qualquer valor a string, com fast-path para string.
func stringify(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}
