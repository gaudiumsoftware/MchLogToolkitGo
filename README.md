# MchLogToolkitGo
Ferramenta de log para Go do sistema Machine.

## Recursos disponíveis na ferramenta
- **Níveis de log**: INFO, TEST, DEBUG, WARN, ERROR e FATAL.
- **Criação de arquivos de log**: cria arquivos de log no formato JSON.
- **Criação de arquivos de log por nível**: cria arquivos de log separados por nível.
- **Criação de arquivos de log separados por hora**: cria arquivos de log separados por hora.
- **Criação de diretório padrão para arquivos de log**: os logs são sempre salvos em /applog/<nome-do-serviço>.

## Utilização nos serviços da Machine
Por ser um pacote privado no github, é necessário informar ao Go que está sendo utilizado um repositório privado.
```bash
export GOPRIVATE=github.com/gaudiumsoftware/mchlogtoolkitgo
```

Adiciona o módulo no projeto go com o comando:
```bash
go get github.com/gaudiumsoftware/mchlogtoolkitgo
```

Importa o módulo no código go:
```go
import "github.com/gaudiumsoftware/mchlogtoolkitgo"
```

Adiciona o nível de log no arquivo de configuração do serviço:
```toml
[log]
level = "debug"
```

Pega o nível de log do arquivo de configuração do serviço:
```go
level := config.GetString("log.level").(string)
```

