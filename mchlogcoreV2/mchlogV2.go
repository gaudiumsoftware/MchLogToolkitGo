package mchlogcoreV2

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

/* Exemplo de uso do mchlogcore.
// Primeiro o pacote precisa ser inicializado com o
// caminho a gravar os arquivos de log:
mchlogcore.InitializeMchLog("./etc/log")
// O pacote mchlogcore contém o objeto público MchLog e
// o único método acessível:  LogSubject()
// Origem do log é uma string ou []byte em formato de json (tem que ser um json válido)
m := fmt.Sprintf("{\"somekey\":{\"id\":\"123\"},\"tick\":%d,\"time\":\"2020-11-09T12:05:19+01:00\",\"testmsg\":\"some msg\"}", 10)
mchlogcore.MchLog.LogSubject("teste", m, nil)
ou
mchlogcore.MchLog.LogSubject("teste", []byte(m), nil)
// output: {"somekey":{"id":"123"},"testmsg":"some msg","tick":10,"time":"2020-11-09T12:05:19+01:00","data_hora":"2022-10-12 19:11:33 UTC"}
// no arquivo ./etc/log/teste/teste.log
// Origem do log é um mapa onde a chave é uma string e o tipo do valor "dinâmico" (interface{}).
// A chave TEM que ser string, o valor pode ser string, int ou float.
// O tipo do valor pode ser estático também (string, por exemplo).
h := make(map[string]any)
h["mapa1"] = "aaa"
h["mapa2"] = 22
mchlogcore.MchLog.LogSubject("teste", h, nil)
// output: {"mapa1":"aaa","mapa2":22,"data_hora":"2022-10-12 19:11:33 UTC"}
// no arquivo ./etc/log/teste/teste.log
// Origem do log é um array/slice de tipo dinâmico.
// O tipo do array/slice pode ser estático, nesse caso só pode ser string.
// Começando do indice zero, os elementos pares são as "chaves" e
// os ímpares os valores, que podem ser string, int ou float.
var a []any
a = append(a, "key_array1", 22, "key_array2")
a = append(a, "55")
mchlogcore.MchLog.LogSubject("teste", a, nil)
// output: {"key_array1":22,"key_array2":"55","data_hora":"2022-10-12 19:11:33 UTC"}
// no arquivo ./etc/log/teste/teste.log
// Usando o log do exemplo anterior, desta vez enviando um erro.
mchlogcore.MchLog.LogSubject("teste", a, errors.New("isto é um erro forçado"))
// output:  {"key_array1":22,"key_array2":"55","error":"isto é um erro forçado","data_hora":"2022-10-12 19:11:33 UTC"}
// no arquivo ./etc/log/err_teste/err_teste.log
*/

const (
	ccLogDataHora         string = "timestamp" //chave ref. ao timestamp, gravada automaticamente no json de log
	ccLogFileSuffix       string = ".log"      //sufixo do arquivo de log
	ccLogErrPrefixSubject string = "err_"      //prefixo do arquivo de log de erro
)

// LogType utiliza a biblioteca 'zerolog' para persistir log em arquivo.
type LogType struct {
	mapLogger sync.Map // cache de loggers para evitar abrir/fechar os arquivos de log repetidamente
	path      string
}

// MchLog é o objeto de acesso ao método LogSubject,
// que efetivamente escreve o log.
var MchLog LogType

// InitializeMchLog configura o diretório base e as regras de formatação do zerolog.
func InitializeMchLog(path string) {
	zerolog.TimestampFieldName = ccLogDataHora
	zerolog.TimeFieldFormat = "2006-01-02 15:04:05"
	zerolog.TimestampFunc = func() time.Time {
		return time.Now().UTC()
	}

	MchLog.path = filepath.FromSlash(path)
}

// LogSubject grava no arquivo de log o conteúdo (content) enviado no parâmetro, que pode ser um json, map ou array.
// Um subdiretório será criado com um arquivo de log dentro, ambos com o nome do subject: <subject>/<subject>.log
// Se o parâmetro errLog estiver preenchido, o subject receberá o prefix "err_", e consequentemente a pasta e o nome do arquivo correspondente.
// O descritivo do erro fará parte do json gravado no arquivo de log
// ascendStackFrame é um parâmetro opcional (default = 1) que indica onde procurar a linha de código que deu origem à mensagem de log, em caso de errLog!=nil.
func (l *LogType) LogSubject(subject string, content any, errLog error, ascendStackFrame ...int) {
	if subject == "" {
		return
	}

	// Se for erro, adiciona o prefixo "err_" no subject
	// Ex.: se subject for "init", cria err_init/err_init.log
	if errLog != nil {
		subject = ccLogErrPrefixSubject + subject
	}

	var logger *zerolog.Logger

	// Tenta recuperar o logger do cache (evita abrir o arquivo novamente)
	if obj, ok := l.mapLogger.Load(subject); ok {
		logger = obj.(*zerolog.Logger)
	} else {
		// Se não estiver no cache, abre o arquivo e inicializa o logger
		filename := filepath.Join(l.path, subject, subject+ccLogFileSuffix)

		// Garante que a árvore de diretórios existe
		_ = os.MkdirAll(filepath.Dir(filename), 0755)

		// Abre o arquivo para escrita ao final (append). Cria se não existir.
		f, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
		if err != nil {
			return
		}

		// Importante: Não fechamos o arquivo (f.Close) aqui pois ele será
		// gerenciado pelo objeto logger que ficará no cache durante a vida do processo.
		newLogger := zerolog.New(f).With().Timestamp().Logger()
		logger = &newLogger
		l.mapLogger.Store(subject, logger)
	}

	// Processa o conteúdo e gera o evento de log
	event, err := l.getJSONLogger(logger, content)
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

// getJSONLogger retorna o evento de log correspondente
// pronto para ser persistido no arquivo.
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
		err = errors.New("")
	}

	if err != nil {
		err = fmt.Errorf("tipo inválido do conteúdo do log: %v.\nEsperados map, json em formato de string ou []byte", v.Type())
	}

	return event, err
}

// GetFileNameFromStreamName mantido apenas para compatibilidade com os testes unitários
func (l *LogType) GetFileNameFromStreamName(subject string) string {
	return filepath.Join(l.path, subject, subject+ccLogFileSuffix)
}
