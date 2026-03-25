# Room Booking Service

Go-сервис бронирования переговорок по спецификации из [`api.yaml`](./api.yaml).

## Что реализовано

Обязательная часть:
- комнаты, расписания, слоты, бронирования, отмена брони;
- `_info` и `dummyLogin`;
- JWT middleware;
- unit/integration тесты.

Дополнительные задания:
- регистрация и логин по email/паролю: `POST /register`, `POST /login`;
- опциональное создание `conferenceLink` при бронировании;
- `Makefile` с `make up`, `make seed`, `make swagger`, `make loadtest`;
- Swagger-кодогенерация из аннотаций в коде в папку [`docs`](./docs);
- конфигурация линтера в [`.golangci.yaml`](./.golangci.yaml);
- краткие результаты нагрузочного теста в [`loadtest/results.md`](./loadtest/results.md).

## Быстрый старт

Запуск приложения:

```bash
make up
```

или

```bash
docker-compose up --build
```


Сервис будет доступен на `http://localhost:8080`.

Проверка healthcheck:

```bash
curl -i http://localhost:8080/_info
```

Наполнение БД тестовыми данными:

```bash
make seed
```

## Тестовые пользователи

После `make seed` доступны:

- `admin@example.com` / `Admin123!`
- `user@example.com` / `User123!`

## Конференц-ссылки

В `POST /bookings/create` поддерживается флаг:

```json
{
  "slotId": "uuid",
  "createConferenceLink": true
}
```

Используется мок внешнего `Conference Service`.

Поддерживаемые сценарии отказа:
- Если `createConferenceLink=false`, бронирование работает как раньше и внешний сервис не вызывается.
- Если `createConferenceLink=true` и mock недоступен, бронь не создаётся, клиент получает `503`.
- Если mock успел вернуть ссылку, но запись брони в БД не сохранилась, сервис делает best-effort cleanup через `DeleteLink`.
- Если cleanup прошёл успешно, наружу возвращается исходная ошибка сохранения брони, например `409`.
- Если cleanup тоже упал, наружу возвращается `500`, потому что система уже не может гарантировать консистентное состояние интеграции.

Для ручного моделирования сбоев есть env-переменные:
- `CONFERENCE_MOCK_FAIL_CREATE=true`
- `CONFERENCE_MOCK_FAIL_DELETE=true`

## Swagger

Документация генерируется из аннотаций в handlers:

```bash
make swagger
```

Результат:
- [`docs/swagger.json`](./docs/swagger.json)
- [`docs/swagger.yaml`](./docs/swagger.yaml)
- Swagger UI http://localhost:8080/swagger/index.html

## Нагрузочный тест

Для hot endpoint добавлен воспроизводимый HTTP load test на `k6`. Он поднимает live-сервис в Docker Compose, создает профиль данных для 50 комнат и 1000 слотов в день, а затем льет нагрузку на `GET /rooms/{roomId}/slots/list?date=YYYY-MM-DD`.

```bash
make loadtest
```

Сценарий измеряет:
- RPS для hot endpoint;
- success rate;
- latency avg / p50 / p95 / p99 / max;
- итоговый markdown-отчет в [`loadtest/results.md`](./loadtest/results.md).

Краткий результат актуального прогона:
- профиль: `50 rooms / 1000 slots per day / 100 RPS`;
- success rate: `100.00%`;
- latency p99: `1.94 ms`;
- подробности: [`loadtest/results.md`](./loadtest/results.md).

Подробности: [`loadtest/results.md`](./loadtest/results.md).

## Тесты и качество

Запуск тестов:

```bash
make test
```

Команда запускает все тесты, сохраняет `coverage.out` и печатает coverage summary с итоговой строкой `total`.

Покрытие локально:

```bash
GOCACHE=$(pwd)/.gocache go test ./... -coverprofile=coverage.out -covermode=atomic
GOCACHE=$(pwd)/.gocache go tool cover -func=coverage.out
```

Краткий результат локальной проверки:
- `go test ./...` проходит успешно;
- суммарное покрытие: `51.6%`;

## Примечания по архитектуре

- Для слотов используется предгенерация на окно ближайших `N` дней.
- Самый горячий endpoint читает уже материализованные слоты из БД.
- Для быстрого чтения используются индексы `idx_slots_room_date` и `idx_bookings_slot_active`.
- `conference_link` сохраняется прямо в записи брони.
