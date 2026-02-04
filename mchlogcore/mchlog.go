package mchlogcore

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/rs/zerolog"
)

const (
	ccLogFileSuffix       string = ".log"
	ccLogErrPrefixSubject string = "err_"
)

// LogType utiliza a biblioteca 'zerolog' para persistir log em arquivo de forma simples.
type LogType struct {
	path string
}

// MchLog é o objeto global de acesso.
var MchLog LogType

// InitializeMchLog apenas configura o caminho base e os formatos globais do zerolog.
func InitializeMchLog(path string) {
	zerolog.TimestampFieldName = "timestamp"
	zerolog.TimeFieldFormat = "2006-01-02 15:04:05"
	zerolog.TimestampFunc = func() time.Time {
		return time.Now().UTC()
	}

	MchLog.path = filepath.FromSlash(path)
}

// LogSubject escreve o log diretamente no arquivo <subject>/<subject>.log.
// Não mantém estado, não rotaciona e não usa mapas.
func (l *LogType) LogSubject(subject string, content any, errLog error, ascendStackFrame ...int) {
	if subject == "" {
		return
	}

	// Se for erro, adiciona o prefixo "err_" no subject (ex: err_error/err_error.log)
	if errLog != nil {
		subject = ccLogErrPrefixSubject + subject
	}

	// 1. Define o caminho: path/subject/subject.log
	filename := filepath.Join(l.path, subject, subject+ccLogFileSuffix)

	// 2. Garante que a pasta existe
	_ = os.MkdirAll(filepath.Dir(filename), 0755)

	// 3. Abre o arquivo em modo Append (cria se não existir)
	f, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return // Silencioso ou log no stderr
	}
	defer f.Close()

	// 4. Cria o logger pontual para este arquivo
	logger := zerolog.New(f).With().Timestamp().Logger()

	// 5. Formata o conteúdo
	event, err := l.getJSONLogger(&logger, content)
	if err != nil {
		return
	}

	skip := 1
	if len(ascendStackFrame) > 0 {
		skip = ascendStackFrame[0]
	}

	if errLog == nil {
		event.Send()
	} else {
		event.Caller(skip).Err(errLog).Send()
	}
}

// getJSONLogger (mantido para processar os tipos de dados via reflexão como antes)
func (l *LogType) getJSONLogger(logger *zerolog.Logger, content any) (*zerolog.Event, error) {
	var err error
	var event *zerolog.Event
	var ok bool

	v := reflect.ValueOf(content)
	switch v.Kind() {
	case reflect.Map:
		var m map[string]any
		if m, ok = content.(map[string]any); !ok {
			m = make(map[string]any)
			keys := v.MapKeys()
			for _, k := range keys {
				va := v.MapIndex(k)
				switch va.Kind() {
				case reflect.String:
					m[k.String()] = va.String()
				case reflect.Int, reflect.Int64, reflect.Int32:
					m[k.String()] = va.Int()
				case reflect.Float64, reflect.Float32:
					m[k.String()] = va.Float()
				}
			}
		}
		event = logger.Log().Fields(m)

	case reflect.Slice, reflect.Array:
		var arrb []byte
		if arrb, ok = content.([]byte); ok {
			var m map[string]any
			if err = json.Unmarshal(arrb, &m); err == nil {
				event = logger.Log().Fields(m)
			}
		} else {
			var arr []any
			if arr, ok = content.([]any); !ok {
				tam := v.Len()
				for i := 0; i < tam; i++ {
					va := v.Index(i)
					switch va.Kind() {
					case reflect.String:
						arr = append(arr, va.String())
					case reflect.Int, reflect.Int64, reflect.Int32:
						arr = append(arr, va.Int())
					case reflect.Float64, reflect.Float32:
						arr = append(arr, va.Float())
					}
				}
			}
			event = logger.Log().Fields(arr)
		}
	case reflect.String:
		s := content.(string)
		var m map[string]any
		if err = json.Unmarshal([]byte(s), &m); err == nil {
			event = logger.Log().Fields(m)
		}
	default:
		err = errors.New("tipo inválido")
	}

	return event, err
}

// GetFileNameFromStreamName mantido para compatibilidade com testes
func (l *LogType) GetFileNameFromStreamName(subject string) string {
	return filepath.Join(l.path, subject, subject+ccLogFileSuffix)
}
