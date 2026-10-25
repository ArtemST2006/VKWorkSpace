# VK WorkSpace — тестовое задание

В репозитории собраны решения трёх частей [тестового задания](TASK.md).

| Каталог | Описание |
|---|---|
| [msgfmt/](msgfmt/README.md) | Библиотека и CLI на C++17 для форматирования сообщений, работы с UTF-16-диапазонами и экранирования текста |
| [service/](service/README.md) | Консольная утилита `ghreport` на Go для получения отчётов по открытым GitHub Issues |
| [api_tests/](api_tests/README.md) | Интеграционные тесты `ghreport` на pytest с локальным HTTP-сервером |

В README каждого каталога есть подробности реализации, доступные параметры и инструкции по проверке.

## Быстрый запуск

### ghreport через Docker Compose

Самый простой способ запустить `ghreport` — через Docker Compose.

Из корня репозитория выполните:

```bash
docker compose build
docker compose run --rm -T ghreport
```

По умолчанию утилита сформирует JSON-отчёт по открытым Issues репозитория `cli/cli`, выведет его в терминал и завершит работу.

Чтобы указать другие репозитории и лимит:

```bash
GHREPORT_REPOS=cli/cli,octocat/Hello-World \
GHREPORT_LIMIT=10 \
docker compose run --rm -T ghreport
```

Результат можно сохранить в файл:

```bash
docker compose run --rm -T ghreport > report.json
```

Для постоянных настроек скопируйте `.env.example` в `.env` и укажите нужные значения.

`GITHUB_TOKEN` необязателен при работе с публичными репозиториями. При необходимости можно задать `GITHUB_API_BASE_URL` для использования совместимого API.

Файл `.env` не отслеживается Git. Секреты не встраиваются в Docker-образ.

## Сборка и тестирование

Для локальной сборки понадобятся:

- CMake 3.20+ и компилятор с поддержкой C++17
- Go 1.22+
- Python 3.10+ и pytest 7+
- Docker 24+ для контейнерного запуска

Все команды ниже выполняются из корня репозитория.

### 1. C++ — msgfmt

Сборка и запуск тестов:

```bash
cmake -S msgfmt -B build -DCMAKE_BUILD_TYPE=Release
cmake --build build
ctest --test-dir build --output-on-failure
```

Примеры использования:

```bash
printf 'Привет, **мир**!' | build/msgfmt
printf '2 * 3' | build/msgfmt escape
```

### 2. Go — ghreport

Проверка и сборка:

```bash
make -C service verify
make -C service build
```

Пример запуска:

```bash
service/bin/ghreport --repos cli/cli --limit 20 --out -
```

Для авторизованных запросов можно передать `GITHUB_TOKEN` через переменную окружения.

### 3. Интеграционные тесты

Тесты запускают собранный Go-бинарник как отдельный процесс и используют локальный HTTP-сервер вместо реального GitHub API.

```bash
GHREPORT_BIN="$PWD/service/bin/ghreport" pytest api_tests -q
```

### 4. Запуск через Docker без Compose

```bash
docker build -t ghreport service/
docker run --rm ghreport --repos cli/cli --limit 20 --out -
```

При необходимости переменные окружения передаются контейнеру через `-e`.

## Особенности реализации

**msgfmt (C++17)**

Использует только стандартную библиотеку C++. Для обработки Unicode применяется встроенная таблица свойств Unicode 15.0.0.

**ghreport (Go)**

Выполняет HTTP-запросы последовательно, поддерживает файловый ETag-кэш и записывает диагностические события в формате JSON Lines.

**Интеграционные тесты (Python)**

Проверяют работу собранной утилиты через отдельный процесс с использованием локального HTTP-стаба.

Подробности архитектуры, форматов данных и проверок находятся в README соответствующих каталогов.

## О разработке

При разработке использовался OpenAI Codex.

Фактически затраченное время: **2 дня** = 8 рабочих часов.