package MchLogToolkitGo

import (
	"errors"
	"gaudium.com.br/gaudiumsoftware/MchLogToolkitGo/mchlogcore"
	"path/filepath"
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

	path := DebugPath
	if level != DebugLevel {
		path = ProdPath
	}
	mchlogcore.InitializeMchLog(filepath.Join(path, service))
	return &Logger{log: mchlogcore.MchLog, service: service, level: level}, nil
}

func (l *Logger) Debug(message string) {
	if l.level == DebugLevel {
		l.log.LogSubject(DebugLevel, []byte(message), nil)
	}
}

func (l *Logger) Warn(message string) {
	if l.level == WarnLevel || l.level == DebugLevel {
		l.log.LogSubject(WarnLevel, []byte(message), nil)
	}
}

func (l *Logger) Info(message string) {
	l.log.LogSubject(InfoLevel, []byte(message), nil)
}

func (l *Logger) Error(message string) {
	l.log.LogSubject(ErrorLevel, []byte(message), nil)
}
