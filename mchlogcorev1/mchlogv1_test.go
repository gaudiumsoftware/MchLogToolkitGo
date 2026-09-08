package mchlogcorev1

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_assert "github.com/stretchr/testify/assert"
)

// Os testes NÃO usam t.Parallel: MchLog, _chLog e o cache mapLogger são
// globais do pacote e seriam compartilhados entre casos paralelos.

// subjectSeq garante subjects únicos no binário de teste, já que o cache
// mapLogger é keyed por subject e sobrevive ao fim de cada caso.
var subjectSeq int

func uniqueSubject(prefix string) string {
	subjectSeq++
	return fmt.Sprintf("%s-%d", prefix, subjectSeq)
}

// expectedFileName reproduz o nome de arquivo esperado para um subject,
// servindo de referência independente da implementação.
func expectedFileName(basePath, subject string) string {
	ip := localIP()
	if ip != "" {
		ip = "-" + ip
	}

	dataHora := time.Now().UTC().Format(ccDateTimeMask)

	return filepath.FromSlash(filepath.Join(basePath, subject, subject+ip+"-"+dataHora+ccLogFileSuffix))
}

// localIP devolve o primeiro IPv4 não-loopback da máquina — mesma regra de
// getLocalIP, reescrita aqui para servir de referência ao teste.
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

// readLogLines devolve as linhas gravadas no arquivo de log, ou nil quando o
// arquivo não existe.
func readLogLines(t *testing.T, fileName string) []string {
	t.Helper()

	content, err := os.ReadFile(fileName)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("Error reading the log file: %v", err)
	}

	trimmed := strings.TrimRight(string(content), "\n")
	if trimmed == "" {
		return nil
	}

	return strings.Split(trimmed, "\n")
}

func TestCloseFile(t *testing.T) {
	assert := _assert.New(t)

	testCases := []struct {
		name        string
		withFile    bool
		closeBefore bool
	}{
		{
			name: "Ignora o descritor não aberto",
		},
		{
			name:     "Fecha o arquivo aberto",
			withFile: true,
		},
		{
			name:        "É seguro para um arquivo já fechado",
			withFile:    true,
			closeBefore: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			fileLog := fileLogType{filename: filepath.Join(t.TempDir(), "teste.log")}

			if testCase.withFile {
				file, err := os.Create(fileLog.filename)
				assert.NoError(err)
				fileLog.file = file

				if testCase.closeBefore {
					assert.NoError(file.Close())
				}
			}

			assert.NotPanics(fileLog.closeFile)

			if testCase.withFile && !testCase.closeBefore {
				_, err := fileLog.file.WriteString("x")
				assert.Error(err)
			}
		})
	}
}

func TestMkDir(t *testing.T) {
	assert := _assert.New(t)

	testCases := []struct {
		name        string
		subDir      string
		blockedBy   string
		expectError bool
	}{
		{
			name:   "Cria o diretório inexistente",
			subDir: "novo",
		},
		{
			name:   "Cria a árvore de diretórios inexistente",
			subDir: "a/b/c",
		},
		{
			name: "Não faz nada quando o diretório já existe",
		},
		{
			name:        "Devolve erro quando o caminho está ocupado por um arquivo",
			subDir:      "arquivo/sub",
			blockedBy:   "arquivo",
			expectError: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			basePath := t.TempDir()

			if testCase.blockedBy != "" {
				assert.NoError(os.WriteFile(filepath.Join(basePath, testCase.blockedBy), []byte(""), 0644))
			}

			fileLog := fileLogType{filename: filepath.Join(basePath, testCase.subDir, "teste.log")}

			err := fileLog.mkDir()

			if testCase.expectError {
				assert.Error(err)
				return
			}

			assert.NoError(err)
			assert.DirExists(filepath.Dir(fileLog.filename))
		})
	}
}

func TestGetLocalIP(t *testing.T) {
	assert := _assert.New(t)

	testCases := []struct {
		name string
	}{
		{
			name: "Devolve o IPv4 não-loopback da máquina",
		},
		{
			name: "É estável entre chamadas",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			assert.Equal(localIP(), getLocalIP())
			assert.NotContains(getLocalIP(), "127.0.0.1")
		})
	}
}

func TestInitializeMchLog(t *testing.T) {
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
		{
			name: "Reinicializa apontando para outro caminho",
			path: t.TempDir() + "/outro",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			InitializeMchLog(testCase.path)

			assert.Equal(filepath.FromSlash(testCase.path), MchLog.path)
			assert.Equal(localIP(), MchLog.ip)

			subject := uniqueSubject("init")
			MchLog.LogSubject(subject, map[string]any{"chave": "valor"}, nil)

			lines := readLogLines(t, expectedFileName(testCase.path, subject))
			assert.Len(lines, 1)
			assert.Contains(lines[0], `"chave":"valor"`)
		})
	}
}

func TestGetFileNameFromStreamName(t *testing.T) {
	assert := _assert.New(t)

	basePath := t.TempDir()
	InitializeMchLog(basePath)

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
			subject: ccLogErrPrefixSubject + "teste",
		},
		{
			name:    "Compõe o nome para um subject com separador",
			subject: "grupo/teste",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			assert.Equal(
				expectedFileName(basePath, testCase.subject),
				MchLog.GetFileNameFromStreamName(testCase.subject),
			)
		})
	}
}

