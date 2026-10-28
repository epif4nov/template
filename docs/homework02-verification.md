# ЛР2: протокол финальной проверки

Проверено 28.10.2026, Ubuntu/WSL, ветка homework/02. Исходный HEAD: 718dbf8. В начале дерево было чистым: реализация OpenTelemetry уже была в HEAD. В ходе проверки новые коммиты, staging, push и PR не выполнялись.

## Официальные требования

Сопоставлены [задание](https://github.com/course-go-autumn-2026/course/tree/main/homework/tasks/02-architecture-and-observability), [conventions](https://github.com/course-go-autumn-2026/course/blob/main/homework/docs/conventions.md) и [каталог сигналов](https://github.com/course-go-autumn-2026/course/blob/main/homework/docs/observability.md).

Обязательная часть проверена. Единственное найденное функциональное расхождение — отсутствие типизированной ошибки валидации — исправлено: ValidationError содержит Field/Reason, не сохраняет входные значения, поддерживает errors.Is и errors.As. Маппер возвращает 400 invalid_request с безопасной причиной. Добавлены тесты типа ошибки и HTTP-маппинга. OpenAPI и миграции не редактировались.

## Окружение

`tripgoctl environment start` завершился успешно: postgres, otel-collector, jaeger, prometheus, grafana и loki — 6/6 ready. Docker Server 29.4.3 доступен из WSL.

Адреса из `tripgoctl connect` этого запуска:

| Компонент | Адрес |
|---|---|
| PostgreSQL | localhost:22032 |
| OTel HTTP / gRPC | localhost:22318 / localhost:22317 |
| Jaeger | http://localhost:22686 |
| Prometheus | http://localhost:22909 |
| Grafana | http://localhost:22300 |

`make migrate-status` и чтение goose_db_version непосредственно в БД подтвердили версии 1 и 2 с is_applied=true. Миграции повторно не изменялись и не откатывались. Prometheus /-/ready и Grafana /api/health успешны; scrape Collector имеет health=up.

Первичный запуск sandbox-команд не работал из-за ошибки helper; проверки выполнены разрешёнными командами вне sandbox через WSL. Windows Git отказал из-за владельца WSL-файлов; использовался Linux Git, глобальные настройки не менялись.

## Команды качества

Финальная последовательность после исправления:

```bash
make generate
make lint
make test
make test-integration
go build ./...
git diff --check
git status --short --untracked-files=all
```

Все команды завершились успешно. Генерация не изменила моки и сгенерированный API. Линтер: golangci-lint 2.15.0, собранный Go 1.27.3, результат `0 issues.`

`make test` выполняет `CGO_ENABLED=1 go test -race -count=2 ./...`, затем отдельный сбор покрытия. `make test-integration` выполняет `CGO_ENABLED=1 go test -race -count=2 -tags=integration ./internal/repository/trip/...` с реальной БД из локального окружения. Все сценарии прошли дважды, сообщений race detector нет. Отдельный сбор покрытия в Makefile выполняется без -race; сами эти пакеты уже проверены с -race в первом прогоне.

| Пакет | Покрытие |
|---|---|
| internal/domain/trip | 97,8% |
| internal/usecase/trip/create | 100% |
| internal/usecase/trip/get | 100% |
| internal/usecase/trip/finish | 100% |

## Реальные HTTP-запросы

Выполнен [curl-сценарий](homework02-demo.sh), в том числе на итоговом бинарнике после исправления:

| Сценарий | Получено |
|---|---|
| GET /health, GET /ready | 200, status=ok |
| POST /api/v1/trips | 201, status=active, корректный Location |
| Повторное создание для занятого водителя | 409 driver_busy |
| GET созданной поездки | 200 |
| POST finish | 200, status=completed, finished_at заполнено |
| Повторный finish | 409 trip_completed |
| GET и finish несуществующей поездки | 404 trip_not_found |
| Невалидный JSON и UUID | 400 invalid_request |
| Отрицательная цена | 400, detail=price: must be non-negative |
| Широта 91 | 400, detail с полем start_point и допустимыми диапазонами |

У ошибок проверен application/problem+json; у известных бизнес-ошибок проверены code. UUID поездки итогового прогона: `18ed0c6a-506e-4726-a45a-46fb2e0e3d51`. Демонстрационные поездки созданы с новыми случайными user/driver UUID и завершены; это реальные данные локальной БД.

Ошибка репозитория → безопасный 500 проверена unit-тестами обработчиков и mapper, без искусственного повреждения PostgreSQL. Атомарный rollback, гонки, вложенные транзакции и rollback при панике проверены integration-тестами.

## Реальные трейсы и корреляция

В Jaeger появился service=trip-service. По trace_id из stdout итогового бинарника открыт трейс:

```json
{"msg":"request handled","service":"trip-service","method":"POST","route":"/api/v1/trips","status":201,"trace_id":"57207082a6626987a13121100b1c3e89","span_id":"84c08f6f6a696aa3"}
```

[Локальный трейс](http://localhost:22686/trace/57207082a6626987a13121100b1c3e89): HTTP POST /api/v1/trips, BEGIN, INSERT trips, INSERT trip_status_history, COMMIT. Ещё два дочерних спана — prepare SQL при первом использовании. В UI всего 7 спанов. Трейс появился после остановки короткого процесса: это также подтверждает flush batch exporter при shutdown.

Для GET и finish имена HTTP-спанов и http.route используют /api/v1/trips/{tripId} и /api/v1/trips/{tripId}/finish. Это подтверждено JSON-логами, Jaeger API и HTTP-метками Prometheus.

## Реальные метрики

Получены через Prometheus API и Grafana Explore / Prometheus:

| Экспортированное имя | Наблюдение после первого сценария |
|---|---|
| http_server_request_duration_seconds_count | POST /api/v1/trips 201 = 1; ошибки 400/404/409 представлены отдельными сериями |
| http_server_request_duration_seconds_bucket / _sum | Присутствуют |
| http_server_active_requests | Все проверенные маршруты = 0 после ответа |
| db_client_operation_duration_seconds_count | INSERT trips = 1, UPDATE trips = 1, INSERT history = 2; BEGIN/COMMIT/ROLLBACK и SELECT присутствуют |
| db_client_operation_duration_seconds_bucket / _sum | Присутствуют |
| tripgo_trips_created_total | 1 |
| tripgo_trips_completed_total | 1 |

DB INSERT для занятого водителя имеет error_type=23505. В labels нет UUID, только шаблоны HTTP, тип операции, известная таблица и тип ошибки плюс служебные labels Collector.

Имена из SDK преобразованы экспортером: точки → подчёркивания, единицы секунд → _seconds, Counter → _total. Значения относятся к конкретному процессу; при повторных запусках и перезапуске Collector видны сбросы счётчиков. Первый экспорт метрик по умолчанию может занять 60 секунд плюс 5 секунд scrape; начальный пустой instant-запрос не считался успешной проверкой.

## Collector недоступен

Реально выполнено только в namespace tripgo-lab-02:

```bash
kubectl -n tripgo-lab-02 scale deployment/otel-collector --replicas=0
kubectl -n tripgo-lab-02 wait --for=delete pod \
  -l app.kubernetes.io/name=otel-collector --timeout=60s
# Collector 0/0, PostgreSQL 1/1:
bash docs/homework02-demo.sh
# восстановление через EXIT trap:
kubectl -n tripgo-lab-02 scale deployment/otel-collector --replicas=1
kubectl -n tripgo-lab-02 rollout status deployment/otel-collector --timeout=120s
```

После полного удаления pod Collector сценарий прошёл: health/ready=200, создание=201, завершение=200, ожидаемые бизнес-ошибки. В stdout появилась ошибка экспорта connection reset by peer, HTTP продолжал отвечать. Collector восстановлен до 1/1, PostgreSQL и другие окружения не останавливались. После теста выполнен tripgoctl connect.

## Shutdown итогового бинарника

Бинарник собран во временный каталог, запущен с обычной конфигурацией и PGAPPNAME=tripgo-hw02-verification. После реального HTTP-сценария:

```text
PostgreSQL connections before shutdown: 3
SIGTERM exit=0; elapsed=0s
PostgreSQL connections after shutdown: 0
```

В stdout: shutdown signal received → HTTP server stopped. HTTP-порт перестал отвечать; telemetry shutdown failed отсутствует. Jaeger получил трейс короткого процесса, Prometheus получил финальные метрики. Проверен штатный SIGTERM без выполняющегося в этот момент долгого запроса; сценарий истечения общего дедлайна под нагрузкой отдельно не запускался.

## Архитектура и безопасность Git

- Domain/usecase не импортируют HTTP, chi, pgx, squirrel или OpenAPI; domain также не импортирует OTel. Проверено чтением кода, go list -deps и depguard.
- Контракты у потребителей, обработчики вызывают usecase, конкретные реализации связывает container.
- Телеметрия добавлена HTTP-middleware, декораторами usecase и tracer пула; репозитории и бизнес-операции не инструментированы вручную.
- Ошибки маппятся централизованно; SQL выполняется параметризованно через репозитории и транзакции.
- Ветка homework/02; .env игнорируется и не отслеживается, index пуст.
- `git diff homework/01 -- contracts/openapi migrations` пуст: контракт и миграции сохранены с ЛР1.
- Временные бинарники, логи и вспомогательные скрипты находятся в /tmp, в рабочем дереве остаются только нужные исходники, тесты и материалы docs.

## Скриншоты

- [Jaeger — итоговый HTTP-трейс с дочерними DB-спанами](screenshots/jaeger-create-trip.png).
- [Grafana — created/completed](screenshots/grafana-business-metrics.png).
- [Grafana — HTTP/DB/active requests](screenshots/grafana-http-db-metrics.png).

Это захваты реальных интерфейсов; данные и интерфейсы не генерировались. Показаны результаты запросов Explore, постоянный dashboard не создавался.

## Что осталось вручную

Проверить локальный diff, зафиксировать нужные изменения, отправить homework/02 в свой форк и создать PR в main с [готовым описанием](homework02-pr.md). Эти действия намеренно не выполнялись. Перед показом снова запустить `tripgoctl connect`, `make run` и curl-сценарий; сервис после проверки штатно остановлен, окружение сохранено.

Рекомендуемые отдельные коммиты оставшихся локальных изменений:
1. fix: return typed trip validation errors — домен, два usecase, mapper и новые тесты.
2. docs: record HW02 verification and observability evidence — README, curl-сценарий, протокол, PR, защита и три PNG.

Мутационное тестирование — дополнительное задание со звёздочкой, не выполнялось.
## Изменённые и новые файлы этого сеанса

- [README.md](../README.md).
- [errors.go](../internal/domain/trip/errors.go), [trip.go](../internal/domain/trip/trip.go), [validation_test.go домена](../internal/domain/trip/validation_test.go).
- [domainerror.go](../internal/http/domainerror/domainerror.go), [validation_test.go mapper](../internal/http/domainerror/validation_test.go).
- [finish/usecase.go](../internal/usecase/trip/finish/usecase.go), [get/usecase.go](../internal/usecase/trip/get/usecase.go).
- [homework02-demo.sh](homework02-demo.sh), [homework02-pr.md](homework02-pr.md), [homework02-defense.md](homework02-defense.md), [этот протокол](homework02-verification.md).
- [jaeger-create-trip.png](screenshots/jaeger-create-trip.png), [grafana-business-metrics.png](screenshots/grafana-business-metrics.png), [grafana-http-db-metrics.png](screenshots/grafana-http-db-metrics.png).

Всего 15 файлов: 6 изменённых отслеживаемых и 9 новых. Все изменения остаются локально.