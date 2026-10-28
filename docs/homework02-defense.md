# ЛР2: короткая памятка к защите

## Почему интерфейсы в usecase

[create/contract.go](../internal/usecase/trip/create/contract.go) объявляет только нужные созданию методы: Save, AddStatusHistory и Do. [get](../internal/usecase/trip/get/contract.go) требует только Fetch. Потребитель определяет контракт; PostgreSQL-репозиторий реализует его неявно. В HTTP-пакетах тоже есть свои узкие интерфейсы Execute. Второй транспорт сможет вызвать те же usecase без копирования бизнес-правил.

## Создание и завершение

[create.Execute](../internal/usecase/trip/create/usecase.go) вызывает trip.New, затем внутри Do сохраняет поездку и начальную историю. trip.New проверяет UUID, цену и координаты, создаёт active-поездку. ValidationError содержит поле и причину; Unwrap сохраняет errors.Is(ErrInvalidTrip).

[finish.Execute](../internal/usecase/trip/finish/usecase.go) внутри Do читает поездку, вызывает доменный Finish, выполняет условный UPDATE и сохраняет историю перехода. [репозиторий](../internal/repository/trip/finish_trip.go) обновляет только active: повторное завершение даёт ErrTripCompleted, отсутствие строки — ErrTripNotFound. Домен и SQL вместе защищают от конкурентных завершений. Частичный уникальный индекс защищает от двух active-поездок одного водителя.

## Транзакция и откат

[Manager.Do](../internal/infra/postgres/transaction/manager.go) начинает READ COMMITTED и помещает pgx.Tx в context. [Executor](../internal/infra/postgres/transaction/executor.go) выбирает эту транзакцию вместо пула. После успеха callback — COMMIT; при ошибке, панике или ошибке commit — отложенный ROLLBACK. Для отката снимается отмена родительского контекста и задаётся свой таймаут. Вложенный Do использует текущую транзакцию.

Если запись истории не удалась, первая запись не остаётся в trips. Это проверяет [интеграционный тест](../internal/repository/trip/transaction_integration_test.go) на настоящем PostgreSQL.

## Почему gomock-тесты — unit

В [create/usecase_test.go](../internal/usecase/trip/create/usecase_test.go) репозиторий и менеджер транзакций заменены сгенерированными моками. БД, сеть и SQL не запускаются. Проверяются результат, ошибки и ожидаемые вызовы, например отсутствие успешного результата при сбое истории. Такой тест проверяет оркестрацию usecase; реальный rollback проверяется отдельно integration-тестом.

## Как HTTP становится трейсом

[Middleware.Wrap](../internal/http/middleware/middleware.go) собирает Recover → Tracing → Logging → Metrics → chi. otelhttp извлекает W3C TraceContext и создаёт server span. Запросный context доходит до usecase, Do и pgx. В [container](../internal/container/container.go) tracer подключён к конфигурации пула, поэтому BEGIN, SQL и COMMIT становятся дочерними спанами автоматически.

В реальном создании видны HTTP и 6 DB-спанов: BEGIN, два INSERT, COMMIT и два вложенных prepare на первом использовании SQL. У основных INSERT есть db.collection.name для trips и trip_status_history.

## Почему нельзя UUID в http.route

Метрика группируется по методу, шаблону и статусу. Если писать /api/v1/trips/конкретный-uuid, каждая поездка создаст новые серии и раздует память Prometheus. Поэтому GET использует /api/v1/trips/{tripId}, finish — /api/v1/trips/{tripId}/finish. Имена HTTP-спанов построены так же. UUID допустим как отдельное поле бизнес-лога, но не как label метрики.

## JSON-лог → trace_id → Jaeger

[LogHandler.Handle](../internal/infra/telemetry/logger.go) получает SpanContext из context записи и добавляет trace_id/span_id. Logger передан зависимостью; бизнес-код не вызывает slog.Default. trace_id из request handled вставляется в Jaeger Lookup by Trace ID. Проверенный пример итогового бинарника: `57207082a6626987a13121100b1c3e89`; server span `84c08f6f6a696aa3`. Вне спана поля пустые.

## Пять метрик

| Инструмент | Что объясняю на нашем запросе |
|---|---|
| http.server.request.duration | Гистограмма секунд по методу, шаблону, статусу. Видны 201 создания, 409 занятого водителя и повторного finish, 400/404; bucket нужен для p95. |
| http.server.active_requests | UpDownCounter: +1 на входе, -1 после ответа, включая панику. После завершения curl равен 0. |
| db.client.operation.duration | Гистограмма SQL по операции, таблице и error.type. INSERT занятого водителя даёт PostgreSQL 23505; видны и ROLLBACK. |
| tripgo.trips.created | +1 только после успешного возврата create, включая commit. Ошибка водителя/истории не увеличивает счётчик. |
| tripgo.trips.completed | +1 только после commit завершения. Повторное завершение не увеличивает счётчик. |

Prometheus заменяет точки подчёркиваниями, добавляет _seconds и _total; у histogram есть _bucket, _count, _sum. Эти преобразования не означают, что в приложении другое имя инструмента.

## Что показать за две минуты

1. `make run`, в другом терминале `bash docs/homework02-demo.sh`.
2. В stdout взять trace_id создания и открыть его в Jaeger.
3. Показать вложенные SQL и шаблон route у GET/finish.
4. В Grafana Explore / Prometheus открыть пять запросов из [README](../README.md).
5. Показать [настоящие скриншоты](screenshots/) и [протокол проверки](homework02-verification.md).

Проверка SIGTERM итогового бинарника: exit 0, HTTP закрылся, PostgreSQL-сессии процесса 3 → 0, ошибки shutdown OTel отсутствовали. При остановленном Collector API продолжал работать.