func TestGetIP(t *testing.T) {
	assert := _assert.New(t)

	testCases := []struct {
		name string
	}{
		{
			name: "Devolve o IP local após a inicialização",
		},
		{
			name: "Mantém o IP local após reinicializar",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			InitializeMchLog(t.TempDir())

			assert.Equal(localIP(), MchLog.GetIP())
		})
	}
}

func TestCheckFile(t *testing.T) {
	assert := _assert.New(t)

	basePath := t.TempDir()
	InitializeMchLog(basePath)

	// Diretório ocupado por um arquivo comum: o OpenFile do arquivo de log falha.
	blocked := uniqueSubject("bloqueado")
	assert.NoError(os.WriteFile(filepath.Join(basePath, blocked), []byte(""), 0644))

	testCases := []struct {
		name         string
		subject      string
		callTwice    bool
		reinitialize bool
		expectError  bool
	}{
		{
			name:    "Cria o logger na primeira chamada",
			subject: uniqueSubject("check"),
		},
		{
			name:      "Reaproveita o logger em cache",
			subject:   uniqueSubject("check"),
			callTwice: true,
		},
		{
			name:         "Recria o logger quando o caminho muda",
			subject:      uniqueSubject("check"),
			reinitialize: true,
		},
		{
			name:        "Propaga o erro quando o arquivo não pode ser aberto",
			subject:     blocked,
			expectError: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			logger, err := MchLog.checkFile(testCase.subject)

			if testCase.expectError {
				assert.Error(err)
				assert.Nil(logger)
				return
			}

			assert.NoError(err)
			assert.NotNil(logger)

			if testCase.callTwice {
				cached, err := MchLog.checkFile(testCase.subject)
				assert.NoError(err)
				assert.Same(logger, cached)
			}

			if testCase.reinitialize {
				InitializeMchLog(t.TempDir())
				t.Cleanup(func() { InitializeMchLog(basePath) })

				recreated, err := MchLog.checkFile(testCase.subject)
				assert.NoError(err)
				assert.NotSame(logger, recreated)
			}
		})
	}
}

func TestLogSubject(t *testing.T) {
	assert := _assert.New(t)

	basePath := t.TempDir()
	InitializeMchLog(basePath)

	testCases := []struct {
		name             string
		emptySubject     bool
		contents         []any
		errLog           error
		ascendStackFrame []int
		expectedLines    []string
	}{
		{
			name:         "Ignora subject vazio",
			emptySubject: true,
			contents:     []any{map[string]any{"chave": "valor"}},
		},
		{
			name:          "Grava um map",
			contents:      []any{map[string]any{"mapa1": "aaa", "mapa2": 22}},
			expectedLines: []string{`{"mapa1":"aaa","mapa2":22,`},
		},
		{
			name:          "Grava uma string json",
			contents:      []any{`{"testmsg":"some msg"}`},
			expectedLines: []string{`{"testmsg":"some msg",`},
		},
		{
			name:          "Grava um []byte json",
			contents:      []any{[]byte(`{"tick":10}`)},
			expectedLines: []string{`{"tick":10,`},
		},
		{
			name:          "Grava um slice de pares chave/valor",
			contents:      []any{[]any{"key_array1", 22, "key_array2", "55"}},
			expectedLines: []string{`{"key_array1":22,"key_array2":"55",`},
		},
		{
			name:          "Reaproveita o arquivo em chamadas consecutivas",
			contents:      []any{map[string]any{"seq": 1}, map[string]any{"seq": 2}},
			expectedLines: []string{`{"seq":1,`, `{"seq":2,`},
		},
		{
			name:          "Grava no subject prefixado quando há erro",
			contents:      []any{map[string]any{"chave": "valor"}},
			errLog:        errors.New("isto é um erro forçado"),
			expectedLines: []string{`"error":"isto é um erro forçado"`},
		},
		{
			name:             "Respeita o ascendStackFrame informado",
			contents:         []any{map[string]any{"chave": "valor"}},
			errLog:           errors.New("erro com stack frame"),
			ascendStackFrame: []int{2},
			expectedLines:    []string{`"error":"erro com stack frame"`},
		},
		{
			name:     "Não grava conteúdo de tipo não suportado",
			contents: []any{42},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			subject := uniqueSubject("log")
			if testCase.emptySubject {
				subject = ""
			}

			for _, content := range testCase.contents {
				MchLog.LogSubject(subject, content, testCase.errLog, testCase.ascendStackFrame...)
			}

			fileSubject := subject
			if testCase.errLog != nil {
				fileSubject = ccLogErrPrefixSubject + subject
			}

			lines := readLogLines(t, expectedFileName(basePath, fileSubject))
			assert.Len(lines, len(testCase.expectedLines))

			for i, expected := range testCase.expectedLines {
				assert.Contains(lines[i], expected)
				assert.Contains(lines[i], `"`+ccLogDataHora+`":"`+time.Now().UTC().Format("2006-01-02 15:04:05"))
			}
		})
	}
}
