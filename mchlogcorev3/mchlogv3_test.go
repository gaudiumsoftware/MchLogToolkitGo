package mchlogcorev3

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Graylog2/go-gelf/gelf"
	_assert "github.com/stretchr/testify/assert"
)

// Este pacote é testado com o test package interno (e não com os helpers de
// `unittest`) porque `unittest` importa `mchlogcore`, que por sua vez importa
// `mchlogcorev3` — importá-lo aqui criaria um ciclo de importação. Os testes
// do facade em mchlogcore/ cobrem o V3 usando os helpers compartilhados.
//
// Os testes NÃO usam t.Parallel: activeCfg, MchLog.impl e o global de
// mchlogcorev2 são compartilhados.

// subjectSeq garante subjects únicos no binário de teste, já que o
// mchlogcorev2 (usado pelo destino de arquivo) mantém um cache de loggers por
// subject que sobrevive a novas inicializações.
var subjectSeq int

func uniqueSubject(prefix string) string {
	subjectSeq++
	return fmt.Sprintf("%s-%d", prefix, subjectSeq)
}

// listenUDP sobe um listener UDP local que faz o papel do Graylog.
func listenUDP(t *testing.T) (string, net.PacketConn) {
	t.Helper()

	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Error listening on UDP: %v", err)
	}

	t.Cleanup(func() { _ = conn.Close() })

	return conn.LocalAddr().String(), conn
}

// readDatagram lê o próximo datagrama, descomprimindo quando vier em GZIP.
func readDatagram(t *testing.T, conn net.PacketConn) []byte {
	t.Helper()

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))

	buf := make([]byte, 64*1024)
	n, _, err := conn.ReadFrom(buf)
	if err != nil {
		t.Fatalf("Error reading the datagram: %v", err)
	}

	data := buf[:n]
	if len(data) < 2 || data[0] != 0x1f || data[1] != 0x8b {
		return data
	}

	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Error creating the gzip reader: %v", err)
	}
	defer reader.Close()

	out, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("Error reading the gzip content: %v", err)
	}

	return out
}

// captureStderr substitui os.Stderr por um pipe enquanto fn roda e devolve
// tudo que foi escrito.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()

	original := os.Stderr

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("Error creating the pipe: %v", err)
	}
	os.Stderr = writer

	var (
		buf bytes.Buffer
		wg  sync.WaitGroup
	)

	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(&buf, reader)
	}()

	fn()

	_ = writer.Close()
	wg.Wait()
	os.Stderr = original

	return buf.String()
}

// newGraylogUDP monta um destino de rede apontado para addr.
func newGraylogUDP(t *testing.T, addr, service string) *graylogUDP {
	t.Helper()

	writer, err := gelf.NewWriter(addr)
	if err != nil {
		t.Fatalf("Error creating the GELF writer: %v", err)
	}

	writer.CompressionType = gelf.CompressNone

	return &graylogUDP{
		writer:      writer,
		cfg:         DestinationConfig{Protocol: ProtocolGraylogUDP, Addr: addr, Source: "pod-1"},
		serviceName: service,
	}
}

