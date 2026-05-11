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

## V3 - Destino unificado (arquivo ou Graylog)
A V3 é o destino unificado da toolkit. O serviço escolhe entre **arquivo** (mesmo layout do V2) e **GELF UDP** (Graylog) configurando `DestinationConfig.Protocol`. A API do `Logger` não muda — serviços que ainda usam V1 (default) ou V2 seguem funcionando sem alteração.

A V3 é a forma recomendada daqui em diante. V1 e V2 continuam disponíveis para retrocompatibilidade enquanto serviços migram.

### Modo arquivo (`ProtocolFile`)
Comportamento idêntico ao V2: layout `<basePath>/<service>/<level>/<level>.log`, mesma JSON shape (`message`, `level`, `source`, `line`, `trace`, `timestamp`).
```go
import (
    mchlogtoolkitgo "github.com/gaudiumsoftware/mchlogtoolkitgo"
    "github.com/gaudiumsoftware/mchlogtoolkitgo/mchlogcore"
    "github.com/gaudiumsoftware/mchlogtoolkitgo/mchlogcorev3"
)

func main() {
    if err := mchlogcorev3.Configure(mchlogcorev3.DestinationConfig{
        Protocol: mchlogcorev3.ProtocolFile,
    }); err != nil {
        panic(err)
    }
    mchlogcore.SetVersion(mchlogcore.V3)

    logger, _ := mchlogtoolkitgo.NewLogger("payments-api", "info")
    logger.Initialize() // grava em /applog/payments-api/...
    logger.Info("aplicação iniciada")
}
```

### Modo Graylog UDP (`ProtocolGraylogUDP`)
Para `dev`/`qa` que centralizam logs no Graylog em vez de arquivo local:
```go
import (
    "os"

    mchlogtoolkitgo "github.com/gaudiumsoftware/mchlogtoolkitgo"
    "github.com/gaudiumsoftware/mchlogtoolkitgo/mchlogcore"
    "github.com/gaudiumsoftware/mchlogtoolkitgo/mchlogcorev3"
)

func main() {
    if err := mchlogcorev3.Configure(mchlogcorev3.DestinationConfig{
        Protocol: mchlogcorev3.ProtocolGraylogUDP,
        Addr:     "graylog.dev.internal:12201",
        Source:   "payments-api-qa-" + os.Getenv("POD_NAME"),
        // DisableGZIP: true, // opcional, default = compressão habilitada
    }); err != nil {
        panic(err)
    }
    mchlogcore.SetVersion(mchlogcore.V3)

    logger, _ := mchlogtoolkitgo.NewLogger("payments-api", "debug")
    logger.Initialize()
    logger.Info("aplicação iniciada e ouvindo na porta 80")
}
```

### Campos do `DestinationConfig`
| Campo         | Obrigatório quando…           | Descrição                                                                            |
|---------------|--------------------------------|--------------------------------------------------------------------------------------|
| `Protocol`    | —                              | `ProtocolFile` (default) ou `ProtocolGraylogUDP`.                                    |
| `Addr`        | `Protocol = ProtocolGraylogUDP`| Endereço do Graylog no formato `host:porta`.                                         |
| `Source`      | `Protocol = ProtocolGraylogUDP`| Valor do campo GELF `host` (coluna `source` no Graylog). **Fornecido pelo serviço** — a toolkit não autodetecta. Ex.: `payments-api-qa-pod-7f8d2`. Use `mchlogcorev3.DefaultSource()` se quiser apenas o hostname. |
| `DisableGZIP` | nunca (opcional)               | Default `false` (gzip habilitado). Aplica só ao `ProtocolGraylogUDP`.                |

### Como aparece no Graylog (modo `ProtocolGraylogUDP`)
| GELF field          | Origem                                       | Coluna/campo no Graylog |
|---------------------|----------------------------------------------|-------------------------|
| `host`              | `cfg.Source`                                 | `source` (default)      |
| `short_message`     | chave `message` do payload                   | `message` (default)     |
| `level`             | severity syslog (info=6, debug=7, warn=4, error=3, fatal=2) | `level`                 |
| `_application_name` | parâmetro `service` de `NewLogger`           | `application_name`      |
| `_log_id`           | `<service>-mchlog-<level>` (espelha pasta dos arquivos) | `log_id`                |
| `_level_name`       | level em texto                               | `level_name`            |
| `_file`, `_line`    | `runtime.Caller`                             | `file`, `line`          |
| `_error`            | `errLog.Error()`, quando presente            | `error`                 |

Exemplos de busca:
- `application_name:payments-api AND level:<=3` — erros de um serviço.
- `log_id:payments-api-mchlog-info` — equivale ao arquivo `INFO`.
- `source:*-qa-*` — todos os pods de QA (env embutido em `Source` pelo caller).

### Falhas de envio (modo `ProtocolGraylogUDP`)
UDP é fire-and-forget. Se o destino estiver inacessível, a toolkit
**descarta a mensagem silenciosamente** e emite no máximo **uma linha em
`stderr` a cada 60s** (`mchlogcorev3: GELF UDP send failed: ...`).
Não há fallback automático para arquivo.

### Quando usar cada modo
- **Produção**: `ProtocolFile` (ou seguir em V1/V2). Arquivos persistidos em `/applog/<service>/...` são a fonte de verdade.
- **Dev/QA**: `ProtocolGraylogUDP` para concentrar logs no Graylog.

### Migração de V1/V2 para V3
Trocar `mchlogcore.SetVersion(mchlogcore.V2)` por `Configure(DestinationConfig{Protocol: ProtocolFile}) + SetVersion(V3)` mantém o comportamento bit-a-bit (mesmo layout, mesma JSON shape).
V1 e V2 seguem disponíveis até a próxima onda de migração.