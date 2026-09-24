# roadmap-tool

Небольшой локальный read-only roadmap для GitHub, GitLab и Gitea.

- milestones отображаются точками на временной шкале;
- закрытый, текущий и будущие milestones визуально различаются;
- milestone раскрывается в описание и связанные issues;
- issue попадает в roadmap, только если привязан к milestone и содержит настроенный label;
- due date issues из GitLab и Gitea отображаются маленькими точками;
- настройки и цвета импортируются и экспортируются в JSON;
- токен хранится отдельно и никогда не возвращается через API или экспорт;
- база данных и frontend-фреймворк не используются.

## Быстрый запуск из исходников

Для этого варианта нужен Go 1.23 или новее.

### macOS и Linux

```bash
cp config.example.json config.json
go run ./cmd/roadmap
```

### Windows PowerShell

```powershell
Copy-Item config.example.json config.json
go run ./cmd/roadmap
```

После запуска откройте <http://127.0.0.1:8080>. Репозиторий, цвета и access token настраиваются через кнопку **Settings**.

Пока `go run` работает в терминале, приложение запущено. Для остановки нажмите `Ctrl+C` в этом же терминале.

## Сборка бинарника

Веб-интерфейс встраивается в бинарник, поэтому папки `web` и `cmd` рядом с готовым файлом не нужны. Для запуска потребуются только бинарник и `config.json`; файл `token` появится после сохранения токена через интерфейс.

### Текущая операционная система

macOS/Linux:

```bash
mkdir -p dist
go build -trimpath -ldflags="-s -w" -o dist/roadmap-tool ./cmd/roadmap
```

Windows PowerShell:

```powershell
New-Item -ItemType Directory -Force dist | Out-Null
go build -trimpath -ldflags="-s -w" -o dist\roadmap-tool.exe .\cmd\roadmap
```

### Сборка под разные ОС и архитектуры

Проект написан на чистом Go и не использует CGO, поэтому его можно кросс-компилировать. Следующие команды выполняются в Bash на macOS или Linux:

```bash
mkdir -p dist

CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o dist/roadmap-tool-linux-amd64 ./cmd/roadmap
CGO_ENABLED=0 GOOS=linux   GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o dist/roadmap-tool-linux-arm64 ./cmd/roadmap
CGO_ENABLED=0 GOOS=darwin  GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o dist/roadmap-tool-macos-amd64 ./cmd/roadmap
CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o dist/roadmap-tool-macos-arm64 ./cmd/roadmap
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o dist/roadmap-tool-windows-amd64.exe ./cmd/roadmap
CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o dist/roadmap-tool-windows-arm64.exe ./cmd/roadmap
```

Соответствие архитектур:

- `amd64` — обычные 64-битные Intel/AMD компьютеры;
- `arm64` — Apple Silicon, Windows on ARM и ARM-серверы Linux;
- `darwin` — системное имя macOS в Go.

Для сборки на Windows задайте те же переменные через `$env:GOOS`, `$env:GOARCH` и `$env:CGO_ENABLED`, затем выполните `go build`.

## Запуск готового бинарника

Сначала скопируйте пример конфигурации:

macOS/Linux:

```bash
cp config.example.json config.json
./roadmap-tool -config ./config.json -token-file ./token -listen 127.0.0.1:8080
```

Windows PowerShell:

```powershell
Copy-Item config.example.json config.json
.\roadmap-tool.exe -config .\config.json -token-file .\token -listen 127.0.0.1:8080
```

Затем откройте <http://127.0.0.1:8080>. Для остановки foreground-процесса нажмите `Ctrl+C`.

### Параметры запуска

| Параметр | Значение по умолчанию | Назначение |
| --- | --- | --- |
| `-config` | `config.json` | Путь к несекретной конфигурации |
| `-token-file` | `token` | Путь к отдельному файлу токена |
| `-listen` | `127.0.0.1:8080` | Адрес и порт HTTP-сервера |

По умолчанию сервер доступен только на текущем компьютере. Не меняйте `127.0.0.1` на `0.0.0.0`, если не хотите явно открыть приложение другим устройствам в сети: интерфейс управляет локальным access token.

## Запуск в фоне и остановка

Само приложение не устанавливается как системная служба и не имеет отдельной команды `stop`. Остановка выполняется завершением его процесса.

### macOS и Linux

Запуск в фоне с сохранением PID и логов:

```bash
nohup ./roadmap-tool -config ./config.json -token-file ./token >roadmap.log 2>&1 &
echo $! > roadmap.pid
```

Проверка состояния:

```bash
ps -p "$(cat roadmap.pid)"
```

Остановка:

```bash
kill "$(cat roadmap.pid)"
rm roadmap.pid
```

### Windows PowerShell

Запуск в фоне:

```powershell
$process = Start-Process `
  -FilePath ".\roadmap-tool.exe" `
  -ArgumentList "-config", ".\config.json", "-token-file", ".\token" `
  -RedirectStandardOutput ".\roadmap.log" `
  -RedirectStandardError ".\roadmap-error.log" `
  -PassThru
$process.Id | Set-Content .\roadmap.pid
```

Проверка состояния:

```powershell
Get-Process -Id (Get-Content .\roadmap.pid)
```

Остановка:

```powershell
Stop-Process -Id (Get-Content .\roadmap.pid)
Remove-Item .\roadmap.pid
```

## Конфигурация и токен

Все несекретные настройки находятся в `config.json` и изменяются через интерфейс. Импорт заменяет эти настройки, экспорт скачивает их в JSON. Токен при импорте и экспорте не читается и не меняется.

Токен записывается в отдельный файл с правами только для владельца. Кнопка **Disconnect** удаляет этот файл.

- GitHub использует `https://api.github.com`, если `base_url` не задан;
- GitLab использует `https://gitlab.com`, если `base_url` не задан;
- для Gitea адрес сервера обязателен.

Для публичных репозиториев токен может не понадобиться, но будут действовать ограничения API провайдера.

GitLab и Gitea предоставляют нативный due date issues. В стандартном GitHub Issues API универсального due date нет, поэтому GitHub issues отображаются в списках, но не получают отдельные точки на timeline.