func TestConfigure(t *testing.T) {
	assert := _assert.New(t)

	testCases := []struct {
		name             string
		config           DestinationConfig
		expectedErr      string
		expectedProtocol Protocol
	}{
		{
			name:             "Aplica o protocolo de arquivo por padrão",
			config:           DestinationConfig{},
			expectedProtocol: ProtocolFile,
		},
		{
			name:             "Aceita o protocolo de arquivo sem campos obrigatórios",
			config:           DestinationConfig{Protocol: ProtocolFile},
			expectedProtocol: ProtocolFile,
		},
		{
			name:             "Aceita o protocolo Graylog UDP completo",
			config:           DestinationConfig{Protocol: ProtocolGraylogUDP, Addr: "localhost:12201", Source: "pod-1"},
			expectedProtocol: ProtocolGraylogUDP,
		},
		{
			name:             "Preserva a desativação do GZIP",
			config:           DestinationConfig{Protocol: ProtocolGraylogUDP, Addr: "localhost:12201", Source: "pod-1", DisableGZIP: true},
			expectedProtocol: ProtocolGraylogUDP,
		},
		{
			name:        "Exige Addr no protocolo Graylog UDP",
			config:      DestinationConfig{Protocol: ProtocolGraylogUDP, Source: "pod-1"},
			expectedErr: "mchlogcorev3: Addr is required for ProtocolGraylogUDP",
		},
		{
			name:        "Exige Source no protocolo Graylog UDP",
			config:      DestinationConfig{Protocol: ProtocolGraylogUDP, Addr: "localhost:12201"},
			expectedErr: "mchlogcorev3: Source is required for ProtocolGraylogUDP (caller-provided)",
		},
		{
			name:        "Rejeita um protocolo desconhecido",
			config:      DestinationConfig{Protocol: "graylog-tcp"},
			expectedErr: "mchlogcorev3: unknown Protocol: graylog-tcp",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			resetConfig()
			t.Cleanup(resetConfig)

			err := Configure(testCase.config)

			if testCase.expectedErr != "" {
				assert.EqualError(err, testCase.expectedErr)
				assert.False(IsConfigured())
				return
			}

			assert.NoError(err)
			assert.Equal(testCase.expectedProtocol, ActiveConfig().Protocol)
			assert.Equal(testCase.config.DisableGZIP, ActiveConfig().DisableGZIP)
		})
	}
}

func TestActiveConfig(t *testing.T) {
	assert := _assert.New(t)

	testCases := []struct {
		name           string
		config         *DestinationConfig
		expectedConfig DestinationConfig
	}{
		{
			name:           "Devolve a configuração zerada antes de Configure",
			expectedConfig: DestinationConfig{},
		},
		{
			name:           "Devolve a configuração de arquivo normalizada",
			config:         &DestinationConfig{},
			expectedConfig: DestinationConfig{Protocol: ProtocolFile},
		},
		{
			name:           "Devolve a configuração de rede como informada",
			config:         &DestinationConfig{Protocol: ProtocolGraylogUDP, Addr: "localhost:12201", Source: "pod-1"},
			expectedConfig: DestinationConfig{Protocol: ProtocolGraylogUDP, Addr: "localhost:12201", Source: "pod-1"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			resetConfig()
			t.Cleanup(resetConfig)

			if testCase.config != nil {
				assert.NoError(Configure(*testCase.config))
			}

			assert.Equal(testCase.expectedConfig, ActiveConfig())
		})
	}
}

func TestIsConfigured(t *testing.T) {
	assert := _assert.New(t)

	testCases := []struct {
		name     string
		config   *DestinationConfig
		expected bool
	}{
		{
			name:     "É falso antes de Configure",
			expected: false,
		},
		{
			name:     "É verdadeiro após um Configure bem-sucedido",
			config:   &DestinationConfig{Protocol: ProtocolFile},
			expected: true,
		},
		{
			name:     "Continua falso após um Configure inválido",
			config:   &DestinationConfig{Protocol: ProtocolGraylogUDP},
			expected: false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			resetConfig()
			t.Cleanup(resetConfig)

			if testCase.config != nil {
				_ = Configure(*testCase.config)
			}

			assert.Equal(testCase.expected, IsConfigured())
		})
	}
}

func TestResetConfig(t *testing.T) {
	assert := _assert.New(t)

	testCases := []struct {
		name   string
		config DestinationConfig
	}{
		{
			name:   "Limpa uma configuração de arquivo",
			config: DestinationConfig{Protocol: ProtocolFile},
		},
		{
			name:   "Limpa uma configuração de rede",
			config: DestinationConfig{Protocol: ProtocolGraylogUDP, Addr: "localhost:12201", Source: "pod-1"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Cleanup(resetConfig)

			assert.NoError(Configure(testCase.config))
			assert.True(IsConfigured())

			resetConfig()

			assert.False(IsConfigured())
			assert.Equal(DestinationConfig{}, ActiveConfig())
		})
	}
}

func TestDefaultSource(t *testing.T) {
	assert := _assert.New(t)

	hostname, err := os.Hostname()
	expected := hostname
	if err != nil || hostname == "" {
		expected = "unknown"
	}

	testCases := []struct {
		name string
	}{
		{
			name: "Devolve o hostname da máquina",
		},
		{
			name: "É estável entre chamadas",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			assert.Equal(expected, DefaultSource())
			assert.NotEmpty(DefaultSource())
		})
	}
}

