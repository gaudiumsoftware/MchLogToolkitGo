package mchlogcore

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/gaudiumsoftware/mchlogtoolkitgo/mchlogcorev1"
	"github.com/gaudiumsoftware/mchlogtoolkitgo/mchlogcorev2"
	"github.com/gaudiumsoftware/mchlogtoolkitgo/mchlogcorev3"
	_assert "github.com/stretchr/testify/assert"
)

// Os testes NÃO usam t.Parallel: currentVersion, current e os globais dos
// pacotes de destino são compartilhados.

// initPath é o caminho usado por todos os casos ao inicializar o facade.
// É único no pacote porque o transporte V2 (usado direto e por baixo do V3 em
// modo arquivo) mantém um cache de loggers por subject que sobrevive a novas
// inicializações — apontar cada caso para um diretório diferente faria as
// gravações caírem no diretório do primeiro caso.
var initPath string

// v1FileName casa com o layout de arquivo do V1: <subject>[-<ip>]-<YYYYMMDDHH>.log
var v1FileName = regexp.MustCompile(`(-[0-9.]+)?-\d{10}\.log$`)

// subjectSeq garante subjects únicos no binário de teste, pelo mesmo motivo
// que initPath é único.
var subjectSeq int

func uniqueSubject(prefix string) string {
	subjectSeq++
	return fmt.Sprintf("%s-%d", prefix, subjectSeq)
}

func TestMain(m *testing.M) {
	baseDir, err := os.MkdirTemp("", "mchlogcore-test")
	if err != nil {
		fmt.Println("Failed to create the temp dir for the tests:", err)
		os.Exit(1)
	}

	initPath = filepath.Join(baseDir, "servico") + string(filepath.Separator)

	code := m.Run()

	if err := os.RemoveAll(baseDir); err != nil {
		fmt.Println("Failed to remove the temp dir after all tests:", err)
	}

	os.Exit(code)
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
func readDatagram(t *testing.T, conn net.PacketConn) string {
	t.Helper()

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))

	buf := make([]byte, 64*1024)
	n, _, err := conn.ReadFrom(buf)
	if err != nil {
		t.Fatalf("Error reading the datagram: %v", err)
	}

	data := buf[:n]
	if len(data) < 2 || data[0] != 0x1f || data[1] != 0x8b {
		return string(data)
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

	return string(out)
}

// selectTransport configura o V3 quando necessário, ativa a versão pedida e
// inicializa o facade, devolvendo o endereço do listener UDP usado.
func selectTransport(t *testing.T, assert *_assert.Assertions, version LogVersion, protocol mchlogcorev3.Protocol) (string, net.PacketConn) {
	t.Helper()

	t.Cleanup(func() {
		_ = MchLog.Close()
		SetVersion(V1)
	})

	addr, conn := listenUDP(t)

	if protocol != "" {
		assert.NoError(mchlogcorev3.Configure(mchlogcorev3.DestinationConfig{
			Protocol: protocol,
			Addr:     addr,
			Source:   "pod-1",
		}))
	}

	SetVersion(version)
	InitializeMchLog(initPath)

	return addr, conn
}

func TestTransportFor(t *testing.T) {
	assert := _assert.New(t)

	testCases := []struct {
		name     string
		version  LogVersion
		expected Transport
	}{
		{
			name:     "V1 mapeia para o destino de arquivo com rotação",
			version:  V1,
			expected: &mchlogcorev1.MchLog,
		},
		{
			name:     "V2 mapeia para o destino de arquivo simples",
			version:  V2,
			expected: &mchlogcorev2.MchLog,
		},
		{
			name:     "V3 mapeia para o destino unificado",
			version:  V3,
			expected: &mchlogcorev3.MchLog,
		},
		{
			name:     "Versão desconhecida cai no V1",
			version:  LogVersion(99),
			expected: &mchlogcorev1.MchLog,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			assert.Same(testCase.expected, transportFor(testCase.version))
		})
	}
}

func TestSetVersion(t *testing.T) {
	assert := _assert.New(t)

	testCases := []struct {
		name     string
		version  LogVersion
		expected Transport
	}{
		{
			name:     "Ativa o transporte V1",
			version:  V1,
			expected: &mchlogcorev1.MchLog,
		},
		{
			name:     "Ativa o transporte V2",
			version:  V2,
			expected: &mchlogcorev2.MchLog,
		},
		{
			name:     "Ativa o transporte V3",
			version:  V3,
			expected: &mchlogcorev3.MchLog,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Cleanup(func() { SetVersion(V1) })

			SetVersion(testCase.version)

			assert.Equal(testCase.version, currentVersion)
			assert.Same(testCase.expected, current)
		})
	}
}

func TestGetFileNameFromStreamName(t *testing.T) {
	assert := _assert.New(t)

	testCases := []struct {
		name        string
		version     LogVersion
		protocol    mchlogcorev3.Protocol
		expectedUDP bool
	}{
		{
			name:    "V1 compõe o nome com IP e hora",
			version: V1,
		},
		{
			name:    "V2 compõe o nome com o subject",
			version: V2,
		},
		{
			name:     "V3 em modo arquivo compõe o nome como o V2",
			version:  V3,
			protocol: mchlogcorev3.ProtocolFile,
		},
		{
			name:        "V3 em modo Graylog UDP devolve o descritor de rede",
			version:     V3,
			protocol:    mchlogcorev3.ProtocolGraylogUDP,
			expectedUDP: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			addr, _ := selectTransport(t, assert, testCase.version, testCase.protocol)

			subject := uniqueSubject("nome")
			fileName := MchLog.GetFileNameFromStreamName(subject)

			switch {
			case testCase.expectedUDP:
				assert.Equal("udp://"+addr+"/"+subject, fileName)
			case testCase.version == V1:
				assert.Regexp(v1FileName, fileName)
				assert.Contains(fileName, filepath.Join(initPath, subject))
			default:
				assert.Equal(filepath.Join(initPath, subject, subject+".log"), fileName)
			}
		})
	}
}

