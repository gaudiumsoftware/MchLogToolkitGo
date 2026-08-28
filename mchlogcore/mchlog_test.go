package mchlogcore_test

import (
	"bytes"
	"compress/gzip"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/gaudiumsoftware/mchlogtoolkitgo"
	"github.com/gaudiumsoftware/mchlogtoolkitgo/mchlogcore"
	"github.com/gaudiumsoftware/mchlogtoolkitgo/mchlogcorev3"
	"github.com/gaudiumsoftware/mchlogtoolkitgo/unittest"
	"github.com/gaudiumsoftware/mchlogtoolkitgo/unittest/logger"
)

// mockPath é o diretório usado pelo logger compartilhado dos testes, para o
// qual o facade é devolvido ao final de cada caso.
const mockPath = mchlogtoolkitgo.DebugPath + logger.ServiceName + "/"

// initPath é o caminho usado por todos os casos ao inicializar o facade.
// É único no pacote porque o transporte V2 (usado direto e por baixo do V3 em
// modo arquivo) mantém um cache de loggers por subject que sobrevive a novas
// inicializações — apontar cada caso para um diretório diferente faria as
// gravações caírem no diretório do primeiro caso.
const initPath = mchlogtoolkitgo.DebugPath + "core-tests/servico/"

// v1FileName casa com o layout de arquivo do V1: <subject>[-<ip>]-<YYYYMMDDHH>.log
var v1FileName = regexp.MustCompile(`teste(-[0-9.]+)?-\d{10}\.log$`)

func TestMain(m *testing.M) {
	unittest.RunTests(m)
}

// restoreFacade devolve o facade ao transporte e ao caminho esperados pelos
// helpers de unittest depois que um caso troca a versão ativa.
func restoreFacade(t *testing.T) {
	t.Helper()

	t.Cleanup(func() {
		mchlogcore.SetVersion(mchlogcore.V1)
		mchlogcore.InitializeMchLog(mockPath)
	})
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

// localIP devolve o primeiro IPv4 não-loopback da máquina, mesma regra usada
// pelo transporte V1.
func localIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}

	for _, address := range addrs {
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}

	return ""
}

func TestSetVersion(t *testing.T) {
	assert, teardown := unittest.SetupTests(t)
	defer teardown()

	testCases := []struct {
		name       string
		version    mchlogcore.LogVersion
		protocol   mchlogcorev3.Protocol
		expectedIP string
	}{
		{
			name:       "V1 seleciona o transporte de arquivo com IP no nome",
			version:    mchlogcore.V1,
			expectedIP: localIP(),
		},
		{
			name:    "V2 seleciona o transporte de arquivo simples",
			version: mchlogcore.V2,
		},
		{
			name:     "V3 seleciona o transporte unificado em modo arquivo",
			version:  mchlogcore.V3,
			protocol: mchlogcorev3.ProtocolFile,
		},
		{
			name:     "V3 seleciona o transporte unificado em modo Graylog UDP",
			version:  mchlogcore.V3,
			protocol: mchlogcorev3.ProtocolGraylogUDP,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			teardownTestCase := unittest.SetupTestCase(t)
			defer teardownTestCase(t)

			restoreFacade(t)

			addr, _ := listenUDP(t)
			if testCase.protocol != "" {
				assert.NoError(mchlogcorev3.Configure(mchlogcorev3.DestinationConfig{
					Protocol: testCase.protocol,
					Addr:     addr,
					Source:   "pod-1",
				}))
			}

			mchlogcore.SetVersion(testCase.version)
			mchlogcore.InitializeMchLog(initPath)

			assert.Equal(testCase.expectedIP, mchlogcore.MchLog.GetIP())
		})
	}
}

