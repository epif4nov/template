# HW02: слои приложения, тесты и OpenTelemetry

## Цель

Сохранить HTTP API ЛР1 и отделить бизнес-правила от транспорта и PostgreSQL; добавить проверяемые тестами сценарии, строгий lint и observability. Контракт OpenAPI и обе миграции совпадают с homework/01.

## Слои

```text
HTTP handler → интерфейс usecase → usecase → domain/trip
                                   ↓ интерфейсы потребителя
                            repository + TxManager → PostgreSQL
container → связывает конкретные зависимости
HTTP middleware + pgx tracer + декораторы usecase → OTLP Collector
                                                     ├─ Jaeger
                                                     └─ Prometheus → Grafana
```

- Каждый create/get/finish имеет отдельный пакет и узкий contract.go на стороне потребителя. Сборка зависимостей — вручную в container, запуск и сигналы — в main.
- Доменная модель с закрытыми полями и переходом Finish; типизированная ValidationError сообщает поле/причину и сохраняет errors.Is(ErrInvalidTrip).
- Единый транспортный mapper доменных ошибок в application/problem+json; детали внутренних ошибок клиенту не передаются.
- Создание/завершение и история атомарны через Do. Частичный уникальный индекс и условный UPDATE защищают конкурентные запросы. Вложенные транзакции используют текущую; ошибка/паника откатывают изменения.
- Общие HTTP-преобразования вынесены в response; отдельная model не нужна, между бизнес-слоями передаётся доменная модель. Эти отклонения от примерной структуры объяснены в README.

## Проверка

Реально выполнено в Ubuntu/WSL, Go 1.27.3, PostgreSQL окружения tripgo-lab-02:

| Проверка | Результат |
|---|---|
| make generate | Успех; повторная генерация не изменила артефакты |
| make lint | 0 issues, golangci-lint 2.15.0 |
| make test | Успех, основной прогон -race -count=2 |
| Покрытие domain/trip | 97,8% |
| Покрытие create / get / finish | 100% / 100% / 100% |
| make test-integration | Успех, -race -count=2, реальный PostgreSQL |
| go build ./... | Успех |
| git diff --check | Успех |
| goose migrations | Версии 1 и 2 применены |

HTTP curl-сценарий проверил health/ready, создание (201 + Location), получение и завершение (200), занятого водителя и повторное завершение (409), отсутствующую поездку (404), неверные JSON/UUID/цену/координаты (400). Проверен Problem Content-Type. Ошибка репозитория → безопасный 500 и rollback проверены автоматическими тестами.

Collector реально остановлен отдельно от PostgreSQL: после удаления его pod весь HTTP-сценарий прошёл; Collector восстановлен до 1/1. Итоговый бинарник по SIGTERM завершился с кодом 0, HTTP-порт закрылся, PostgreSQL-сессии процесса снизились с 3 до 0; ошибок завершения OTel нет.

## OpenTelemetry

- OTLP http/protobuf и grpc, ресурс trip-service / tripgo / local с версией сборки.
- Recover → Tracing → Logging → Metrics → router; имена HTTP-спанов и http.route содержат шаблон.
- pgx tracer на пуле даёт дочерние DB-спаны, включая транзакции; SQL-параметры и текст запросов не экспортируются.
- JSON slog в stdout с trace_id/span_id; бизнес-события и счётчики только после commit.
- Реально найдены HTTP/DB-гистограммы, active_requests, created_total и completed_total. После первого сценария created=1, completed=1; повторные ошибки их не увеличили. UUID в labels нет.
- По trace_id `57207082a6626987a13121100b1c3e89` из JSON-лога итогового бинарника найден HTTP-трейс создания с BEGIN, INSERT trips, INSERT trip_status_history и COMMIT.

## Скриншоты

Реальные интерфейсы локального окружения, без демонстрационных данных. Ссылки станут доступны после ручной отправки файлов ветки:

![Jaeger: создание поездки и PostgreSQL](https://github.com/epif4nov/template/blob/homework/02/docs/screenshots/jaeger-create-trip.png?raw=true)

![Grafana: созданные и завершённые поездки](https://github.com/epif4nov/template/blob/homework/02/docs/screenshots/grafana-business-metrics.png?raw=true)

![Grafana: HTTP, PostgreSQL и активные запросы](https://github.com/epif4nov/template/blob/homework/02/docs/screenshots/grafana-http-db-metrics.png?raw=true)

## Воспроизведение и ограничения

Команды запуска, зависимости и PromQL — в README; curl-сценарий — docs/homework02-demo.sh; полный протокол — docs/homework02-verification.md; памятка — docs/homework02-defense.md.

SDK по умолчанию отправляет метрики раз в 60 секунд; после запросов нужно дождаться экспорта и scrape. Счётчики относятся к процессу и обнуляются при перезапуске. После рестарта Collector локальному port-forward может потребоваться повторный tripgoctl connect. Скриншоты фиксируют конкретный запуск, а не текущее состояние.

Дополнительное мутационное тестирование со звёздочкой не выполнялось. Нагрузочная проверка shutdown с зависшим обработчиком отдельно не проводилась; проверены штатный SIGTERM, реальное закрытие пула и завершение экспортеров. Новые ручки и миграции не добавлены.