Inicializa o logger no código para utilização:
```go
const serviceName = "service-name"
logger := mchlogtoolkitgo.NewLogger(serviceName, level)
logger.Initialize()
```
Dessa forma, os logs serão gravados por padrão no diretório /applog/service-name/*

Para alterar o diretório de logs, utilize o método SetPath:
```go
const serviceName = "service-name"
logger := mchlogtoolkitgo.NewLogger(serviceName, level)
logger.SetPath("/path/to/logs")
logger.Initialize()
```

O nível pode ser setado também posteriormente coma função SetLevel:
```go
logger.SetLevel(mchlogtoolkitgo.DebugLevel)
```

Utiliza o logger para logar mensagens:
```go
logger.Info("mensagem de informação")
logger.Test("mensagem de teste")
logger.Debug("mensagem de debug")
logger.Warn("mensagem de aviso")
logger.Error("mensagem de erro")
logger.Fatal("mensagem de erro fatal")
```
Estas chamadas criarão arquivos de logs no diretório /applog/service-name/INFO no formato:
```json
{
  "service": "service-name",
  "timestamp": "2024-08-07 15:39:23",
  "level": "INFO",
  "line": "10",
  "source": "path/service.go",
  "message": "mensagem de informação",
  "trace": ""
}

```

## Envio de logs via UDP (GELF/Graylog)

O logger suporta envio de logs via UDP no formato **GELF 1.1** (Graylog Extended Log Format), compatível com Graylog, Logstash/Kibana, Fluentd e outras plataformas de observabilidade.

### Configuração via código

```go
logger, _ := mchlogtoolkitgo.NewLogger("service-name", "info")
logger.SetUDPTarget("graylog.example.com:12201")  // Habilita envio UDP com GZIP
logger.Initialize()

defer logger.Close()  // Fecha a conexão UDP ao encerrar
```

Para desabilitar a compressão GZIP ou a saída em arquivo:
```go
logger.SetUDPTargetWithOptions("graylog.example.com:12201", false)  // Sem GZIP
logger.DisableFileOutput()  // Somente UDP, sem arquivos locais
```

### Configuração via variáveis de ambiente

O logger detecta automaticamente as seguintes variáveis de ambiente durante o `Initialize()`:

| Variável | Descrição | Exemplo |
|---|---|---|
| `MCHLOG_UDP_TARGET` | Endereço do servidor GELF (host:porta) | `graylog.example.com:12201` |
| `MCHLOG_UDP_COMPRESS` | Compressão GZIP (`true`/`false`, padrão: `true`) | `false` |
| `MCHLOG_FILE_OUTPUT` | Saída em arquivo (`true`/`false`, padrão: `true`) | `false` |

Exemplo de uso com variáveis de ambiente:
```bash
export MCHLOG_UDP_TARGET=graylog.example.com:12201
export MCHLOG_UDP_COMPRESS=true
export MCHLOG_FILE_OUTPUT=false
```

Com as variáveis definidas, o `Initialize()` configura o envio UDP automaticamente:
```go
logger, _ := mchlogtoolkitgo.NewLogger("service-name", "info")
logger.Initialize()  // Detecta MCHLOG_UDP_TARGET e configura UDP
defer logger.Close()
```

### Formato da mensagem GELF

As mensagens são enviadas no formato GELF 1.1:
```json
{
  "version": "1.1",
  "host": "nome-do-host",
  "short_message": "mensagem de informação",
  "timestamp": 1711540800.123,
  "level": 6,
  "_source": "path/service.go",
  "_line": "42",
  "_trace": ""
}
```

O campo `level` segue o padrão syslog: Emergency (0), Error (3), Warning (4), Informational (6), Debug (7).

### Modos de operação

| Modo | Arquivo | UDP | Configuração |
|---|---|---|---|
| Somente arquivo (padrão) | sim | nao | Nenhuma configuração adicional |
| Arquivo + UDP | sim | sim | Definir `MCHLOG_UDP_TARGET` |
| Somente UDP | nao | sim | Definir `MCHLOG_UDP_TARGET` e `MCHLOG_FILE_OUTPUT=false` |

---

## Boas práticas de logs
Nesta seção são apresentados exemplos de bons e maus usos de logs.

### Bons usos
- **Logs de informações**: são úteis para informar o que está acontecendo no sistema.
> Exemplo: na função main, ao realizar a configurar e iniciar a aplicação:
```go
logger.Info("Aplicação iniciada e ouvindo na porta 80")
```
- **Logs de debug**: são úteis para informar detalhes do sistema que podem ser úteis para depurar problemas.
> Exemplo: numa função de busca no redis, ao realizar a busca printa o valor para verificar se está correto:
```go
value, err := redisClient.Get("key").Result()
logger.Debug("Valor encontrado no redis: ", value)
```
- **Logs de teste**: são úteis para informar a respeito da execução de testes.
> Exemplo: numa função de teste, ao realizar uma requisição de teste, informa o valor do parâmetro recebido:
```go
if val, ok := params["test"]; ok {
  logger.Test("Teste de requisição: ", val)
}
``` 

- **Logs de warning**: são aqueles utilizados para informar mensagens de aviso, ou seja, situações que não comprometem o funcionamento do sistema mas que devem ser tratadas com atenção.
> Exemplo: numa função que faz uma busca no redis e não consegue encontrar o dado, é importante informar que o dado não foi encontrado e continuar fazendo uma busca no mysql:
```go
value, err := redisClient.Get("key").Result()
if err != nil {
  logger.Warn("Valor não encontrado no redis, buscando no mysql")
  value, err = mysqlClient.Query("SELECT key FROM table ...")
  ...
}
...
```
- **Logs de erro**: são utilizados para informar erros que podem comprometer o funcionamento do sistema.
> Exemplo: numa função que faz uma busca no mysql porém não consegue se conectar ao banco:
```go
value, err := mysqlClient.Query("SELECT * FROM table ...")
if err != nil {
  logger.Error("Erro ao buscar dados no mysql: ", err)
  return err
}
```

- **Logs de erro fatal**: são utilizados para informar erros que definitivamente comprometem o funcionamento do sistema.
> Exemplo: numa função que faz a conexão com o mysql e falha, é importante informar o erro e encerrar a aplicação:
```go
db, err := sql.Open("mysql", MySQLDNS)
if err != nil {
    logger.Fatal("Error connecting to mysql: " + err.Error())
    os.Exit(1)
}
```