func TestGetFileNameFromStreamName(t *testing.T) {
	assert, teardown := unittest.SetupTests(t, "teste")
	defer teardown()

	testCases := []struct {
		name        string
		version     mchlogcore.LogVersion
		protocol    mchlogcorev3.Protocol
		expectedUDP bool
	}{
		{
			name:    "V1 compõe o nome com IP e hora",
			version: mchlogcore.V1,
		},
		{
			name:    "V2 compõe o nome com o subject",
			version: mchlogcore.V2,
		},
		{
			name:     "V3 em modo arquivo compõe o nome como o V2",
			version:  mchlogcore.V3,
			protocol: mchlogcorev3.ProtocolFile,
		},
		{
			name:        "V3 em modo Graylog UDP devolve o descritor de rede",
			version:     mchlogcore.V3,
			protocol:    mchlogcorev3.ProtocolGraylogUDP,
			expectedUDP: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			teardownTestCase := unittest.SetupTestCase(t)
			defer teardownTestCase(t)

			restoreFacade(t)

			addr, _ := listenUDP(t)
			if testCase.protocol != "" {
				assert.NoError(mchlogcorev3.Configure(mchlogcorev3.DestinationConfig{
					Protocol: testCase.protocol,
					Addr:     addr,
					Source:   "pod-1",
				}))
			}

			mchlogcore.SetVersion(testCase.version)
			mchlogcore.InitializeMchLog(initPath)

			fileName := mchlogcore.MchLog.GetFileNameFromStreamName("teste")

			if testCase.expectedUDP {
				assert.Equal("udp://"+addr+"/teste", fileName)
				return
			}

			if testCase.version == mchlogcore.V1 {
				assert.Regexp(v1FileName, fileName)
				return
			}

			assert.Equal(filepath.Join(initPath, "teste", "teste.log"), fileName)
		})
	}
}

func TestLogSubject(t *testing.T) {
	assert, teardown := unittest.SetupTests(t, "teste")
	defer teardown()

	testCases := []struct {
		name        string
		version     mchlogcore.LogVersion
		protocol    mchlogcorev3.Protocol
		expectedUDP bool
		expected    string
	}{
		{
			name:     "V1 delega para o transporte de arquivo com rotação",
			version:  mchlogcore.V1,
			expected: `"chave":"valor"`,
		},
		{
			name:     "V2 delega para o transporte de arquivo simples",
			version:  mchlogcore.V2,
			expected: `"chave":"valor"`,
		},
		{
			name:     "V3 em modo arquivo delega para o transporte unificado",
			version:  mchlogcore.V3,
			protocol: mchlogcorev3.ProtocolFile,
			expected: `"chave":"valor"`,
		},
		{
			name:        "V3 em modo Graylog UDP envia o datagrama GELF",
			version:     mchlogcore.V3,
			protocol:    mchlogcorev3.ProtocolGraylogUDP,
			expectedUDP: true,
			expected:    `"_application_name":"servico"`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			teardownTestCase := unittest.SetupTestCase(t)
			defer teardownTestCase(t)

			restoreFacade(t)

			addr, conn := listenUDP(t)
			if testCase.protocol != "" {
				assert.NoError(mchlogcorev3.Configure(mchlogcorev3.DestinationConfig{
					Protocol: testCase.protocol,
					Addr:     addr,
					Source:   "pod-1",
				}))
			}

			mchlogcore.SetVersion(testCase.version)
			mchlogcore.InitializeMchLog(initPath)

			if testCase.expectedUDP {
				// Descarta o datagrama do log de inicialização.
				readDatagram(t, conn)
			}

			mchlogcore.MchLog.LogSubject("teste", map[string]any{"chave": "valor"}, nil)

			if testCase.expectedUDP {
				assert.Contains(readDatagram(t, conn), testCase.expected)
				return
			}

			content, err := os.ReadFile(mchlogcore.MchLog.GetFileNameFromStreamName("teste"))
			assert.NoError(err)
			assert.Contains(string(content), testCase.expected)
		})
	}
}

func TestGetIP(t *testing.T) {
	assert, teardown := unittest.SetupTests(t)
	defer teardown()

	testCases := []struct {
		name       string
		version    mchlogcore.LogVersion
		protocol   mchlogcorev3.Protocol
		expectedIP string
	}{
		{
			name:       "Delega para o V1 quando ele é o transporte ativo",
			version:    mchlogcore.V1,
			expectedIP: localIP(),
		},
		{
			name:    "Devolve vazio para o V2",
			version: mchlogcore.V2,
		},
		{
			name:     "Devolve vazio para o V3",
			version:  mchlogcore.V3,
			protocol: mchlogcorev3.ProtocolFile,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			teardownTestCase := unittest.SetupTestCase(t)
			defer teardownTestCase(t)

			restoreFacade(t)

			if testCase.protocol != "" {
				assert.NoError(mchlogcorev3.Configure(mchlogcorev3.DestinationConfig{
					Protocol: testCase.protocol,
				}))
			}

			mchlogcore.SetVersion(testCase.version)
			mchlogcore.InitializeMchLog(initPath)

			assert.Equal(testCase.expectedIP, mchlogcore.MchLog.GetIP())
		})
	}
}

