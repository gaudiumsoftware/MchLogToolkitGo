package MchLogToolkitGo

import (
	"encoding/json"
	"errors"
	"gaudium.com.br/gaudiumsoftware/MchLogToolkitGo/mchlogcore"
	"path/filepath"
	"runtime"
	"strconv"
)

const (
	DebugLevel = "DEBUG"
	InfoLevel  = "INFO"
	WarnLevel  = "WARN"
	ErrorLevel = "ERROR"

	DebugPath = "./applog/"
	ProdPath  = "/applog/"
)

// Logger é a estrutura que encapsula as funcionalidades de log da aplicação
type Logger struct {
	log         mchlogcore.LogType
	path        string
	service     string
	level       string
	development bool
}

// NewLogger cria uma instância do logger para ser utilizado pela aplicação
// service: nome do serviço que está utilizando o logger
// level: nível de log que será utilizado (DEBUG, INFO, WARN, ERROR)
// Retorna um ponteiro para a instância do logger e um erro caso ocorra
func NewLogger(service, level string) (*Logger, error) {
	if service == "" {
		return nil, errors.New("service name is required")
	}

	if level == "" || level != DebugLevel && level != InfoLevel && level != WarnLevel && level != ErrorLevel {
		return nil, errors.New("level is invalid")
	}

	return &Logger{log: mchlogcore.MchLog, path: ProdPath, service: service, level: level}, nil
}

// Initialize inicializa o logger
func (l *Logger) Initialize() {
	mchlogcore.InitializeMchLog(filepath.Join(l.path, l.service))
}

// SetPath define o caminho onde os logs serão armazenados
// **deve ser chamado antes de chamar o método Initialize**.
func (l *Logger) SetPath(path string) {
	if path == "" {
		panic("path cannot be empty")
	}

	l.path = path
}

func (l *Logger) Debug(message string) {
	if l.level != DebugLevel {
		return
	}

	byteMessage := formatLog(message, DebugLevel)
	if byteMessage == nil {
		panic("error formatting log message")
	}

	l.log.LogSubject(DebugLevel, byteMessage, nil)
}

func (l *Logger) Warn(message string) {
	if l.level != WarnLevel && l.level != DebugLevel {
		return
	}

	byteMessage := formatLog(message, WarnLevel)
	if byteMessage == nil {
		panic("error formatting log message")
	}

	l.log.LogSubject(WarnLevel, byteMessage, nil)
}

func (l *Logger) Info(message string) {
	byteMessage := formatLog(message, InfoLevel)
	if byteMessage == nil {
		panic("error formatting log message")
	}

	l.log.LogSubject(InfoLevel, byteMessage, nil)
}

func (l *Logger) Error(message string) {
	byteMessage := formatLog(message, DebugLevel)
	if byteMessage == nil {
		panic("error formatting log message")
	}

	l.log.LogSubject(DebugLevel, byteMessage, nil)
}

func formatLog(message, level string) []byte {
	_, source, line, ok := runtime.Caller(2)
	if !ok {
		return nil
	}

	formatedMessage := map[string]string{
		"message": message,
		"level":   level,
		"source":  source,
		"line":    strconv.Itoa(line),
		"trace":   "",
	}

	result, err := json.Marshal(formatedMessage)
	if err != nil {
		return nil
	}

	return result
}