func TestLevelToSyslog(t *testing.T) {
	assert := _assert.New(t)

	testCases := []struct {
		name     string
		level    string
		expected int32
	}{
		{
			name:     "fatal vira LOG_CRIT",
			level:    "fatal",
			expected: gelf.LOG_CRIT,
		},
		{
			name:     "error vira LOG_ERR",
			level:    "error",
			expected: gelf.LOG_ERR,
		},
		{
			name:     "warn vira LOG_WARNING",
			level:    "warn",
			expected: gelf.LOG_WARNING,
		},
		{
			name:     "info vira LOG_INFO",
			level:    "info",
			expected: gelf.LOG_INFO,
		},
		{
			name:     "debug vira LOG_DEBUG",
			level:    "debug",
			expected: gelf.LOG_DEBUG,
		},
		{
			name:     "test vira LOG_DEBUG",
			level:    "test",
			expected: gelf.LOG_DEBUG,
		},
		{
			name:     "Nível desconhecido vira LOG_INFO",
			level:    "verbose",
			expected: gelf.LOG_INFO,
		},
		{
			name:     "Nível vazio vira LOG_INFO",
			level:    "",
			expected: gelf.LOG_INFO,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			assert.Equal(testCase.expected, levelToSyslog(testCase.level))
		})
	}
}

func TestStringify(t *testing.T) {
	assert := _assert.New(t)

	testCases := []struct {
		name     string
		value    any
		expected string
	}{
		{
			name:     "Devolve a própria string",
			value:    "abc",
			expected: "abc",
		},
		{
			name:     "Formata um inteiro",
			value:    42,
			expected: "42",
		},
		{
			name:     "Formata um float",
			value:    1.5,
			expected: "1.5",
		},
		{
			name:     "Formata um booleano",
			value:    true,
			expected: "true",
		},
		{
			name:     "Formata um valor nulo",
			value:    nil,
			expected: "<nil>",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			assert.Equal(testCase.expected, stringify(testCase.value))
		})
	}
}

func TestContentToMap(t *testing.T) {
	assert := _assert.New(t)

	testCases := []struct {
		name        string
		content     any
		expected    map[string]any
		expectedErr string
	}{
		{
			name:     "Copia um map[string]any",
			content:  map[string]any{"message": "hello"},
			expected: map[string]any{"message": "hello"},
		},
		{
			name:     "Converte um map[string]string",
			content:  map[string]string{"message": "hello"},
			expected: map[string]any{"message": "hello"},
		},
		{
			name:     "Converte um map com value type estático via reflect",
			content:  map[string]int{"tick": 10},
			expected: map[string]any{"tick": 10},
		},
		{
			name:     "Decodifica um []byte json",
			content:  []byte(`{"message":"hello"}`),
			expected: map[string]any{"message": "hello"},
		},
		{
			name:     "Decodifica uma string json",
			content:  `{"message":"hello"}`,
			expected: map[string]any{"message": "hello"},
		},
		{
			name:        "Rejeita conteúdo nulo",
			content:     nil,
			expectedErr: "mchlogcorev3: nil content",
		},
		{
			name:        "Rejeita um []byte que não é json",
			content:     []byte(`{invalido}`),
			expectedErr: "invalid character 'i' looking for beginning of object key string",
		},
		{
			name:        "Rejeita uma string que não é json",
			content:     "isto nao e json",
			expectedErr: "invalid character 'i' looking for beginning of value",
		},
		{
			name:        "Rejeita um tipo não suportado",
			content:     42,
			expectedErr: "mchlogcorev3: unsupported content type int",
		},
		{
			name:        "Rejeita um map com chave não textual",
			content:     map[int]string{1: "a"},
			expectedErr: "mchlogcorev3: unsupported content type map[int]string",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			fields, err := contentToMap(testCase.content)

			if testCase.expectedErr != "" {
				assert.Nil(fields)
				assert.EqualError(err, testCase.expectedErr)
				return
			}

			assert.NoError(err)
			assert.Equal(testCase.expected, fields)
		})
	}
}

