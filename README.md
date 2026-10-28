# TripGo — сервис поездок

Учебный проект курса **«Разработка микросервисов на Go»**. В одном репозитории выполняются пять последовательных лабораторных работ. Текущая ветка — `homework/02`: архитектурный рефакторинг лабораторной работы № 2 на основе работающего API первой работы. Подключены OpenTelemetry, структурированные логи и метрики лабораторной № 2.

## Реализовано в лабораторной работе № 1

- HTTP-сервер на `net/http` и `chi` с таймаутами и корректной остановкой по `SIGINT` / `SIGTERM`.
- HTTP API, типы и интерфейсы которого генерируются из OpenAPI с помощью `oapi-codegen`.
- PostgreSQL через `pgxpool`, SQL-запросы через `squirrel` с параметрами `$1`, `$2` и т. д.
- Миграции `goose` для таблиц `trips` и `trip_status_history`.
- Создание и завершение поездки вместе с записью переходов статуса в одной транзакции.
- Проверка входных данных и ошибки API в формате `application/problem+json`.
- Интеграционные тесты жизненного цикла, отката транзакций и конкурентных запросов.

## Требования

- Go **1.27.1+** (проект проверялся с Go 1.27.3).
- Docker Desktop с включённой интеграцией WSL 2 (при запуске в WSL).
- Установленная утилита `tripgoctl` и доступный Docker daemon.
- `make` и Bash; для ручных HTTP-проверок — `curl`.

