package mchlogcore

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
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
// no arquivo ./etc/log/teste/teste-169.254.215.202-2022101222.log
// Origem do log é um mapa onde a chave é uma string e o tipo do valor "dinâmico" (interface{}).
// A chave TEM que ser string, o valor pode ser string, int ou float.
// O tipo do valor pode ser estático também (string, por exemplo).
h := make(map[string]any)
h["mapa1"] = "aaa"
h["mapa2"] = 22
mchlogcore.MchLog.LogSubject("teste", h, nil)
// output: {"mapa1":"aaa","mapa2":22,"data_hora":"2022-10-12 19:11:33 UTC"}
// no arquivo ./etc/log/teste/teste-169.254.215.202-2022101222.log
// Origem do log é um array/slice de tipo dinâmico.
// O tipo do array/slice pode ser estático, nesse caso só pode ser string.
// Começando do indice zero, os elementos pares são as "chaves" e
// os ímpares os valores, que podem ser string, int ou float.
var a []any
a = append(a, "key_array1", 22, "key_array2")
a = append(a, "55")
mchlogcore.MchLog.LogSubject("teste", a, nil)
// output: {"key_array1":22,"key_array2":"55","data_hora":"2022-10-12 19:11:33 UTC"}
// no arquivo ./etc/log/teste/teste-169.254.215.202-2022101222.log
// Usando o log do exemplo anterior, desta vez enviando um erro.
mchlogcore.MchLog.LogSubject("teste", a, errors.New("isto é um erro forçado"))
// output:  {"key_array1":22,"key_array2":"55","error":"isto é um erro forçado","data_hora":"2022-10-12 19:11:33 UTC"}
// no arquivo ./etc/log/err_teste/err_teste-169.254.215.202-2022101222.log
*/

const (
	ccLogDataHora         string = "data_hora"  //chave ref. ao timestamp, gravada automaticamente no json de log
	ccDateTimeMask        string = "2006010215" //YYYYMMDDHH - timestamp que define a rotação dos arquivos de log
	ccLogFileSuffix       string = ".log"       //sufixo do arquivo de log
	ccLogErrPrefixSubject string = "err_"       //prefixo do arquivo de log de erro
)

// LogType utiliza a biblioteca 'zerolog' para persistir log em arquivo.
type LogType struct {
	//key = subject; value = obj do tipo fileLogType
	mapLogger sync.Map
	path      string
	ip        string
}

type fileLogType struct {
	filename string
	file     *os.File
	err      error
	logger   *zerolog.Logger
}

// closeFile tenta fechar o arquivo de instrumentação, caso esteja aberto
func (flt *fileLogType) closeFile() {
	if flt.file != nil {
		_ = flt.file.Close()
	}
}

func (flt *fileLogType) mkDir() error {
	var err error
	dirName := filepath.Dir(flt.filename)
	if _, err = os.Stat(dirName); os.IsNotExist(err) {
		err = os.MkdirAll(dirName, os.ModePerm)
	}
	return err
}

// fileStreamType é usado na comunicação por canal
type fileStreamType struct {
	filename string
	subject  string
	chReturn chan fileLogType
}

var _chLog chan fileStreamType

// MchLog é o objeto de acesso ao método LogSubject,
// que efetivamente escreve o log.
var MchLog LogType

