package mchlogtoolkitgo

import (
	"encoding/json"
	"errors"
	"runtime"
	"strconv"
	"strings"

	"github.com/gaudiumsoftware/mchlogtoolkitgo/mchlogcore"
)

const (
	TestLevel  = "test"
	DebugLevel = "debug"
	InfoLevel  = "info"
	WarnLevel  = "warn"
	ErrorLevel = "error"
	FatalLevel = "fatal"

	DebugPath = "./applog/"
	ProdPath  = "/applog/"
)

// Logger é a estrutura que encapsula as funcionalidades de log da aplicação
type Logger struct {
	log         *mchlogcore.LogType
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
	l := &Logger{log: nil, path: ProdPath}

	if service == "" {
		return nil, errors.New("service name is required")
	}

	l.service = service
	err := l.SetLevel(level)
	if err != nil {
		return nil, err
	}
	return l, nil
}

// Initialize inicializa o logger
func (l *Logger) Initialize() {
	mchlogcore.InitializeMchLog(l.path + l.service + "/")
	l.log = &mchlogcore.MchLog
}

// SetPath define o caminho onde os logs serão armazenados
// **deve ser chamado antes de chamar o método Initialize**.
func (l *Logger) SetPath(path string) {
	if path == "" {
		panic("path cannot be empty")
	}

	l.path = path
}

// SetLevel define o nível de log que será utilizado
// level: nível de log que será utilizado (DEBUG, INFO, WARN, ERROR)
// Retorna um erro caso o nível de log seja inválido
func (l *Logger) SetLevel(level string) error {
	level = strings.ToLower(level)

	if level == "" || level != DebugLevel && level != InfoLevel && level != WarnLevel && level != ErrorLevel && level != FatalLevel && level != TestLevel {
		return errors.New("invalid log level")
	}

	l.level = level
	return nil
}

func (l *Logger) Test(message string) {
	if l.level != TestLevel {
		return
	}

	byteMessage := formatLog(message, TestLevel)
	if byteMessage == nil {
		panic("error formatting log message")
	}

	l.log.LogSubject(TestLevel, byteMessage, nil)
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
	byteMessage := formatLog(message, ErrorLevel)
	if byteMessage == nil {
		panic("error formatting log message")
	}

	l.log.LogSubject(ErrorLevel, byteMessage, nil)
}

func (l *Logger) Fatal(message string) {
	byteMessage := formatLog(message, FatalLevel)
	if byteMessage == nil {
		panic("error formatting log message")
	}

	l.log.LogSubject(FatalLevel, byteMessage, nil)
}

func formatLog(message, level string) []byte {
	if message == "" || level == "" {
		return nil
	}

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
