# Московский Спорт официальный бот

Телеграм-бот `@mos_sport_bot`, который уведомляет подписанные чаты, когда
`mos-sport-backend` обнаруживает доступную запись на секцию `sport.mos.ru`.

Бэкенд сам вызывает HTTP-ручку бота при переходе состояния в `available` —
бот не опрашивает бэкенд, только принимает пуш.

Бот и бэкенд разворачиваются на разных серверах, поэтому общаются по
публичному адресу и порту, а не через docker-сеть: бэкенд стучится в
`http://<адрес бота>:8088/webhook/notify`, а бот (опционально, для `/status`)
ходит в `http://<адрес бэкенда>:<порт>/status`. Порт бота на сервере
деплоя должен быть открыт во внешнем файрволе отдельно — самим Docker Compose
это не делается.

## Команды бота

- `/start` — подписать текущий чат на уведомления.
- `/stop` — отписать текущий чат.
- `/status` — показать текущее состояние мониторинга (запрашивает `GET /status`
  у бэкенда).

## Запуск

```sh
cp .env.example .env
go run ./cmd/bot
```

Или через Docker Compose:

```sh
cp .env.example .env
docker compose up --build
```

## Переменные окружения

| Переменная | Назначение | По умолчанию |
| --- | --- | --- |
| `TELEGRAM_BOT_TOKEN` | Токен бота из BotFather. Обязательна. | — |
| `WEBHOOK_SECRET` | Секрет, которым бэкенд подписывает вызовы `/webhook/notify`. Обязательна. | — |
| `LISTEN_ADDR` | Адрес HTTP-сервера бота. | `:8088` |
| `SUBSCRIBERS_FILE` | Путь к JSON-файлу со списком подписанных чатов. | `/data/subscribers.json` |
| `BACKEND_STATUS_URL` | Публичный URL `GET /status` бэкенда для команды `/status`. Если пусто — команда отвечает, что не настроена. | `` (пусто) |
| `REQUEST_TIMEOUT` | Таймаут запроса к бэкенду. | `10s` |

## HTTP API

### `GET /healthz`

Проверка, что HTTP-сервер запущен. Ответ `200 ok\n`.

### `POST /webhook/notify`

Вызывается бэкендом при появлении записи. Требует заголовок
`X-Webhook-Secret` со значением `WEBHOOK_SECRET`.

Тело запроса:

```json
{
  "available": true,
  "checked_at": "2026-08-25T07:26:03.221219Z",
  "url": "https://sport.mos.ru/sections/11716",
  "active_buttons": 1,
  "buttons": ["Записаться"]
}
```

Ответы:

- `200 {"sent":N,"failed":M}` — уведомление разослано подписчикам.
- `204` — `available:false`, рассылки не было (принято, но проигнорировано).
- `400` — тело запроса не разобрать как JSON.
- `401` — неверный или отсутствующий `X-Webhook-Secret`.

## Деплой

CI/CD — GitHub Actions (`.github/workflows/build-and-deploy-bot.yml`), по
образцу `mos-sport-backend`: тесты → сборка и публикация Docker-образа →
деплой по SSH с healthcheck и автооткатом.

Для работы пайплайна в репозитории на GitHub нужно завести секреты:

- `DOCKER_USERNAME`, `DOCKER_TOKEN` — доступ к Docker Hub.
- `DEPLOY_HOST_IP`, `DEPLOY_HOST_USERNAME`, `DEPLOY_HOST_KEY` — SSH-доступ к
  серверу (`qlibin_server`).
- `DEPLOY_HOST_PROJECT_PATH` — `/home/evgeny/projects/mos-sport-bot`.
- `TELEGRAM_BOT_TOKEN` — токен бота.
- `WEBHOOK_SECRET` — общий секрет с бэкендом (сгенерировать, например,
  `openssl rand -hex 32`, и прописать то же значение в `BOT_WEBHOOK_SECRET`
  бэкенда на сервере).
- `BACKEND_STATUS_URL` — опционально, публичный адрес `GET /status` бэкенда,
  если он открыт наружу (`/status` в боте работает только при заданном
  значении).

Порт `8088` должен быть открыт на сервере деплоя (внешний файрвол/security
group) — иначе бэкенд не сможет достучаться до `/webhook/notify`.
