package mchlogcorev1_test

import (
	"errors"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/gaudiumsoftware/mchlogtoolkitgo/mchlogcorev1"
	"github.com/gaudiumsoftware/mchlogtoolkitgo/unittest"
)

// ccDateTimeMask espelha a máscara de rotação usada pelo pacote para compor
// o nome do arquivo de log.
const ccDateTimeMask = "2006010215"

func TestMain(m *testing.M) {
	unittest.RunTests(m)
}

// expectedFileName reproduz o nome de arquivo esperado para um subject,
// independentemente da implementação, para servir de referência aos testes.
func expectedFileName(basePath, subject string) string {
	ip := localIP()
	if ip != "" {
		ip = "-" + ip
	}

	dataHora := time.Now().UTC().Format(ccDateTimeMask)

	return filepath.FromSlash(filepath.Join(basePath, subject, subject+ip+"-"+dataHora+".log"))
}

// localIP devolve o primeiro IPv4 não-loopback da máquina, ou "" caso não
// exista — mesma regra usada internamente pelo pacote.
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

func TestInitializeMchLog(t *testing.T) {
	assert, teardown := unittest.SetupTests(t, "teste")
	defer teardown()

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
		{
			name: "Reinicializa apontando para outro caminho",
			path: t.TempDir() + "/outro",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			teardownTestCase := unittest.SetupTestCase(t)
			defer teardownTestCase(t)

			mchlogcorev1.InitializeMchLog(testCase.path)

			assert.Equal(
				expectedFileName(testCase.path, "teste"),
				mchlogcorev1.MchLog.GetFileNameFromStreamName("teste"),
			)

			mchlogcorev1.MchLog.LogSubject("teste", map[string]any{"chave": "valor"}, nil)

			unittest.CompareLogs(t, assert, map[string][]string{
				"teste": {`"chave":"valor"`},
			})
		})
	}
}

func TestGetFileNameFromStreamName(t *testing.T) {
	assert, teardown := unittest.SetupTests(t)
	defer teardown()

	basePath := t.TempDir()
	mchlogcorev1.InitializeMchLog(basePath)

	testCases := []struct {
		name    string
		subject string
	}{
		{
			name:    "Compõe o nome para um subject simples",
			subject: "teste",
		},
		{
			name:    "Compõe o nome para um subject de erro",
			subject: "err_teste",
		},
		{
			name:    "Compõe o nome para um subject com separador",
			subject: "grupo/teste",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			teardownTestCase := unittest.SetupTestCase(t)
			defer teardownTestCase(t)

			assert.Equal(
				expectedFileName(basePath, testCase.subject),
				mchlogcorev1.MchLog.GetFileNameFromStreamName(testCase.subject),
			)
		})
	}
}

func TestGetIP(t *testing.T) {
	assert, teardown := unittest.SetupTests(t)
	defer teardown()

	testCases := []struct {
		name string
		path string
	}{
		{
			name: "Devolve o IP local após a inicialização",
			path: t.TempDir(),
		},
		{
			name: "Mantém o IP local após reinicializar",
			path: t.TempDir(),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			teardownTestCase := unittest.SetupTestCase(t)
			defer teardownTestCase(t)

			mchlogcorev1.InitializeMchLog(testCase.path)

			assert.Equal(localIP(), mchlogcorev1.MchLog.GetIP())
		})
	}
}

func TestLogSubject(t *testing.T) {
	assert, teardown := unittest.SetupTests(t, "teste", "err_teste")
	defer teardown()

	basePath := t.TempDir()
	mchlogcorev1.InitializeMchLog(basePath)

	testCases := []struct {
		name             string
		subject          string
		contents         []any
		errLog           error
		ascendStackFrame []int
		expectedLogs     map[string][]string
	}{
		{
			name:         "Ignora subject vazio",
			subject:      "",
			contents:     []any{map[string]any{"chave": "valor"}},
			expectedLogs: map[string][]string{},
		},
		{
			name:     "Grava um map",
			subject:  "teste",
			contents: []any{map[string]any{"mapa1": "aaa", "mapa2": 22}},
			expectedLogs: map[string][]string{
				"teste": {`{"mapa1":"aaa","mapa2":22,`},
			},
		},
		{
			name:     "Grava uma string json",
			subject:  "teste",
			contents: []any{`{"testmsg":"some msg"}`},
			expectedLogs: map[string][]string{
				"teste": {`{"testmsg":"some msg",`},
			},
		},
		{
			name:     "Grava um []byte json",
			subject:  "teste",
			contents: []any{[]byte(`{"tick":10}`)},
			expectedLogs: map[string][]string{
				"teste": {`{"tick":10,`},
			},
		},
		{
			name:     "Grava um slice de pares chave/valor",
			subject:  "teste",
			contents: []any{[]any{"key_array1", 22, "key_array2", "55"}},
			expectedLogs: map[string][]string{
				"teste": {`{"key_array1":22,"key_array2":"55",`},
			},
		},
		{
			name:     "Reaproveita o arquivo em chamadas consecutivas",
			subject:  "teste",
			contents: []any{map[string]any{"seq": 1}, map[string]any{"seq": 2}},
			expectedLogs: map[string][]string{
				"teste": {`{"seq":1,`, `{"seq":2,`},
			},
		},
		{
			name:     "Grava no subject prefixado quando há erro",
			subject:  "teste",
			contents: []any{map[string]any{"chave": "valor"}},
			errLog:   errors.New("isto é um erro forçado"),
			expectedLogs: map[string][]string{
				"err_teste": {`"error":"isto é um erro forçado"`},
			},
		},
		{
			name:             "Respeita o ascendStackFrame informado",
			subject:          "teste",
			contents:         []any{map[string]any{"chave": "valor"}},
			errLog:           errors.New("erro com stack frame"),
			ascendStackFrame: []int{2},
			expectedLogs: map[string][]string{
				"err_teste": {`"error":"erro com stack frame"`},
			},
		},
		{
			name:         "Não grava conteúdo de tipo não suportado",
			subject:      "teste",
			contents:     []any{42},
			expectedLogs: map[string][]string{},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			teardownTestCase := unittest.SetupTestCase(t)
			defer teardownTestCase(t)

			for _, content := range testCase.contents {
				mchlogcorev1.MchLog.LogSubject(testCase.subject, content, testCase.errLog, testCase.ascendStackFrame...)
			}

			unittest.CompareLogs(t, assert, testCase.expectedLogs)
		})
	}
}
