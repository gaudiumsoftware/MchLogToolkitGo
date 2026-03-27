package mchlogtoolkitgo

import (
	"encoding/json"
	"errors"
	"log"
	"os"
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

	// EnvUDPTarget is the environment variable name for the UDP target address.
	// Format: "host:port" (e.g., "graylog.example.com:12201")
	EnvUDPTarget = "MCHLOG_UDP_TARGET"

	// EnvUDPCompress is the environment variable to enable/disable GZIP compression for UDP.
	// Values: "true" or "false" (default: "true")
	EnvUDPCompress = "MCHLOG_UDP_COMPRESS"

	// EnvFileOutput is the environment variable to enable/disable file output.
	// Values: "true" or "false" (default: "true")
	EnvFileOutput = "MCHLOG_FILE_OUTPUT"
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

// Initialize inicializa o logger.
// If the environment variable MCHLOG_UDP_TARGET is set, UDP output is automatically configured.
// If MCHLOG_FILE_OUTPUT is set to "false", file output is disabled.
func (l *Logger) Initialize() {
	// Check environment variables for UDP configuration
	if target := os.Getenv(EnvUDPTarget); target != "" {
		compress := true
		if v := os.Getenv(EnvUDPCompress); strings.ToLower(v) == "false" {
			compress = false
		}
		if err := mchlogcore.SetUDPTarget(target, compress); err != nil {
			log.Printf("[mchlog] failed to configure UDP target %q from environment: %v", target, err)
		}
	}

	// Check environment variable for file output
	if v := os.Getenv(EnvFileOutput); strings.ToLower(v) == "false" {
		mchlogcore.SetFileOutput(false)
	}

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

// SetUDPTarget configures the logger to send GELF messages via UDP to the given address.
// The address should be in "host:port" format (e.g., "graylog.example.com:12201").
// GZIP compression is enabled by default.
//
// Note: UDP target and file output settings are global and shared across all
// Logger instances. Changing them on one instance affects all others.
func (l *Logger) SetUDPTarget(address string) error {
	return mchlogcore.SetUDPTarget(address, true)
}

// SetUDPTargetWithOptions configures the logger to send GELF messages via UDP with explicit options.
// The address should be in "host:port" format (e.g., "graylog.example.com:12201").
// If compress is true, messages will be GZIP compressed before sending.
func (l *Logger) SetUDPTargetWithOptions(address string, compress bool) error {
	return mchlogcore.SetUDPTarget(address, compress)
}

// DisableFileOutput disables file-based log output.
// When called, logs are only sent via UDP (if configured).
// This is a global setting shared across all Logger instances.
// When disabled before Initialize(), no log directories or files are created.
func (l *Logger) DisableFileOutput() {
	mchlogcore.SetFileOutput(false)
}

// Close drains any buffered UDP messages and closes the UDP connection.
// It does NOT close file handles used by the file logging backends (V1/V2),
// as those write directly to unbuffered *os.File handles managed by zerolog
// and are kept open for the lifetime of the process.
func (l *Logger) Close() error {
	return mchlogcore.CloseUDP()
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