func TestLogSubject(t *testing.T) {
	assert := _assert.New(t)

	testCases := []struct {
		name        string
		version     LogVersion
		protocol    mchlogcorev3.Protocol
		expectedUDP bool
		expected    string
	}{
		{
			name:     "V1 delega para o destino de arquivo com rotação",
			version:  V1,
			expected: `"chave":"valor"`,
		},
		{
			name:     "V2 delega para o destino de arquivo simples",
			version:  V2,
			expected: `"chave":"valor"`,
		},
		{
			name:     "V3 em modo arquivo delega para o destino unificado",
			version:  V3,
			protocol: mchlogcorev3.ProtocolFile,
			expected: `"chave":"valor"`,
		},
		{
			name:        "V3 em modo Graylog UDP envia o datagrama GELF",
			version:     V3,
			protocol:    mchlogcorev3.ProtocolGraylogUDP,
			expectedUDP: true,
			expected:    `"_application_name":"servico"`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, conn := selectTransport(t, assert, testCase.version, testCase.protocol)

			if testCase.expectedUDP {
				// Descarta o datagrama do log de inicialização.
				readDatagram(t, conn)
			}

			subject := uniqueSubject("log")
			MchLog.LogSubject(subject, map[string]any{"chave": "valor"}, nil)

			if testCase.expectedUDP {
				assert.Contains(readDatagram(t, conn), testCase.expected)
				return
			}

			content, err := os.ReadFile(MchLog.GetFileNameFromStreamName(subject))
			assert.NoError(err)
			assert.Contains(string(content), testCase.expected)
		})
	}
}

func TestGetIP(t *testing.T) {
	assert := _assert.New(t)

	testCases := []struct {
		name          string
		version       LogVersion
		protocol      mchlogcorev3.Protocol
		delegatesToV1 bool
	}{
		{
			name:          "Delega para o V1 quando ele é o transporte ativo",
			version:       V1,
			delegatesToV1: true,
		},
		{
			name:    "Devolve vazio para o V2",
			version: V2,
		},
		{
			name:     "Devolve vazio para o V3",
			version:  V3,
			protocol: mchlogcorev3.ProtocolFile,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			selectTransport(t, assert, testCase.version, testCase.protocol)

			if testCase.delegatesToV1 {
				assert.Equal(mchlogcorev1.MchLog.GetIP(), MchLog.GetIP())
				return
			}

			assert.Equal("", MchLog.GetIP())
		})
	}
}

func TestClose(t *testing.T) {
	assert := _assert.New(t)

	testCases := []struct {
		name     string
		version  LogVersion
		protocol mchlogcorev3.Protocol
	}{
		{
			name:    "É no-op para o V1",
			version: V1,
		},
		{
			name:    "É no-op para o V2",
			version: V2,
		},
		{
			name:     "É no-op para o V3 em modo arquivo",
			version:  V3,
			protocol: mchlogcorev3.ProtocolFile,
		},
		{
			name:     "Libera o socket do V3 em modo Graylog UDP",
			version:  V3,
			protocol: mchlogcorev3.ProtocolGraylogUDP,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			selectTransport(t, assert, testCase.version, testCase.protocol)

			assert.NoError(MchLog.Close())
			assert.NoError(MchLog.Close())
		})
	}
}

func TestInitializeMchLog(t *testing.T) {
	assert := _assert.New(t)

	testCases := []struct {
		name            string
		version         LogVersion
		protocol        mchlogcorev3.Protocol
		badAddr         bool
		expectedVersion string
	}{
		{
			name:            "Registra a inicialização do V1",
			version:         V1,
			expectedVersion: `"version":"V1"`,
		},
		{
			name:            "Registra a inicialização do V2",
			version:         V2,
			expectedVersion: `"version":"V2"`,
		},
		{
			name:            "Registra a inicialização do V3",
			version:         V3,
			protocol:        mchlogcorev3.ProtocolFile,
			expectedVersion: `"version":"V3"`,
		},
		{
			name:     "Não registra quando a inicialização do V3 falha",
			version:  V3,
			protocol: mchlogcorev3.ProtocolGraylogUDP,
			badAddr:  true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Cleanup(func() {
				_ = MchLog.Close()
				SetVersion(V1)
			})

			addr, _ := listenUDP(t)
			if testCase.badAddr {
				addr = "endereco-invalido"
			}

			if testCase.protocol != "" {
				assert.NoError(mchlogcorev3.Configure(mchlogcorev3.DestinationConfig{
					Protocol: testCase.protocol,
					Addr:     addr,
					Source:   "pod-1",
				}))
			}

			SetVersion(testCase.version)

			// Zera o destino do V3 para que uma inicialização que falha não
			// deixe o destino da execução anterior ativo.
			assert.NoError(MchLog.Close())

			InitializeMchLog(initPath)

			fileName := MchLog.GetFileNameFromStreamName("info")

			if testCase.expectedVersion == "" {
				assert.Equal("", fileName)
				return
			}

			content, err := os.ReadFile(fileName)
			assert.NoError(err)
			assert.Contains(string(content), `"message":"MchLogToolkit initialized"`)
			assert.Contains(string(content), testCase.expectedVersion)
		})
	}
}