func TestBuildGELFMessage(t *testing.T) {
	assert := _assert.New(t)

	config := DestinationConfig{Protocol: ProtocolGraylogUDP, Addr: "localhost:12201", Source: "pod-1"}

	testCases := []struct {
		name          string
		level         string
		content       any
		errLog        error
		expectedShort string
		expectedExtra map[string]any
		expectedErr   string
	}{
		{
			name:          "Monta os campos obrigatórios",
			level:         "info",
			content:       []byte(`{"message":"hello","level":"info","source":"x.go","line":"12","trace":""}`),
			expectedShort: "hello",
			expectedExtra: map[string]any{
				"_application_name": "payments-api",
				"_log_id":           "payments-api-mchlog-info",
				"_level_name":       "info",
				"_file":             "x.go",
				"_line":             "12",
				"_trace":            "",
				"_level":            "info",
			},
		},
		{
			name:          "Prefixa as demais chaves do payload",
			level:         "warn",
			content:       map[string]any{"message": "atenção", "pedido": "123"},
			expectedShort: "atenção",
			expectedExtra: map[string]any{
				"_application_name": "payments-api",
				"_log_id":           "payments-api-mchlog-warn",
				"_level_name":       "warn",
				"_pedido":           "123",
			},
		},
		{
			name:          "Formata uma mensagem que não é string",
			level:         "info",
			content:       map[string]any{"message": 42},
			expectedShort: "42",
			expectedExtra: map[string]any{
				"_application_name": "payments-api",
				"_log_id":           "payments-api-mchlog-info",
				"_level_name":       "info",
			},
		},
		{
			name:          "Aceita payload sem a chave message",
			level:         "info",
			content:       map[string]string{"pedido": "123"},
			expectedShort: "",
			expectedExtra: map[string]any{
				"_application_name": "payments-api",
				"_log_id":           "payments-api-mchlog-info",
				"_level_name":       "info",
				"_pedido":           "123",
			},
		},
		{
			name:          "Inclui o erro associado",
			level:         "error",
			content:       map[string]any{"message": "falhou"},
			errLog:        errors.New("boom"),
			expectedShort: "falhou",
			expectedExtra: map[string]any{
				"_application_name": "payments-api",
				"_log_id":           "payments-api-mchlog-error",
				"_level_name":       "error",
				"_error":            "boom",
			},
		},
		{
			name:        "Propaga o erro de conteúdo inválido",
			level:       "info",
			content:     []byte(`{invalido}`),
			expectedErr: "invalid character 'i' looking for beginning of object key string",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			message, err := buildGELFMessage("payments-api", testCase.level, testCase.content, testCase.errLog, config)

			if testCase.expectedErr != "" {
				assert.Nil(message)
				assert.EqualError(err, testCase.expectedErr)
				return
			}

			assert.NoError(err)
			assert.Equal("1.1", message.Version)
			assert.Equal("pod-1", message.Host)
			assert.Equal(levelToSyslog(testCase.level), message.Level)
			assert.Equal(testCase.expectedShort, message.Short)
			assert.Equal(testCase.expectedExtra, message.Extra)
			assert.NotZero(message.TimeUnix)

			_, err = json.Marshal(message)
			assert.NoError(err)
		})
	}
}

