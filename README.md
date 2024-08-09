# MchLogToolkitGo
Ferramenta de log para Go do sistema Machine.

## Recursos disponíveis na ferramenta
- **Níveis de log**: INFO, DEBUG, WARN e ERROR.
- **Criação de arquivos de log**: cria arquivos de log no formato JSON.
- **Criação de arquivos de log por nível**: cria arquivos de log separados por nível.
- **Criação de arquivos de log separados por hora**: cria arquivos de log separados por hora.
- **Criação de diretório padrão para arquivos de log**: os logs são sempre salvos em /applog/<nome-do-serviço>.

## Utilização nos serviços da Machine
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
level := config.GetString("log.level")
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

Utiliza o logger para logar mensagens:
```go
logger.Info("mensagem de informação")
logger.Debug("mensagem de debug")
logger.Warn("mensagem de aviso")
logger.Error("mensagem de erro")
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
Exemplo na função main, ao realizar a configurar e iniciar a aplicação:
```go
logger.Info("Aplicação iniciada e ouvindo na porta 80")
```
- **Logs de debug**: são úteis para informar detalhes do sistema que podem ser úteis para depurar problemas.
Exemplo numa função de busca no redis, ao realizar a busca printa o valor para verificar se está correto:
```go
value, err := redisClient.Get("key").Result()
logger.Debug("Valor encontrado no redis: ", value)
```