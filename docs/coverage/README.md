# Отчёты покрытия

Дата замеров: **16 ноября 2026**. Код: [`10af7ad`](https://github.com/1BaEdeV/am-cp-ai-buddy/commit/10af7ad49b8e00d0ff72f01c87a22ff9a9c09e6f).
После изменения прикладного кода отчёты нужно обновить.

| Компонент | Метрика | Результат | Отчёт |
| --- | --- | --- | --- |
| Backend | Go statements | **82,4%** | [backend.txt](backend.txt), [исходный профиль ZIP](backend-profile.zip) |
| Frontend | Строки | **95,81%** — 504 из 526 | [frontend-summary.json](frontend-summary.json), [HTML ZIP](frontend-html.zip) |
| ML | Исполняемые строки | **84,75%** — 100 из 118 | [ml.txt](ml.txt), [исходный JSON](ml-coverage.json) |

Go измеряет statements, поэтому его показатель не называется покрытием строк.
Общий процент из этих трёх значений не вычисляется.

## Состав измерения

- Go: все пакеты через `-coverpkg=./...`, включая команды запуска и вспомогательный
  `tests/integration/testdb`. Файлы `*_test.go` инструментом не учитываются.
  Выполнены unit-тесты и интеграционные тесты SQLite; PostgreSQL-тесты пропущены
  без `ACCESS_TEST_DATABASE_URL`.
- Frontend: весь исполняемый `src`, включая API-клиент, UI и автономное демо.
  Исключены тесты, `src/test` и файл типов. Все 99 тестов прошли.
  В JSON пути сделаны относительными к `frontend`, числа Vitest сохранены.
- ML: весь прикладной модуль `main.py`. Все 11 тестов прошли с подменённым Gemini,
  платные вызовы модели не выполнялись. Замер получен ранее в тот же день;
  код и тесты ML совпадают с указанным коммитом.

Версии: Go **1.26.4**, Node.js **26.6.0**, npm **11.18.0**, Vitest/V8 **5.1.0**;
Python **3.13** в Docker, Coverage.py **7.17.3**. Зависимости приложения закреплены
в `go.mod`/`go.sum`, `package-lock.json` и `ml/requirements.txt`.

## Повторный замер

Команды выполняются из корня репозитория. Для Go нужны CGO и C-компилятор.
Для ML нужен Python 3.13; ключ Gemini для тестов не требуется.

### Backend

```sh
mkdir -p /tmp/ai-buddy-coverage
cd backend
go test -count=1 -coverpkg=./... -coverprofile=/tmp/ai-buddy-coverage/backend.coverage.out ./...
go tool cover -func=/tmp/ai-buddy-coverage/backend.coverage.out > ../docs/coverage/backend.txt
zip -j ../docs/coverage/backend-profile.zip /tmp/ai-buddy-coverage/backend.coverage.out
cd ..
```

При заданном `ACCESS_TEST_DATABASE_URL` выполнятся и PostgreSQL-тесты,
поэтому результат может отличаться от сохранённого замера.
Профиль из ZIP можно проверить той же командой `go tool cover -func=...`.

### Frontend

```sh
cd frontend
npm ci
npm run test:coverage
python3 - <<'PY'
import json
from pathlib import Path

root = Path.cwd().as_posix() + "/"
summary = json.loads(Path("coverage/coverage-summary.json").read_text())
portable = {key.removeprefix(root): value for key, value in summary.items()}
Path("../docs/coverage/frontend-summary.json").write_text(
    json.dumps(portable, ensure_ascii=False, indent=2) + "\n"
)
PY
(cd coverage && zip -qr ../../docs/coverage/frontend-html.zip .)
cd ..
```

Для просмотра HTML распакуй `frontend-html.zip` и открой `index.html`.

### ML

```sh
mkdir -p /tmp/ai-buddy-coverage
python3.13 -m venv /tmp/ai-buddy-coverage-venv
/tmp/ai-buddy-coverage-venv/bin/python -m pip install -r ml/requirements.txt coverage==7.17.3
cd ml
/tmp/ai-buddy-coverage-venv/bin/python -m coverage run --source=main --data-file=/tmp/ai-buddy-coverage/ml.coverage -m unittest discover -v
/tmp/ai-buddy-coverage-venv/bin/python -m coverage report --data-file=/tmp/ai-buddy-coverage/ml.coverage > ../docs/coverage/ml.txt
/tmp/ai-buddy-coverage-venv/bin/python -m coverage json --data-file=/tmp/ai-buddy-coverage/ml.coverage -o ../docs/coverage/ml-coverage.json
cd ..
```

Текстовый вывод Coverage.py округляет процент до **85%**; точное значение
**84,7457627%** сохранено в JSON.