// InitializeMchLog inicia os procedimentos para persistir
// logs em arquivo pelo objeto MchLog.
func InitializeMchLog(path string) {
	_chLog = make(chan fileStreamType)
	zerolog.TimestampFieldName = ccLogDataHora
	zerolog.TimeFieldFormat = "2006-01-02 15:04:05"

	zerolog.TimestampFunc = func() time.Time {
		loc, _ := time.LoadLocation("UTC")
		return time.Now().In(loc)
	}

	MchLog.path = filepath.FromSlash(path)
	MchLog.ip = getLocalIP()

	go func() {
		for filestream := range _chLog {
			var fileLog fileLogType
			var ok bool
			var obj any

			if obj, ok = MchLog.mapLogger.Load(filestream.subject); ok {
				fileLog = obj.(fileLogType)
			}

			// se o registro não foi encontrado no mapa de controle ou
			// o nome do arquivo mudou, ele será gerado e registrado no mapa.
			if !ok || (filestream.filename != fileLog.filename) {
				fileLog.closeFile() // se houver arquivo aberto ele será fechado.
				fileLog = fileLogType{filename: filestream.filename}

				// se não houver diretório, que seja criado.
				// se o diretório existir nada faz e não retorna erro.
				if fileLog.err = fileLog.mkDir(); fileLog.err == nil {
					// criar arquivo de log com o nome passado pelo canal
					fileLog.file, fileLog.err = os.OpenFile(filestream.filename, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
					if fileLog.err == nil {
						logg := log.With().Logger().Output(fileLog.file)
						fileLog.logger = &logg
						MchLog.mapLogger.Store(filestream.subject, fileLog)
					}
				}
			}

			filestream.chReturn <- fileLog
		}
	}()
}

// LogSubject grava no arquivo de log o conteúdo (content)
// enviado no parâmetro, que pode ser um json, map ou array.
// Um subdiretório será criado com o nome do subject e parte
// do nome do arquivo também conterá o parâmetro subject.
// Se o parâmetro errLog estiver preenchido, o subject
// receberá o prefix "err_", consequentemente a pasta e
// o nome do arquivo correspondente. O descritivo do erro
// fará parte do json gravado no arquivo de log
// ascendStackFrame é um parâmetro opcional (default = 1)
// que indica onde procurar a linha de código que deu origem
// à mensagem de log, em caso de errLog!=nil.
func (l *LogType) LogSubject(subject string, content any, errLog error, ascendStackFrame ...int) {
	if subject == "" {
		return
	}

	var err error
	var logger *zerolog.Logger
	var event *zerolog.Event
	var skip = 1

	// o subject de um log de erro recebe um prefixo correspondente
	if errLog != nil {
		subject = ccLogErrPrefixSubject + subject
	}

	if len(ascendStackFrame) > 0 {
		skip = ascendStackFrame[0]
	}

	if logger, err = l.checkFile(subject); err == nil {
		if event, err = l.getJSONLogger(logger, content); err == nil {
			if errLog == nil {
				event.Send()
			} else {
				event.Caller(skip).Err(errLog).Send()
			}
		}
	}

	if err != nil { // mostrar log de erro na saída default do zerolog (console)
		log.Log().Msg("LogType.LogSubject: " + subject + " - " + err.Error())
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

// checkFile trata da rotação dos arquivos de log, dependendo
// da máscara do timestamp definido na constante ccDateTimeMask.
func (l *LogType) checkFile(subject string) (*zerolog.Logger, error) {
	var flt fileLogType

	filename := l.GetFileNameFromStreamName(subject)
	obj, ok := l.mapLogger.Load(subject)

	if ok {
		flt = obj.(fileLogType)
	}

	if !ok || (filename != flt.filename) {
		//criando canal de retorno dinamicamente e passando
		//para a estrutura que será enviada para o canal de
		//manipulação do arquivo
		chret := make(chan fileLogType)
		_chLog <- fileStreamType{filename: filename, subject: subject, chReturn: chret}
		flt = <-chret
	}

	return flt.logger, flt.err
}

// getFileNameFromStreamName monta o diretório completo com o nome do arquivo
func (l *LogType) GetFileNameFromStreamName(subject string) string {
	loc, _ := time.LoadLocation("UTC")
	dataHora := time.Now().In(loc).Format(ccDateTimeMask)

	ip := l.ip
	if ip != "" {
		ip = "-" + ip
	}

	logFile := filepath.Join(l.path, subject, subject+ip+"-"+dataHora+ccLogFileSuffix)
	return filepath.FromSlash(logFile)
}

// GetIP retorna o IP onde o log está rodando
func (l *LogType) GetIP() string {
	return l.ip
}

// getLocalIP retorna o endereço local do IP, desconsiderando o loopback
func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, address := range addrs {
		// check the address type and if it is not a loopback the display it
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return ""
}