Локальную инфраструктуру предоставляет [`tripgoctl`](https://github.com/course-go-autumn-2026/course-infra). Для второй работы окружение включает PostgreSQL и observability.

## Запуск из чистого клона

```bash
git clone https://github.com/epif4nov/template.git
cd template
git switch homework/02

# Первоначальная настройка кластера (один раз на машине)
tripgoctl cluster start

# Запуск окружения и получение актуальных адресов
tripgoctl environment start
tripgoctl connect

# В результате создаётся локальный .env. Он не коммитится.
make migrate
make run
```

Сервис по умолчанию слушает `:8080` (значение `HTTP_ADDR` из `.env`). Во время работы `make run` терминал занят сервером; HTTP-запросы отправляются из другого терминала. Остановка — `Ctrl+C`.

Для повторного запуска после настройки кластера достаточно запустить окружение, применить миграции и выполнить `make run`. Если PostgreSQL недоступен при старте, приложение завершится с ошибкой.

### Переменные окружения

Все параметры приложения читаются из окружения. `tripgoctl environment start` формирует `.env` с адресом локальной базы; названия остальных переменных приведены в `.env.example`.

| Переменная | Назначение |
|---|---|
| `HTTP_ADDR` | Адрес HTTP-сервера |
| `HTTP_READ_TIMEOUT`, `HTTP_READ_HEADER_TIMEOUT`, `HTTP_WRITE_TIMEOUT`, `HTTP_IDLE_TIMEOUT` | Таймауты HTTP-сервера |
| `DATABASE_URL` | Строка подключения к PostgreSQL |
| `DATABASE_MAX_CONNS`, `DATABASE_MIN_CONNS` | Границы размера пула подключений |
| `DATABASE_MAX_CONN_LIFETIME` | Максимальное время жизни подключения |
| `DATABASE_CONNECT_TIMEOUT` | Таймаут подключения к БД |
| `DATABASE_QUERY_TIMEOUT` | Таймаут операций с БД и транзакций |
| `SHUTDOWN_TIMEOUT` | Время, отведённое на остановку сервера |
| `LOG_LEVEL` | Уровень логирования |

Обязательные переменные проверяются при запуске. Реальные пароли и адреса из локального `.env` в Git не добавляются.

## HTTP API

| Метод | Путь | Успешный ответ |
|---|---|---|
| `GET` | `/health` | `200` — процесс работает (без запроса к БД) |
| `GET` | `/ready` | `200` — PostgreSQL доступен; при недоступности — `503` |
| `POST` | `/api/v1/trips` | `201` — поездка создана, возвращаются JSON и заголовок `Location` |
| `GET` | `/api/v1/trips/{tripId}` | `200` — данные поездки |
| `POST` | `/api/v1/trips/{tripId}/finish` | `200` — поездка завершена |

Пример создания поездки:

```bash
curl -i -X POST http://localhost:8080/api/v1/trips \
  -H 'Content-Type: application/json' \
  -d '{
    "user_id":"5cb72c04-7650-45c9-a79b-bcdba0631e0c",
    "driver_id":"8860b315-ec86-42eb-a17c-7c163d721ff5",
    "start_point":{"latitude":59.9398,"longitude":30.3146},
    "end_point":{"latitude":59.9290,"longitude":30.3626},
    "price":1450
  }'
```

В ответе `201 Created` содержатся UUID и статус `active`. Далее UUID поездки можно использовать для `GET` и `POST /finish`.

Типовые ошибки: `400 invalid_request` (неверный JSON, UUID, координаты или цена), `404 trip_not_found`, `409 driver_busy` (у водителя уже есть активная поездка), `409 trip_completed` (повторное завершение), `500 internal_error`. Ошибки возвращаются с `Content-Type: application/problem+json`.

## Работа с базой данных

```bash
make migrate         # применить миграции
make migrate-status  # узнать состояние
make migrate-down    # откатить последнюю миграцию
```

**Внимание:** откат таблиц удаляет данные. Проверять полный цикл `up`/`down` следует на отдельной временной базе, а не на окружении с нужными данными.

## Генерация, сборка и тестирование

```bash
make generate        # OpenAPI и gomock через go:generate
make build           # go build ./...
make test            # unit-тесты с -race -count=2 и покрытие domain/usecase
make lint            # установленный golangci-lint v2
```

Unit-тесты не требуют PostgreSQL и локального `.env`. Моки генерируются закреплённым в `go.mod` инструментом `go.uber.org/mock/mockgen`:

```bash
go generate -x ./internal/usecase/trip/... ./internal/http/trip/...
go test -race -count=2 ./...
go test -cover ./internal/domain/... ./internal/usecase/...
```

Интеграционные тесты отделены тегом `integration`. Требуется доступная PostgreSQL с применёнными миграциями:

```bash
tripgoctl environment start
tripgoctl connect
make migrate
make test-integration
```

`make test-integration` загружает `.env`, если он есть, и запускает тесты с `-race -count=2 -tags=integration`. Если `DATABASE_URL` не задана или база недоступна, тесты завершаются ошибкой, а не пропускаются. Проверяются жизненный цикл поездки через новые usecase, откат при ошибке истории, конкурентное создание и завершение, вложенные транзакции и откат при панике. Все тесты используют собственные UUID и убирают созданные данные.

Для `make lint` нужен установленный `golangci-lint` v2, собранный с поддержкой версии Go проекта. Правила зависимостей запрещают инфраструктуру и OpenAPI в бизнес-слоях, а репозитории и инфраструктуру — в HTTP-слое.

## Принятые технические решения

**Уровень изоляции — `READ COMMITTED`.** Это стандартный уровень PostgreSQL, достаточный для данной реализации при использовании ограничений БД и условного обновления. Он не требует сериализации всех транзакций и не заменяет механизмы защиты от гонок.

**Менеджер транзакций.** Интерфейс `TxManager` предоставляет `Do(ctx, fn)`. Менеджер начинает транзакцию и записывает её в `context.Context`; репозиторий использует транзакцию из контекста либо пул, если транзакции нет. При успехе выполняется `COMMIT`, при ошибке или панике — `ROLLBACK`. Вложенный `Do` использует существующую транзакцию. Бизнес-логика не передаёт объекты `pgx.Tx` напрямую.

**Атомарное создание.** `INSERT` в `trips` и `INSERT` в `trip_status_history` выполняются внутри одного `Do`. Ошибка второго запроса откатывает оба изменения.

**Один водитель — одна активная поездка.** Частичный уникальный индекс `trips_active_driver_idx` по `driver_id WHERE status = 'active'` предотвращает нарушение правила даже при параллельных запросах. Ошибка PostgreSQL `23505` для этого индекса переводится в `409 driver_busy`.

**Безопасное завершение.** `UPDATE` меняет статус только для строки с `status = 'active'`. Если затронуто ноль строк, репозиторий различает отсутствие поездки и уже завершённую поездку. Таким образом, два конкурентных завершения не перезаписывают `finished_at`.

## Архитектура

```text
cmd/trip-service/                   конфигурация, запуск, сигналы, shutdown
internal/container/                 создание и связывание конкретных зависимостей
internal/config/                    конфигурация из env
internal/domain/trip/               закрытая модель, инварианты, переходы и ошибки
internal/usecase/trip/{create,get,finish}/
                                    бизнес-операции и контракты зависимостей
internal/http/trip/{create,get,finish}/
                                    HTTP-обработчики и контракты usecase
internal/http/domainerror/          единый перевод ошибок в api.Problem
internal/http/response/             JSON, валидация тела и преобразование ответа
internal/repository/trip/           SQL и адаптация данных PostgreSQL в домен
internal/infra/postgres/transaction/ менеджер транзакций и выбор исполнителя
internal/generated/                 сгенерированный HTTP-код
contracts/openapi/                  неизменяемый контракт
migrations/                         существующие миграции goose
```

Зависимости направлены внутрь: **HTTP → интерфейс usecase → домен**. Репозиторий реализует интерфейсы, объявленные потребителями в `contract.go`; usecase не импортируют SQL, pgx, HTTP или OpenAPI. Контейнер создаёт пул, менеджер транзакций, репозиторий, usecase, обработчики и маршрутизатор. Проверка `/ready` использует интерфейс `Ping`, `/health` не обращается к базе.

`get` запрашивает поездку по UUID и возвращает доменную модель. `finish` читает поездку внутри транзакции, вызывает доменный `Finish` с подменяемым временем, выполняет условный UPDATE и записывает историю только после успешного обновления. Проигравший конкурентный запрос получает `ErrTripCompleted`; история и `finished_at` не перезаписываются. При ошибке истории или коммита результат операции не возвращается как успешный.

Поля доменной модели закрыты. `Restore` восстанавливает сохранённую идентичность и проверяет статус, координаты, цену и согласованность времён; геттер времени завершения возвращает копию. SQL-структура `repository.Trip` используется только адаптером хранения и интеграционными фикстурами. Отдельный пакет `model` не нужен: между бизнес-слоями передаётся доменная модель, на вход создания — `create.Input`.

Общий пакет `http/response` содержит только сериализацию, проверку JSON и преобразование в OpenAPI. Это небольшое дополнение к структуре задания исключает дублирование между тремя обработчиками. Старый `internal/service` удалён: все HTTP-операции выполняют новые usecase. В интеграционных тестах остался небольшой тестовый адаптер для сохранения прежних сценариев; собственной бизнес-логики в нём нет.

Доменные ошибки объявлены в домене, `%w` сохраняет их идентичность при оборачивании. Только `http/domainerror` сопоставляет ошибки с HTTP-кодами и безопасными сообщениями. Ошибки БД логируются на границе HTTP, их текст не попадает в ответ клиенту.

## Сдача работы и материалы курса

Каждая работа выполняется в отдельной ветке `homework/NN`; для проверки создаётся **Pull Request в `main` своего форка**. До завершения ревью PR не сливается. По актуальному сообщению преподавателя отдельную ссылку на PR отправлять не нужно — ревьюер найдёт его в форке.

- [Задание № 2](https://github.com/course-go-autumn-2026/course/tree/main/homework/tasks/02-architecture-and-observability)
- [Задание № 1 и критерии оценки](https://github.com/course-go-autumn-2026/course/tree/main/homework/tasks/01-http-and-postgres)
- [Общие требования](https://github.com/course-go-autumn-2026/course/blob/main/homework/docs/conventions.md)
- [Инструкция по окружению](https://github.com/course-go-autumn-2026/course/blob/main/homework/docs/getting-started.md)
- [Оценивание и порядок сдачи](https://github.com/course-go-autumn-2026/course/blob/main/homework/docs/grading.md)
- [Инфраструктура `tripgoctl`](https://github.com/course-go-autumn-2026/course-infra)

При дальнейших лабораторных работа развивается в том же репозитории, без переписывания сервиса с нуля.

## Observability (лабораторная № 2)

Сервис отправляет трейсы и метрики по OTLP. Обязательны
`OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_EXPORTER_OTLP_PROTOCOL` (`http/protobuf`
или `grpc`) и `OTEL_TRACES_SAMPLER`; пример — `parentbased_always_on`.
Для ratio-сэмплирования SDK также читает `OTEL_TRACES_SAMPLER_ARG`.
Адреса берите из `tripgoctl connect` и локального окружения. `make run` сначала загружает значения из `.env.example`, затем локальные переопределения из `.env`: это сохраняет совместимость с окружением, в котором ещё нет HTTP-таймаутов.

Ресурс: `service.name=trip-service`, `service.namespace=tripgo`,
`deployment.environment=local`; `service.version` передаётся через
`make build VERSION=...` / `make run VERSION=...` (по умолчанию Git describe,
при прямом `go run` — `dev`).
Экспорт асинхронный: недоступный Collector не блокирует бизнес-запросы.
HTTP shutdown и завершение провайдеров используют общий дедлайн
`SHUTDOWN_TIMEOUT`.

[HTTP middleware](internal/http/middleware/middleware.go) выполняются в порядке
Recover → Tracing → Logging → Metrics → chi.
Предварительный Match в отдельном chi-контексте даёт шаблон для active requests
до входа в обработчик; после маршрутизации Tracing читает окончательный
RoutePattern. Неизвестные маршруты используют `unmatched`, нестандартные методы
в метриках и именах спанов — `_OTHER`. Ответ буферизуется до выхода из цепочки:
при панике даже частично сформированный JSON заменяется безопасным Problem 500.
Это рассчитано на текущие JSON-ручки, без streaming.

[Телеметрия](internal/infra/telemetry/) содержит SDK, JSON slog handler с
trace_id/span_id (пустые вне спана), декораторы usecase и pgx tracer.
Декораторы считают и логируют создание/завершение только после успешного возврата
usecase, включая commit. Домен и usecase не импортируют OTel.
otelpgx подключён к ConnConfig пула и работает также внутри транзакций.
Его DB-гистограмма отключена: единая гистограмма на том же уровне tracer
добавляет operation, collection и error.type. SQL-параметры и текст запросов
в спаны не записываются; имена таблиц ограничены известной схемой.

Реализованы пять обязательных метрик: `http.server.request.duration` и
`db.client.operation.duration` в секундах, `http.server.active_requests`
(UpDownCounter), `tripgo.trips.created` и `tripgo.trips.completed` (Counter).
Дублирующие HTTP-метрики otelhttp отключены. Необязательная
`tripgo.trips.active` пока отсутствует: корректный общий Gauge должен читать
состояние PostgreSQL, а разность счётчиков процесса не учитывает перезапуски и
другие экземпляры сервиса.

Установка проверенной версии линтера в пользовательский каталог:

```bash
GOBIN="$HOME/.local/bin" go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0
make lint
```

Makefile добавляет этот каталог в PATH. Все обязательные линтеры и depguard
сохранены. `make generate` обновляет также моки декораторов телеметрии.

Unit-тесты проверяют шаблоны маршрутов, W3C-контекст, статусы/паники, метрики,
корреляцию логов и HTTP при недоступном OTLP для обоих протоколов.
`make test-integration` дополнительно проверяет связанные HTTP/DB-спаны
реального создания поездки, включая BEGIN, оба INSERT и COMMIT.

Для ручной проверки после `make run` создайте и завершите поездку:
найдите trace_id из JSON-лога в Jaeger, проверьте вложенные запросы к обеим
таблицам и шаблоны маршрутов. В Grafana/Prometheus найдите
`http_server_request_duration_seconds`, `http_server_active_requests`,
`db_client_operation_duration_seconds`, `tripgo_trips_created` и
`tripgo_trips_completed` (экспортер может добавлять суффиксы единиц/`_total`).
Снимки экрана для сдачи делаются с реальных интерфейсов Jaeger/Grafana.
