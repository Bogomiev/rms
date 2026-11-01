# Задания сотрудников

Миграция `008_tasks.sql` применяется штатным механизмом при запуске RMS.
Она добавляет `task_types`, `task_scheduling`, `tasks`, `task_status_history`.
Внешние ключи используют `SMALLINT` / `BIGINT`, без отдельных последовательностей.
Магазины хранятся только в массиве UUID `task_scheduling.stores`, с GIN-индексом
для поиска по элементам. Ограничивающий триггер проверяет существование каждого
магазина непосредственно по массиву. Удаление магазина и изменение его UUID
запрещены, пока он используется в расписании. Отдельной таблицы связей нет.

## HTTP API

Все методы доступны администратору через существующие cookie-сессии и CSRF.
Для каждого из четырёх ресурсов:

- `GET /<resource>/?limit=100&offset=0` — список (максимум 100 за запрос).
- `GET /<resource>/{id}` — запись.
- `POST /<resource>/` — создание.
- `PUT /<resource>/{id}` — полная замена редактируемых полей.
- `DELETE /<resource>/{id}` — удаление.

Ответы используют обычную обёртку RMS: `resultCode`, `messages`, `data`.
`POST` возвращает 201, ошибки ввода — 400, отсутствующая запись — 404,
конфликт уникальности или ссылок — 409. Поля `id`, `created_at`, `update_at`
задаются сервером и не передаются в теле записи.
История дополнительно имеет технический `id` для адресации операций CRUD.

Индексируемые фильтры:

| Ресурс | Параметры |
| --- | --- |
| Все | `id` |
| `task_types` | `name` (точное совпадение) |
| `task_scheduling` | `task_type_id`, `store_id` (вхождение в массив) |
| `tasks` | `type_id`, `store_id` |
| `task_status_history` | `task_id`, `user_id`, `period_from`, `period_to` |

История сортируется по `period DESC, id DESC`. Для статуса задания на дату:
`GET /task_status_history/?task_id=123&period_to=2026-11-02T12:00:00&limit=1`.
Типы сортируются по `sort_order, id`, остальные ресурсы — по `id`.

`service`: `cash_collection`, `price_print`, `without_service`.
`status`: `new`, `in_progress`, `completed`, `cancelled`.
`icon`: base64 PNG/JPEG/WebP/GIF до 1 МиБ или `null`.
`color`, `color_dark`: `#RRGGBB`.

`frequency`: 1–10 недель (повторная 6 в исходном перечне трактуется как 8).
`period_from` и `period_to`: даты `YYYY-MM-DD`. Сервер и интерфейс расширяют
их до понедельника слева и воскресенья справа. `days` содержит ровно
`frequency * 7` значений 0/1, начиная с понедельника первой недели.
`time`: `HH:MM` или `HH:MM:SS`. `stores`: массив UUID, повторы удаляются.
Пустой массив означает, что магазины не выбраны.
Временные метки заданий и истории хранятся как PostgreSQL `timestamp` без
часового пояса; предпочтительный входной формат — `YYYY-MM-DDTHH:MM:SS`.

CRUD заданий и истории независим: автоматическая генерация заданий из расписаний
и запись истории при смене статуса в эту реализацию не входят.

## Проверка

```sh
TEST_POSTGRES_DSN='postgres://user@localhost/test?sslmode=disable' \
  go test ./internal/services/task ./internal/storage/postgres \
  -run 'TestTask|TestSchedule|TestRejectInvalid'
go test ./internal/app/http -run TestTaskResourcesRequireAuthentication
```

Тесты PostgreSQL создают и удаляют изолированные схемы в указанной тестовой базе.
