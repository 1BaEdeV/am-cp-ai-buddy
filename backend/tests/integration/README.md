# Интеграционные проверки

Запускай команды из папки `backend`. Обычный `go test ./...` выполняет проверки
на временной SQLite. Тесты с именем `TestPostgres...` без
`ACCESS_TEST_DATABASE_URL` явно пропускаются: SQLite не проверяет блокировки
и конкурентное поведение PostgreSQL.

## PostgreSQL

Для отдельной локальной тестовой БД, при запущенном Docker:

```sh
docker run --detach --rm --name ai-buddy-postgres-tests \
  --publish 127.0.0.1:15432:5432 \
  --env POSTGRES_USER=access_test \
  --env POSTGRES_PASSWORD=access_test \
  --env POSTGRES_DB=access_test \
  postgres:17-alpine
docker exec ai-buddy-postgres-tests pg_isready -U access_test -d access_test
```

Дождись ответа `accepting connections`. Данные этого контейнера временные;
приведённые учётные данные предназначены только для локальной тестовой БД.

```sh
export ACCESS_TEST_DATABASE_URL='postgres://access_test:access_test@127.0.0.1:15432/access_test?sslmode=disable'
go test -race -count=1 -run Postgres -v ./tests/integration/...
go test -race -count=1 ./...
go vet ./...
```

Пользователю БД нужно право `CREATE SCHEMA`. Каждый PostgreSQL-тест создаёт
свою случайную схему и удаляет её после завершения; таблицы приложения не
меняются. Если переменная задана, но подключение недоступно, тест завершается
ошибкой. `-count=1` гарантирует новый запуск вместо сохранённого результата.

После проверки останови только созданный тестовый контейнер:

```sh
docker stop ai-buddy-postgres-tests
unset ACCESS_TEST_DATABASE_URL
```

## Что проверяется на PostgreSQL

| Папка | Проверки |
| --- | --- |
| `accessrequest` | Повторные миграции, номера, внешние ключи, CHECK и уникальные ограничения, параллельные подтверждения и решения, захват задания одним обработчиком |
| `demoprovider` | Повтор одного ID операции, конфликт параметров и конкурентное изменение уровней без понижения прав |
| `demoseed` | Миграция всей схемы, загрузка демо и начальных прав, повтор с сохранением профилей и прогресса, откат всей загрузки при ошибке провайдера |
| `mltools` | Четыре HTTP-инструмента, подтверждение пользователем, решение руководителя, реальный фоновый обработчик, прогресс 2/4 → 3/4 → 4/4 и восстановление после потери ответа провайдера |

Остальные проверки с БД и HTTP выполняются на SQLite. Тесты также находятся
рядом с кодом в `internal` и `cmd/api`: sqlmock-проверки и проверки чата,
плана и авторизации с временной SQLite. Для SQLite нужны CGO и C-компилятор.
Наличие теста в `internal` не означает, что он работает без БД.