func TestServiceFromPath(t *testing.T) {
	assert := _assert.New(t)

	testCases := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "Extrai o serviço de um path absoluto",
			path:     "/applog/payments-api/",
			expected: "payments-api",
		},
		{
			name:     "Extrai o serviço sem a barra final",
			path:     "/applog/payments-api",
			expected: "payments-api",
		},
		{
			name:     "Extrai o serviço de um path relativo",
			path:     "./applog/payments-api/",
			expected: "payments-api",
		},
		{
			name:     "Extrai o serviço de um path com separador do Windows",
			path:     `C:\applog\payments-api\`,
			expected: "payments-api",
		},
		{
			name:     "Aceita apenas o nome do serviço",
			path:     "payments-api",
			expected: "payments-api",
		},
		{
			name:     "Rejeita um path vazio",
			path:     "",
			expected: "",
		},
		{
			name:     "Rejeita um path só com barras e espaços",
			path:     "/ /",
			expected: "",
		},
		{
			name:     "Rejeita o diretório corrente",
			path:     "./",
			expected: "",
		},
		{
			name:     "Rejeita o diretório pai",
			path:     "../",
			expected: "",
		},
		{
			name:     "Rejeita a raiz do Windows",
			path:     `C:\`,
			expected: "",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			assert.Equal(testCase.expected, serviceFromPath(testCase.path))
		})
	}
}

func TestNewFileDestination(t *testing.T) {
	assert := _assert.New(t)

	testCases := []struct {
		name string
		path string
	}{
		{
			name: "Inicializa com um caminho absoluto",
			path: t.TempDir(),
		},
		{
			name: "Inicializa com um caminho terminado em barra",
			path: t.TempDir() + "/",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			subject := uniqueSubject("novo")

			destination := newFileDestination(testCase.path)

			assert.NotNil(destination.inner)
			assert.Equal(
				filepath.Join(testCase.path, subject, subject+".log"),
				destination.GetFileNameFromStreamName(subject),
			)
		})
	}
}

func TestFileDestinationLogSubject(t *testing.T) {
	assert := _assert.New(t)

	testCases := []struct {
		name            string
		nilDestination  bool
		nilInner        bool
		subjectPrefix   string
		errLog          error
		expectedSubject string
		expectedContent string
	}{
		{
			name:           "Ignora o receiver nulo",
			nilDestination: true,
		},
		{
			name:     "Ignora o destino sem inner",
			nilInner: true,
		},
		{
			name:            "Grava o conteúdo no arquivo do subject",
			subjectPrefix:   "arquivo",
			expectedContent: `"chave":"valor"`,
		},
		{
			name:            "Prefixa o subject quando há erro",
			subjectPrefix:   "arquivo",
			errLog:          errors.New("isto é um erro forçado"),
			expectedContent: `"error":"isto é um erro forçado"`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			basePath := t.TempDir()

			var destination *fileDestination
			switch {
			case testCase.nilDestination:
			case testCase.nilInner:
				destination = &fileDestination{}
			default:
				destination = newFileDestination(basePath)
			}

			subject := uniqueSubject(testCase.subjectPrefix)

			assert.NotPanics(func() {
				destination.LogSubject(subject, map[string]any{"chave": "valor"}, testCase.errLog)
			})

			if testCase.expectedContent == "" {
				return
			}

			fileSubject := subject
			if testCase.errLog != nil {
				fileSubject = "err_" + subject
			}

			content, err := os.ReadFile(filepath.Join(basePath, fileSubject, fileSubject+".log"))
			assert.NoError(err)
			assert.Contains(string(content), testCase.expectedContent)
		})
	}
}

func TestFileDestinationGetFileNameFromStreamName(t *testing.T) {
	assert := _assert.New(t)

	testCases := []struct {
		name           string
		nilDestination bool
		nilInner       bool
		expectEmpty    bool
	}{
		{
			name:           "Devolve vazio para o receiver nulo",
			nilDestination: true,
			expectEmpty:    true,
		},
		{
			name:        "Devolve vazio para o destino sem inner",
			nilInner:    true,
			expectEmpty: true,
		},
		{
			name: "Delega para o destino de arquivo",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			basePath := t.TempDir()
			subject := uniqueSubject("nome")

			var destination *fileDestination
			switch {
			case testCase.nilDestination:
			case testCase.nilInner:
				destination = &fileDestination{}
			default:
				destination = newFileDestination(basePath)
			}

			fileName := destination.GetFileNameFromStreamName(subject)

			if testCase.expectEmpty {
				assert.Equal("", fileName)
				return
			}

			assert.Equal(filepath.Join(basePath, subject, subject+".log"), fileName)
		})
	}
}

func TestFileDestinationClose(t *testing.T) {
	assert := _assert.New(t)

	testCases := []struct {
		name           string
		nilDestination bool
	}{
		{
			name:           "É no-op para o receiver nulo",
			nilDestination: true,
		},
		{
			name: "É no-op para um destino ativo",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			var destination *fileDestination
			if !testCase.nilDestination {
				destination = newFileDestination(t.TempDir())
			}

			assert.NoError(destination.Close())
			assert.NoError(destination.Close())
		})
	}
}

func TestGraylogUDPLogSubject(t *testing.T) {
	assert := _assert.New(t)

	testCases := []struct {
		name          string
		nilReceiver   bool
		closeWriter   bool
		subject       string
		content       any
		expectedShort string
		expectedWarn  string
	}{
		{
			name:        "Ignora o receiver nulo",
			nilReceiver: true,
			subject:     "info",
			content:     map[string]any{"message": "hello"},
		},
		{
			name:    "Ignora o subject vazio",
			subject: "",
			content: map[string]any{"message": "hello"},
		},
		{
			name:          "Envia o datagrama GELF",
			subject:       "info",
			content:       []byte(`{"message":"hello","source":"x.go","line":"1","trace":""}`),
			expectedShort: "hello",
		},
		{
			name:         "Avisa quando o conteúdo é inválido",
			subject:      "info",
			content:      []byte(`{invalido}`),
			expectedWarn: "mchlogcorev3: GELF UDP send failed",
		},
		{
			name:         "Avisa quando o envio falha",
			subject:      "info",
			closeWriter:  true,
			content:      map[string]any{"message": "hello"},
			expectedWarn: "mchlogcorev3: GELF UDP send failed",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			addr, conn := listenUDP(t)

			var destination *graylogUDP
			if !testCase.nilReceiver {
				destination = newGraylogUDP(t, addr, "payments-api")
				t.Cleanup(func() { _ = destination.Close() })
			}

			if testCase.closeWriter {
				assert.NoError(destination.writer.Close())
			}

			stderr := captureStderr(t, func() {
				assert.NotPanics(func() {
					destination.LogSubject(testCase.subject, testCase.content, nil)
				})
			})

			if testCase.expectedWarn != "" {
				assert.Contains(stderr, testCase.expectedWarn)
				return
			}

			assert.Empty(stderr)

			if testCase.expectedShort == "" {
				_ = conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
				_, _, err := conn.ReadFrom(make([]byte, 1024))
				assert.Error(err)
				return
			}

			var received map[string]any
			assert.NoError(json.Unmarshal(readDatagram(t, conn), &received))
			assert.Equal(testCase.expectedShort, received["short_message"])
			assert.Equal("payments-api", received["_application_name"])
		})
	}
}

func TestGraylogUDPGetFileNameFromStreamName(t *testing.T) {
	assert := _assert.New(t)

	testCases := []struct {
		name        string
		nilReceiver bool
		subject     string
		expectEmpty bool
	}{
		{
			name:        "Devolve vazio para o receiver nulo",
			nilReceiver: true,
			subject:     "info",
			expectEmpty: true,
		},
		{
			name:    "Compõe o descritor de rede",
			subject: "info",
		},
		{
			name:    "Compõe o descritor para um subject de erro",
			subject: "err_info",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			addr, _ := listenUDP(t)

			var destination *graylogUDP
			if !testCase.nilReceiver {
				destination = newGraylogUDP(t, addr, "payments-api")
				t.Cleanup(func() { _ = destination.Close() })
			}

			descriptor := destination.GetFileNameFromStreamName(testCase.subject)

			if testCase.expectEmpty {
				assert.Equal("", descriptor)
				return
			}

			assert.Equal("udp://"+addr+"/"+testCase.subject, descriptor)
		})
	}
}

func TestGraylogUDPClose(t *testing.T) {
	assert := _assert.New(t)

	testCases := []struct {
		name        string
		nilReceiver bool
		nilWriter   bool
	}{
		{
			name:        "É no-op para o receiver nulo",
			nilReceiver: true,
		},
		{
			name:      "É no-op para o destino sem writer",
			nilWriter: true,
		},
		{
			name: "Fecha o socket uma única vez",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			addr, _ := listenUDP(t)

			var destination *graylogUDP
			switch {
			case testCase.nilReceiver:
			case testCase.nilWriter:
				destination = &graylogUDP{}
			default:
				destination = newGraylogUDP(t, addr, "payments-api")
			}

			assert.NoError(destination.Close())
			assert.NoError(destination.Close())
		})
	}
}

func TestWarnOnce(t *testing.T) {
	assert := _assert.New(t)

	testCases := []struct {
		name         string
		lastWarn     time.Time
		expectedWarn bool
	}{
		{
			name:         "Emite o primeiro aviso",
			expectedWarn: true,
		},
		{
			name:         "Silencia dentro da janela",
			lastWarn:     time.Now(),
			expectedWarn: false,
		},
		{
			name:         "Emite novamente após a janela",
			lastWarn:     time.Now().Add(-2 * warnWindow),
			expectedWarn: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			addr, _ := listenUDP(t)

			destination := newGraylogUDP(t, addr, "payments-api")
			t.Cleanup(func() { _ = destination.Close() })

			destination.lastWarn = testCase.lastWarn

			stderr := captureStderr(t, func() {
				destination.warnOnce(errors.New("boom"))
			})

			if testCase.expectedWarn {
				assert.Contains(stderr, "mchlogcorev3: GELF UDP send failed: boom")
				return
			}

			assert.Empty(stderr)
		})
	}
}

func TestInitialize(t *testing.T) {
	assert := _assert.New(t)

	testCases := []struct {
		name            string
		config          *DestinationConfig
		rawProtocol     Protocol
		badAddr         bool
		path            string
		expectedErr     string
		expectedFileDst bool
	}{
		{
			name:        "Exige Configure antes de Initialize",
			path:        "/applog/payments-api/",
			expectedErr: "mchlogcorev3: Configure must be called before Initialize",
		},
		{
			name:        "Rejeita um path sem nome de serviço",
			config:      &DestinationConfig{Protocol: ProtocolFile},
			path:        "./",
			expectedErr: "mchlogcorev3: cannot extract service name from path: ./",
		},
		{
			name:            "Instala o destino de arquivo",
			config:          &DestinationConfig{Protocol: ProtocolFile},
			expectedFileDst: true,
		},
		{
			name:   "Instala o destino Graylog UDP",
			config: &DestinationConfig{Protocol: ProtocolGraylogUDP, Source: "pod-1"},
		},
		{
			name:   "Instala o destino Graylog UDP sem compressão",
			config: &DestinationConfig{Protocol: ProtocolGraylogUDP, Source: "pod-1", DisableGZIP: true},
		},
		{
			name:        "Propaga a falha ao abrir o socket UDP",
			config:      &DestinationConfig{Protocol: ProtocolGraylogUDP, Source: "pod-1"},
			badAddr:     true,
			expectedErr: "mchlogcorev3: dial GELF UDP endereco-invalido:",
		},
		{
			name:        "Rejeita um protocolo sem destino correspondente",
			rawProtocol: "graylog-tcp",
			expectedErr: "mchlogcorev3: unsupported Protocol: graylog-tcp",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			resetConfig()
			t.Cleanup(func() {
				_ = MchLog.Close()
				resetConfig()
			})

			addr, _ := listenUDP(t)

			switch {
			case testCase.rawProtocol != "":
				// Configure rejeita protocolos desconhecidos, então o estado é
				// forçado para exercitar o default do switch de Initialize.
				cfgMu.Lock()
				activeCfg = DestinationConfig{Protocol: testCase.rawProtocol}
				configured = true
				cfgMu.Unlock()
			case testCase.config != nil:
				config := *testCase.config
				if config.Protocol == ProtocolGraylogUDP {
					config.Addr = addr
					if testCase.badAddr {
						config.Addr = "endereco-invalido"
					}
				}
				assert.NoError(Configure(config))
			}

			path := testCase.path
			if path == "" {
				path = t.TempDir() + "/payments-api/"
			}

			err := Initialize(path)

			if testCase.expectedErr != "" {
				assert.ErrorContains(err, testCase.expectedErr)
				return
			}

			assert.NoError(err)

			if testCase.expectedFileDst {
				assert.IsType(&fileDestination{}, MchLog.impl)
			} else {
				assert.IsType(&graylogUDP{}, MchLog.impl)
			}

			// Reentrância: a segunda chamada substitui e fecha o destino anterior.
			previous := MchLog.impl
			assert.NoError(Initialize(path))
			assert.NotSame(previous, MchLog.impl)
		})
	}
}

func TestLogTypeLogSubject(t *testing.T) {
	assert := _assert.New(t)

	testCases := []struct {
		name            string
		protocol        Protocol
		beforeInit      bool
		subject         string
		expectedContent string
	}{
		{
			name:       "É no-op antes de Initialize",
			beforeInit: true,
			subject:    "info",
		},
		{
			name:            "Delega para o destino de arquivo",
			protocol:        ProtocolFile,
			subject:         "info",
			expectedContent: `"message":"hello"`,
		},
		{
			name:            "Delega para o destino Graylog UDP",
			protocol:        ProtocolGraylogUDP,
			subject:         "info",
			expectedContent: `"short_message":"hello"`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			resetConfig()
			t.Cleanup(func() {
				_ = MchLog.Close()
				resetConfig()
			})

			addr, conn := listenUDP(t)
			basePath := t.TempDir() + "/payments-api/"

			if testCase.beforeInit {
				assert.NoError(MchLog.Close())

				assert.NotPanics(func() {
					MchLog.LogSubject(testCase.subject, map[string]any{"message": "hello"}, nil)
				})
				return
			}

			assert.NoError(Configure(DestinationConfig{
				Protocol: testCase.protocol,
				Addr:     addr,
				Source:   "pod-1",
			}))
			assert.NoError(Initialize(basePath))

			subject := uniqueSubject(testCase.subject)
			MchLog.LogSubject(subject, map[string]any{"message": "hello"}, nil)

			if testCase.protocol == ProtocolGraylogUDP {
				assert.Contains(string(readDatagram(t, conn)), testCase.expectedContent)
				return
			}

			content, err := os.ReadFile(filepath.Join(basePath, subject, subject+".log"))
			assert.NoError(err)
			assert.Contains(string(content), testCase.expectedContent)
		})
	}
}

func TestLogTypeGetFileNameFromStreamName(t *testing.T) {
	assert := _assert.New(t)

	testCases := []struct {
		name       string
		protocol   Protocol
		beforeInit bool
	}{
		{
			name:       "Devolve vazio antes de Initialize",
			beforeInit: true,
		},
		{
			name:     "Delega para o destino de arquivo",
			protocol: ProtocolFile,
		},
		{
			name:     "Delega para o destino Graylog UDP",
			protocol: ProtocolGraylogUDP,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			resetConfig()
			t.Cleanup(func() {
				_ = MchLog.Close()
				resetConfig()
			})

			addr, _ := listenUDP(t)
			basePath := t.TempDir() + "/payments-api/"
			subject := uniqueSubject("nome")

			if testCase.beforeInit {
				assert.NoError(MchLog.Close())
				assert.Equal("", MchLog.GetFileNameFromStreamName(subject))
				return
			}

			assert.NoError(Configure(DestinationConfig{
				Protocol: testCase.protocol,
				Addr:     addr,
				Source:   "pod-1",
			}))
			assert.NoError(Initialize(basePath))

			if testCase.protocol == ProtocolGraylogUDP {
				assert.Equal("udp://"+addr+"/"+subject, MchLog.GetFileNameFromStreamName(subject))
				return
			}

			assert.True(strings.HasPrefix(
				MchLog.GetFileNameFromStreamName(subject),
				filepath.Join(basePath, subject),
			))
		})
	}
}

func TestLogTypeClose(t *testing.T) {
	assert := _assert.New(t)

	testCases := []struct {
		name       string
		protocol   Protocol
		beforeInit bool
	}{
		{
			name:       "Devolve nil antes de Initialize",
			beforeInit: true,
		},
		{
			name:     "Fecha o destino de arquivo",
			protocol: ProtocolFile,
		},
		{
			name:     "Fecha o destino Graylog UDP",
			protocol: ProtocolGraylogUDP,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			resetConfig()
			t.Cleanup(func() {
				_ = MchLog.Close()
				resetConfig()
			})

			addr, _ := listenUDP(t)

			if !testCase.beforeInit {
				assert.NoError(Configure(DestinationConfig{
					Protocol: testCase.protocol,
					Addr:     addr,
					Source:   "pod-1",
				}))
				assert.NoError(Initialize(t.TempDir() + "/payments-api/"))
			}

			assert.NoError(MchLog.Close())
			assert.NoError(MchLog.Close())
			assert.Nil(MchLog.impl)
		})
	}
}