func TestClose(t *testing.T) {
	assert, teardown := unittest.SetupTests(t)
	defer teardown()

	testCases := []struct {
		name     string
		version  mchlogcore.LogVersion
		protocol mchlogcorev3.Protocol
	}{
		{
			name:    "É no-op para o V1",
			version: mchlogcore.V1,
		},
		{
			name:    "É no-op para o V2",
			version: mchlogcore.V2,
		},
		{
			name:     "É no-op para o V3 em modo arquivo",
			version:  mchlogcore.V3,
			protocol: mchlogcorev3.ProtocolFile,
		},
		{
			name:     "Libera o socket do V3 em modo Graylog UDP",
			version:  mchlogcore.V3,
			protocol: mchlogcorev3.ProtocolGraylogUDP,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			teardownTestCase := unittest.SetupTestCase(t)
			defer teardownTestCase(t)

			restoreFacade(t)

			addr, _ := listenUDP(t)
			if testCase.protocol != "" {
				assert.NoError(mchlogcorev3.Configure(mchlogcorev3.DestinationConfig{
					Protocol: testCase.protocol,
					Addr:     addr,
					Source:   "pod-1",
				}))
			}

			mchlogcore.SetVersion(testCase.version)
			mchlogcore.InitializeMchLog(initPath)

			assert.NoError(mchlogcore.MchLog.Close())
			assert.NoError(mchlogcore.MchLog.Close())
		})
	}
}

func TestInitializeMchLog(t *testing.T) {
	assert, teardown := unittest.SetupTests(t)
	defer teardown()

	testCases := []struct {
		name        string
		version     mchlogcore.LogVersion
		protocol    mchlogcorev3.Protocol
		addr        string
		expectedLog string
	}{
		{
			name:        "Registra a inicialização do V1",
			version:     mchlogcore.V1,
			expectedLog: `"version":"V1"`,
		},
		{
			name:        "Registra a inicialização do V2",
			version:     mchlogcore.V2,
			expectedLog: `"version":"V2"`,
		},
		{
			name:        "Registra a inicialização do V3",
			version:     mchlogcore.V3,
			protocol:    mchlogcorev3.ProtocolFile,
			expectedLog: `"version":"V3"`,
		},
		{
			name:     "Não registra quando a inicialização do V3 falha",
			version:  mchlogcore.V3,
			protocol: mchlogcorev3.ProtocolGraylogUDP,
			addr:     "endereco-invalido",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			teardownTestCase := unittest.SetupTestCase(t)
			defer teardownTestCase(t)

			restoreFacade(t)

			addr := testCase.addr
			if addr == "" {
				addr, _ = listenUDP(t)
			}

			if testCase.protocol != "" {
				assert.NoError(mchlogcorev3.Configure(mchlogcorev3.DestinationConfig{
					Protocol: testCase.protocol,
					Addr:     addr,
					Source:   "pod-1",
				}))
			}

			mchlogcore.SetVersion(testCase.version)

			// Zera o destino do V3 para que uma inicialização que falha não
			// deixe o destino da execução anterior ativo.
			assert.NoError(mchlogcore.MchLog.Close())

			mchlogcore.InitializeMchLog(initPath)

			fileName := mchlogcore.MchLog.GetFileNameFromStreamName("info")

			if testCase.expectedLog == "" {
				assert.Equal("", fileName)
				return
			}

			content, err := os.ReadFile(fileName)
			assert.NoError(err)
			assert.Contains(string(content), `"message":"MchLogToolkit initialized"`)
			assert.Contains(string(content), testCase.expectedLog)
		})
	}
}
