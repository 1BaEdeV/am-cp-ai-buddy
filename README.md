# AI-buddy

Онбординг стажёра: знакомство с командой, получение рабочих доступов, подготовка к первой задаче.

## Состав

| Каталог | Назначение |
| --- | --- |
| `frontend` | React, TypeScript, Vite. План, чат, заявки и очередь руководителя. |
| `backend` | Go 1.24+, PostgreSQL, GORM. API, бизнес-правила, фоновые обработчики чата и выдачи. |
| `ml` | Python, Gemini. Ответы по контексту задания и намерение подготовить один доступ. |
| `docs` | OpenAPI, архитектура, сценарии и инструкции интеграции. |

## Запуск

### Docker Compose

Из корня проекта, нужны Docker и Compose:

```sh
cp .env.example .env
# Укажи GEMINI_API_KEY в .env.
docker compose up -d --build --wait
```

Интерфейс и API: http://localhost:8080, Swagger: http://localhost:2048.
Compose запускает PostgreSQL, ML, Go API, nginx и Swagger. Данные БД сохраняются
в volume `db-data`. `APP_PORT` меняет внешний порт, `ML_HTTPS_PROXY` задаёт прокси
только для ML. Вне localhost для чата нужен HTTPS.

Остановка: `docker compose down`. Сброс БД: `docker compose down -v`.
Проверка логов: `docker compose logs backend ml`.

### Frontend

Нужен Node.js 24+. Из корня проекта:

```sh
cd frontend
npm ci
npm run dev
```

Адрес: http://127.0.0.1:5173. По умолчанию работает автономное демо:
подготовленные ответы, данные в `localStorage`, Go и PostgreSQL не нужны.

Для Go API скопируй `frontend/.env.example` в `frontend/.env.local`, установи
`VITE_DATA_SOURCE=api` и перезапусти Vite. Прокси по умолчанию ведёт на порт 8080.
[Настройки и ограничения интеграции](frontend/README.md).

### Backend

Нужны Go 1.24+, PostgreSQL и созданная база. Из корня проекта:

```sh
cd backend
cp .env.example .env
# Укажи DATABASE_URL своей базы.
set -a
source .env
set +a
go run ./cmd/api
```

При старте выполняются миграции и запускаются оба фоновых обработчика.
В примере окружения включена загрузка демо-данных. Отдельная команда загрузки:
`go run ./cmd/seed-demo`. Повторная загрузка сохраняет существующие записи.

| Переменная | Назначение / значение по умолчанию |
| --- | --- |
| `DATABASE_URL` | Обязательная строка подключения PostgreSQL. |
| `HTTP_ADDR` | Адрес API, `:8080`. |
| `ML_AGENT_URL` | ML endpoint, `http://localhost:8090/chat`. |
| `ISSUANCE_POLL_INTERVAL` | Опрос очереди выдачи, `500ms`. |
| `DEMO_SEED_ON_STARTUP` | Загрузка демо при старте; без переменной отключена. |
| `ML_TOOLS_TOKEN` | Секрет внутренних инструментов. При пустом значении маршруты отключены. |

Go читает окружение процесса; файл `.env` загружает команда `source`.

### ML

В отдельном терминале из корня проекта:

```sh
cd ml
python3 -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
cp .env.example .env
# Укажи GEMINI_API_KEY в .env.
python main.py
```

Адрес по умолчанию: http://127.0.0.1:8090. Модель задаёт `GEMINI_MODEL`,
адрес сервера — `ML_HOST` и `ML_PORT`. Без ключа `POST /chat` возвращает 503.
Текущий Python-сервис не вызывает внутренние Go-инструменты: намерение
`prepareAccessRequestResourceId` обрабатывает Go-worker.

### Swagger

Из корня проекта:

```sh
docker compose up -d openapi-swagger
```

Адрес: http://localhost:2048. Этот сервис можно запускать отдельно от приложения.

## Демо-пользователи

| ID | Состояние |
| --- | --- |
| `intern-alina` | Стажёр команды «Поиск», профиль заполнен, выданы Вики и YouTrack: 2/4. |
| `manager-dmitry` | Руководитель Алины, согласует недостающие GitLab и Grafana. |
| `intern-ivan` | Новый стажёр, требуется регистрация. |

Вход — по демо-ID или почте. Последующие запросы передают
`X-Demo-Employee-Id`; ID хранится в памяти вкладки. После обновления страницы
нужно войти снова. В API-режиме данные сохраняются в PostgreSQL.

## Доступы и ограничения

- Одна заявка — один ресурс. Черновик создаётся по просьбе агенту и действует 15 минут.
- Заявка `pending` появляется после отдельного подтверждения стажёра.
- Руководитель видит назначенные ему `pending`; одобрение запускает выдачу, отказ требует причины.
- Выдачу выполняет демо-провайдер. При ошибке руководитель повторяет прежнюю заявку.
- План, знакомство и чат подключены к Go. Формат ответа чата преобразует HTTP-клиент frontend: [контракт](frontend/README.md#контракт-чата).
- Первая задача — заглушка. После знакомства и всех доступов прогресс равен 2/3, статус задачи — `coming_soon`.
- Встреча с наставником — фиксированные демо-данные, календарной интеграции нет.

## Проверки

Команды выполняются в соответствующем каталоге.

```sh
# frontend
npm run test:coverage
npm run lint
npm run format:check
npm run build
npx playwright install chromium
npm run test:e2e -- --project=desktop --project=mobile --project=api
```

Vitest проверяет весь исполняемый `src`. Порог CI — 81% по строкам,
выражениям, функциям и ветвлениям. HTML-отчёт: `frontend/coverage/index.html`.

```sh
# backend
go test ./...
go vet ./...
go test -count=1 -coverpkg=./... -coverprofile=coverage.out ./...
go tool cover -func=coverage.out

# ml, с установленными зависимостями
python -m unittest discover -v
```

Проверки в Docker:

```sh
docker compose run --rm frontend-tests npm run test:coverage
docker compose run --rm backend-tests
docker compose run --rm ml python -m unittest discover -v
```

Go-тесты используют sqlmock и временную SQLite; для SQLite нужны CGO и C-компилятор.
PostgreSQL-тесты требуют `ACCESS_TEST_DATABASE_URL`, без неё пропускаются.
[Настройка отдельной тестовой БД](backend/tests/integration/README.md).

## Документация

- [HTTP API](docs/openapi.yaml)
- [Архитектура и обработка заявок](docs/architecture.md)
- [Пользовательские сценарии](docs/scenarios.md)
- [Демо-данные](docs/demo-data.md)
- [Внутренние ML-инструменты](docs/ml-tools.md)
- [Контракт ML-сервиса](ml/SDD.md)
- [План пилота](docs/pilot.md)
