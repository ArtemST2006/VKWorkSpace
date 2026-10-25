# Интеграционные тесты ghreport

Тесты запускают готовый бинарник как отдельный процесс и поднимают локальный HTTP-стаб GitHub API. Они не обращаются к публичному GitHub и не требуют токена. Каждый тест получает собственный сервер и временный каталог кэша.

Из корня репозитория:

```bash
make -C service build
python3 -m pip install -r api_tests/requirements.txt
GHREPORT_BIN="$PWD/service/bin/ghreport" pytest api_tests -q
```

`GHREPORT_BIN` необязателен, если бинарник уже находится в `service/bin/ghreport`. `GITHUB_API_BASE_URL` и `GITHUB_TOKEN` тесты подменяют значениями локального стаба; реальные значения окружения не используются. Python 3.10+ и pytest 7+.
