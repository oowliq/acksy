# Acksy

[![CI](https://github.com/oowliq/acksy/actions/workflows/ci.yml/badge.svg)](https://github.com/oowliq/acksy/actions/workflows/ci.yml)
[![Security](https://github.com/oowliq/acksy/actions/workflows/security.yml/badge.svg)](https://github.com/oowliq/acksy/actions/workflows/security.yml)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue)](LICENSE)

> [!WARNING]
> **Проект в разработке.** Релизов пока нет, API и схема базы ещё могут меняться без миграционного пути. Образ `:main` собирается из каждого коммита в `main`. Пробовать можно, полагаться в продакшене пока не стоит.

Self-hosted сервис уведомлений. Задача та же, что у [gotify](https://gotify.net) и [ntfy](https://ntfy.sh), но модель другая: уведомления принадлежат пользователям, а не топикам, а состояние «прочитано» хранится на сервере и синхронизируется между всеми устройствами.

**Что уже работает:** пользователи, API-токены, топики с приглашениями, уведомления с серверным read-state, realtime (SSE и WebSocket), вход через OIDC, адаптеры ntfy, gotify и SMTP, Web Push, исходящие вебхуки и веб-интерфейс.

## Чем отличается

1. **User-first.** Уведомление всегда адресовано пользователю. Топики — необязательные категории для фильтрации, шаринга и дефолтных настроек.
2. **Серверный read-state.** Прочитал, заархивировал или отложил на одном устройстве — изменение сразу видно на остальных.
3. **API-токены с правами.** Scope-ы (`notifications:publish`, `notifications:read`, …), ограничение по топикам, срок жизни. Токен для CI может только публиковать в один топик.
4. **OIDC.** Вход через любой OIDC-провайдер (Authentik, Keycloak, Authelia, Google, …), авто-провижининг, маппинг claim → роль.
5. **Realtime.** WebSocket и SSE. Каждое изменение (новое уведомление, прочтение, удаление) доставляется всем активным клиентам. После переподключения клиент дочитывает пропущенные события по `since=<seq>`.
6. **Приоритеты, группировка, дедупликация, retention.** Пять уровней приоритета, `group_key` для свёртки похожих уведомлений, `dedup_key`, чтобы повтор обновлял существующее уведомление вместо создания нового, и TTL/лимиты хранения на уровне пользователя, топика и приоритета.
7. **Адаптеры.** Входящие сообщения проходят через подключаемые адаптеры. Встроенные: `acksy` (родной API), `ntfy` и `gotify` (совместимость с их publish-API: существующие скрипты и интеграции работают без изменений), а также SMTP для приложений, отправляющих письма.
8. **Нормальный markdown.** GFM: таблицы, списки задач, подсветка кода, ссылки. На сервере и клиенте рендер проходит санитайз.

## Архитектура

```
             ┌──────────── adapters (ingest) ────────────┐
 HTTP  ───▶  │  acksy  │  ntfy-compat  │  gotify-compat  │
 SMTP     ▶  │  smtp                                     │
             └────────────────────┬──────────────────────┘
                                  ▼  IncomingMessage
                       service.Publish
            auth/scope → получатели → dedup/group → store (tx + event seq)
                                  │
                                  ▼
                          broker (Redis / memory)
                ┌─────────────────┼──────────────────┐
                ▼                 ▼                  ▼
          WS / SSE hubs       Web Push           webhooks
```

Ключевые понятия:

| Сущность | Суть |
|---|---|
| **User** | Владелец уведомлений. Роли `admin` / `user`. Может быть связан с OIDC identity (`issuer + sub`). |
| **Topic** | Необязательная категория пользователя. Её можно расшарить другим пользователям через приглашение, которое они должны принять. Тогда сообщение размножается на всех участников, и у каждого свой read-state. |
| **Notification** | `id` (ULID), получатель, топик, `title`, `body` (markdown), `priority` 1–5, `tags`, `group_key`, `dedup_key`, `actions`, `click_url`, `expires_at`. |
| **Read state** | `read_at`, `archived_at`, `snoozed_until`. Хранится на сервере. |
| **API token** | Секрет показывается один раз при создании или перевыпуске, в БД хранится хеш. Сохраняются scope-ы, ограничение по топикам, `expires_at`. |
| **Event** | `notification.created`, `notification.updated`, `notification.deleted`, `notifications.bulk`, `topic.changed`, `topic.removed`, `invitations.changed`. У каждого пользователя монотонный `seq`. |

## Стек

- **Backend:** Go, `chi`, `sqlx` + `goose` (один портируемый SQL для обеих СУБД, см. [ADR 0001](docs/adr/0001-portable-sql.md)), `pgx`, `modernc.org/sqlite` (без CGO), `go-redis`, `coder/websocket`, `go-oidc`, `goldmark` + `bluemonday`, `slog`.
- **Хранилище:** SQLite по умолчанию (один бинарь, ноль зависимостей), PostgreSQL для больших инсталляций.
- **Pub/Sub:** Redis для нескольких инстансов, in-memory для одного узла и разработки.
- **Frontend:** React + TypeScript 7, Vite, TanStack Router и Query, Tailwind, примитивы Radix, PWA + Web Push. Пакетный менеджер — pnpm, линтер и форматтер — Biome, типы API генерирует [Kubb](https://kubb.dev). SPA встраивается в Go-бинарь через `embed`.

## Быстрый старт

### Docker

Образ `ghcr.io/oowliq/acksy` публичный, собирается для `linux/amd64` и `linux/arm64`. Пока нет релизов, используйте тег `:main`. После первого релиза появятся `:latest` и теги версий.

```bash
# SQLite в томе acksy-data, без Redis. Первый админ создаётся при старте, если пользователей ещё нет;
# после этого ACKSY_ADMIN_* можно убрать из запуска.
docker run -d --name acksy -p 8080:8080 -v acksy-data:/data \
  -e ACKSY_ADMIN_USERNAME=admin -e ACKSY_ADMIN_PASSWORD='придумайте-длинный-пароль' \
  ghcr.io/oowliq/acksy:main

# токен для скриптов (или в интерфейсе: Settings → API tokens)
ACKSY_TOKEN=$(docker exec acksy acksy token create --username admin --name ci --scopes notifications:publish)
```

Для доступа извне поставьте перед Acksy обратный прокси с HTTPS (Caddy, Traefik, nginx) и задайте `ACKSY_BASE_URL=https://…`. От него зависят флаг `Secure` у cookie, OIDC и Web Push. HSTS и ограничение частоты запросов по IP настраиваются на прокси.

### Из исходников

Нужны Go (версия из `go.mod`), Node.js 24 и pnpm.

```bash
# SPA + бинарь со встроенным UI
make deps build

# один бинарь, SQLite (data/acksy.db), без Redis; первый админ создаётся при старте
ACKSY_ADMIN_USERNAME=admin ACKSY_ADMIN_PASSWORD='придумайте-длинный-пароль' ./bin/acksy serve

# токен для скриптов; --topic ограничит его существующими топиками
ACKSY_TOKEN=$(./bin/acksy token create --username admin --name ci --scopes notifications:publish)
```

### Отправить уведомление

```bash
curl -H "Authorization: Bearer $ACKSY_TOKEN" -H "Content-Type: application/json" \
     -d '{"title":"Deploy","body":"**prod** обновлён","priority":4,"topic":"ci"}' \
     http://localhost:8080/api/v1/notifications
```

### Совместимость с ntfy и gotify

```bash
# ntfy-стиль (вместо Bearer можно curl -u :$ACKSY_TOKEN)
curl -H "Authorization: Bearer $ACKSY_TOKEN" -H "X-Priority: 4" -H "X-Title: Backup" \
     -d "Backup finished" http://localhost:8080/ntfy/backups

# gotify-стиль: токен с одним топиком (--topic backups) работает как «приложение gotify»
curl "http://localhost:8080/gotify/message?token=$ACKSY_TOKEN" \
     -F title=Backup -F message="Backup finished" -F priority=5
```

Поддерживаются обновление, удаление и clear ntfy по sequence ID, действия `view`, `X-Click` и `X-Attach`, а у gotify — `extras` с click URL. Без поддержки: отложенная доставка, вложения-файлы, подписки в формате ntfy/gotify. Полный маппинг полей описан в [docs/adapters.md](docs/adapters.md).

### Уведомления по SMTP

Приложение может отправить письмо прямо в топик Acksy. Например, письмо на `backups@notify.example.org` попадёт в топик `backups`. Тема письма станет заголовком, текст — телом уведомления.

1. Для TLS задайте `ACKSY_SMTP_LISTEN=:2465`, `ACKSY_SMTP_DOMAIN=notify.example.org`, пути к сертификату и ключу и перезапустите Acksy. Для Docker опубликуйте порт, например `465:2465`, и примонтируйте файлы сертификата и ключа с правом чтения для пользователя контейнера.
2. Включите **SMTP** в **Settings → Server → Integrations**. Создайте API-токен с правом `notifications:publish` для нужного топика.
3. Для TLS в приложении укажите имя сервера из сертификата, опубликованный порт, TLS с начала соединения (SMTPS), любой логин и API-токен в поле пароля. Укажите одного получателя в настроенном домене.

Для доверенной LAN можно включить SMTP без сертификатов. Пример настроек сервиса в Docker Compose (замените IP на LAN-адрес хоста):

```yaml
environment:
  ACKSY_SMTP_MODE: plain
  ACKSY_SMTP_LISTEN: ":2525"
  ACKSY_SMTP_DOMAIN: acksy.home.arpa
ports:
  - "192.168.1.20:2525:2525"
```

Включите интеграцию и создайте токен, как в шаге 2. В клиенте укажите LAN-адрес, порт `2525`, соединение без шифрования и API-токен в поле пароля. Токен и письмо передаются открытым текстом, поэтому ограничьте доступ к этому порту доверенной сетью.

[Форматы писем и пример проверки через curl](docs/adapters.md#smtp)

## Конфигурация

В окружении задаётся только то, без чего сервер не запустится. Всё остальное админ меняет в веб-интерфейсе, в Settings → Server и Settings → Sign-in. Эти настройки хранятся в базе и применяются без рестарта, другие инстансы подхватывают их в течение 15 секунд (см. [ADR 0008](docs/adr/0008-server-settings.md)).

| Переменная | По умолчанию | Описание |
|---|---|---|
| `ACKSY_LISTEN` | `:8080` | Адрес HTTP-сервера |
| `ACKSY_SMTP_LISTEN` | — | Адрес SMTP-сервера. Пустое значение отключает порт |
| `ACKSY_SMTP_MODE` | `tls` | `tls` для TLS с начала соединения, `plain` для SMTP без шифрования |
| `ACKSY_SMTP_DOMAIN` | — | Домен адресов получателей, обязателен при включённом порте |
| `ACKSY_SMTP_CERT_FILE` / `ACKSY_SMTP_KEY_FILE` | — | PEM-файлы цепочки сертификатов и закрытого ключа для режима `tls` |
| `ACKSY_DATABASE_URL` | `sqlite://data/acksy.db` (в Docker `sqlite:///data/acksy.db`) | `sqlite://…` или `postgres://…` |
| `ACKSY_REDIS_URL` | — | Если задан, используется Redis-брокер, иначе in-memory |
| `ACKSY_BASE_URL` | — | Публичный URL: OIDC redirect, ссылки в вебхуках, VAPID subject по умолчанию |
| `ACKSY_SECURE_COOKIES` | `true`, если `BASE_URL` — https | Флаг `Secure` у cookie сессии |
| `ACKSY_ADMIN_USERNAME` / `_PASSWORD` | — | Создаёт первого админа, если пользователей ещё нет |
| `ACKSY_VAPID_PUBLIC_KEY` / `_PRIVATE_KEY` | генерируются | Ключи Web Push; без них генерируются при первом старте и хранятся в БД |
| `ACKSY_LOG_LEVEL` / `_FORMAT` | `info` / `text` | `debug`, `info`, `warn`, `error` / `text`, `json` |
| `ACKSY_SHUTDOWN_TIMEOUT` | `15s` | Время на graceful shutdown |

В Settings → Server (только админ из login-сессии) настраиваются:

| Раздел | По умолчанию | Что задаёт |
|---|---|---|
| Integrations | ntfy и gotify включены, SMTP выключен | Доступность входящих HTTP-адаптеров и приёма писем на настроенном SMTP-порту |
| Publishing limits | `60` / `120` | Публикаций в минуту на API-токен и на пользователя (по всем его токенам), всплеск того же размера; `0` отключает. Превышение — 429 с `Retry-After` |
| Web Push | включён | Push и контакт оператора для push-сервисов (https URL или e-mail; по умолчанию https-`ACKSY_BASE_URL`, без него Safari отклоняет push) |
| Webhooks | включены, LAN запрещена, 7 дней | Исходящие вебхуки, доступ к адресам в локальной сети (защита от SSRF), срок хранения неудачных доставок |
| HTTP actions | включены, LAN запрещена | Кнопки-действия с HTTP-запросом. URL задаёт публикующий: LAN разрешайте, только если доверяете всем, кто может публиковать |
| Retention | 90 дней, непрочитанные вечно, 10000 | Сроки для прочитанных (можно задать по приоритетам) и непрочитанных, максимум уведомлений на пользователя; `0` выключает правило |
| Sessions & events | 30 дней / 7 дней | Время жизни login-сессии (продлевается при активности) и хранение событий для дочитывания после разрыва |

Прежние переменные этих настроек (`ACKSY_PUBLISH_LIMIT_*`, `ACKSY_RETENTION_*`, `ACKSY_WEBHOOK*`, `ACKSY_ACTIONS_*`, `ACKSY_ADAPTERS_*`, `ACKSY_PUSH_ENABLED`, `ACKSY_VAPID_SUBJECT`, `ACKSY_SESSION_TTL`, `ACKSY_EVENT_RETENTION`, `ACKSY_OIDC_*`) больше не читаются. Если они заданы, сервер пишет в лог предупреждение. Задайте значения заново в интерфейсе.

### OIDC

1. Задайте `ACKSY_BASE_URL`: из него строится redirect URI.
2. Админ открывает Settings → Sign-in, копирует оттуда Redirect URL и регистрирует в провайдере confidential-клиент (authorization code) с этим URI.
3. Там же вводит Issuer URL, Client ID и Client secret, нажимает Test connection и Save. Кнопка SSO на странице входа появляется сразу, без рестарта; другие инстансы подхватывают изменения в течение 15 секунд.
4. Чтобы админов назначали группы IdP, укажите Admin values (например `acksy-admins`) и убедитесь, что claim `groups` попадает в ID token или UserInfo (в Keycloak для этого нужен mapper, в Authentik достаточно scope `profile`).

Секрет клиента хранится в базе и через API не возвращается. Вход по паролю можно отключить только после того, как у вас самого есть привязанная к этому провайдеру идентичность. Если провайдер пропал, вход по паролю возвращает `acksy oidc allow-password`.

При первом входе создаётся пользователь без пароля. Существующие локальные пользователи с OIDC-аккаунтами автоматически **не** связываются, даже при совпадении username или email: это защита от захвата аккаунта. Связать их можно в Settings → Users или из CLI (`sub` есть в логе сервера при неудачном входе и в профиле IdP):

```bash
acksy user link-oidc --username admin --subject 3f2a…   # issuer по умолчанию из настроек Sign-in
acksy user identities --username admin
```

Если создание пользователей при первом входе выключено, войти через OIDC можно только в связанный аккаунт. Подробности описаны в [ADR 0002](docs/adr/0002-oidc.md) и [ADR 0006](docs/adr/0006-oidc-settings.md).

Стартовые параметры можно задать и YAML-файлом (`acksy serve --config acksy.yaml` или `ACKSY_CONFIG`), переменные окружения имеют приоритет. Служебные эндпоинты: `/healthz` (процесс жив) и `/readyz` (БД и Redis доступны).

## API

Спецификация OpenAPI 3.1 лежит в [`api/openapi.yaml`](api/openapi.yaml) и отдаётся сервером на `/api/v1/openapi.yaml`.

- `POST /auth/login`, `POST /auth/logout`, `GET/PATCH /me`, `POST /me/password`
- `GET /auth/methods` (какие способы входа включены), `GET /auth/oidc/login` и `GET /auth/oidc/callback` (OIDC, навигация браузера)
- `GET/PUT /settings/oidc`, `POST /settings/oidc/test`: настройки входа через OIDC (админ, только login-сессия)
- `POST /notifications`: публикация (себе или во все подписки топика; `dedup_key` обновляет существующее уведомление в том же топике, а без топика — среди уведомлений без топика)
- `GET /notifications`: лента с фильтрами `view` (inbox, archived, snoozed, all), `unread`, `topic`, `min_priority`, `group_key`, поиском `q` (все слова должны встретиться в заголовке, тексте или тегах, регистр не важен, см. [ADR 0005](docs/adr/0005-search.md)) и курсором `before`
- `PATCH /notifications/{id}`: read, archive, snooze; `POST /notifications/bulk`: то же для выборки (с теми же `q` и `min_priority`); `GET /notifications/counts`: непрочитанные и число отложенных
- `/topics`, `/topics/{id}/members`, `/topics/{id}/invites`, `/invitations`: топики и шаринг. Владелец приглашает по имени, приглашённый получает уведомление и становится участником, только когда примет приглашение (см. [ADR 0007](docs/adr/0007-topic-invitations.md))
- `/webhooks`: исходящие вебхуки (форматы `acksy`, `discord`, `slack`, `telegram`, подпись HMAC, ретраи), см. [docs/webhooks.md](docs/webhooks.md)
- `/push/config`, `/push/subscriptions`: Web Push-подписки браузеров (только из login-сессии), порог приоритета на устройство, тестовый push
- `/tokens`: API-токены со scope-ами `notifications:publish`, `notifications:read`, `notifications:write`, `topics:manage`, `admin`, ограничением по топикам и сроком жизни
- `/users`: администрирование. Admin-токен может только читать пользователей и идентичности. Создавать, менять и удалять пользователей и привязывать идентичности можно только из login-сессии: иначе утёкший токен мог бы задать себе пароль и выпустить новые токены
- `GET /events` (SSE) и `GET /ws` (WebSocket): поток изменений в реальном времени, см. ниже
- Вне `/api/v1`: `/ntfy/*` и `/gotify/message`, совместимые publish-API (см. [docs/adapters.md](docs/adapters.md))

Аутентификация: cookie сессии для браузера или `Authorization: Bearer ack_…` для скриптов. Межсайтовые небезопасные запросы из браузера отклоняются (защита от CSRF).

### Realtime

Каждое изменение уведомлений и топиков пользователя становится событием с номером `seq`, общим для всех устройств пользователя. Номера идут по порядку без пропусков. Клиент подключается к `/events` (SSE) или `/ws` (WebSocket):

1. С `?since=<seq>` (или заголовком `Last-Event-ID`) сервер сначала дочитывает из БД всё, что клиент пропустил, затем шлёт `{"type":"ready","seq":N}` и переходит к живым событиям.
2. Если события уже вычищены (старше срока хранения событий из Settings → Server), приходит `{"type":"reset","seq":N}`: клиент перечитывает состояние и слушает дальше.
3. Пропуски, например потерянное сообщение брокера или инстанс без общего Redis, сервер замечает сам и дочитывает из БД. Раз в 30 секунд он перепроверяет сессию или токен: отозванный доступ закрывает поток.

```bash
curl -N -H "Authorization: Bearer $ACKSY_TOKEN" "http://localhost:8080/api/v1/events?since=0"
```

В web клиент `web/src/lib/realtime.ts` превращает события в инвалидацию кэша TanStack Query, поэтому вкладки и устройства синхронизируются без перезагрузки.

### Кнопки-действия

У уведомления может быть до 4 кнопок:
- **`view`** открывает ссылку;
- **`http`** выполняет запрос на сервере, когда получатель её нажимает. Например, «Закрыть гараж» через Home Assistant.

```bash
curl -H "Authorization: Bearer $ACKSY_TOKEN" -H "Content-Type: application/json" http://localhost:8080/api/v1/notifications -d '{
  "title": "Garage door open", "priority": 4,
  "actions": [{"type": "http", "label": "Close garage", "method": "POST",
    "url": "http://ha.lan:8123/api/services/cover/close_cover",
    "headers": {"Authorization": "Bearer <HA token>"}, "body": "{\"entity_id\": \"cover.garage\"}", "clear": true}]
}'
```

- **Секреты.** Заголовки и тело видит только сервер. Получателям, участникам топика, вебхукам и push отдаются тип, надпись, метод и URL без query.
- **Результат.** Кнопка показывает итог. С `clear: true` успешное выполнение отмечает уведомление прочитанным.
- **Повторные нажатия.** Кнопку можно нажимать повторно, но не во время выполнения. Есть лимит частоты на пользователя.
- **Права.** Нажать может только получатель уведомления, со scope `notifications:write`.
- **Локальная сеть.** Адреса в LAN нужно разрешить в Settings → Server → HTTP actions.
- **Редиректы.** Сервер не переходит по редиректу, иначе заголовки и тело ушли бы на другой адрес. Ответ 3xx показывается как ошибка с его статусом.

Адаптер ntfy принимает `http`-действия в обоих форматах (`http, Label, URL, method=…, headers.X=…, body=…, clear=true`).

### Хранение

Раз в час уборщик удаляет старые уведомления:
- прочитанные и архивные — через 90 дней (Settings → Server → Retention), для отдельных приоритетов можно задать свой срок;
- непрочитанные — только если для них задан срок;
- уведомления с истёкшим `expires_at` — через сутки после истечения.

Сверх лимита на пользователя вытесняются сначала старые прочитанные.

Владелец топика может переопределить срок для топика (`retention_days`) и ограничить число его уведомлений у каждого участника (`max_notifications`). Например, чтобы в топике `ci` хранились только последние 500.

Удаление видно открытым клиентам сразу. Подробности описаны в [ADR 0004](docs/adr/0004-retention.md).

### Web Push

Push настраивается в Settings → Push (пункт «Push notifications» в меню пользователя ведёт туда же). Там push включается на текущем устройстве и задаётся порог приоритета (All, P3+, P4+, P5), а также видны и удаляются другие устройства. Push приходит, даже когда Acksy закрыт. Клик по уведомлению открывает его в приложении, повтор по `dedup_key` заменяет показанное уведомление, а не добавляет новое.

- Нужен HTTPS (или `localhost`) и контакт в Settings → Server → Web Push либо https-`ACKSY_BASE_URL`.
- iPhone и iPad: сначала добавить Acksy на экран «Домой» (Поделиться → На экран «Домой») и включать push из установленного приложения.
- В push уходят заголовок и начало текста (до 200 символов, без разметки). Содержимое зашифровано до браузера, push-сервис его не видит. Подробности описаны в [ADR 0003](docs/adr/0003-outbound-delivery.md).

### Веб-интерфейс

Встроен в бинарь и открывается на `/`. По умолчанию тема Catppuccin Mocha, Latte включается в меню пользователя. Инбокс сделан для быстрого разбора: `j`/`k` переходят по списку, `e` отмечает прочитанным и сразу открывает следующее, `a` архивирует, `s` откладывает, `x` выделяет, `/` открывает поиск, `?` показывает все клавиши. Поиск и фильтр приоритета работают на сервере, поэтому «Mark all read» отмечает ровно то, что показано. Уведомления с одинаковым `group_key` собираются в один лоток, «Show all» у лотка открывает всю группу. У Snoozed в сайдбаре виден счётчик отложенных. Изменения с других устройств появляются без перезагрузки.

Настройки открываются из меню пользователя (Settings) или по адресу `/settings`:

- **Account:** имя, email и смена пароля (для входа через SSO пароль не показывается).
- **Push:** web push на текущем устройстве и список других устройств.
- **API tokens:** создание токена со scope-ами, ограничением по топикам и сроком действия. Секрет показывается один раз вместе с готовыми примерами `curl` для Acksy, ntfy и gotify. Здесь же токен отзывается.
- **Topics:** создание топиков, название, описание и приоритет по умолчанию, хранение (`retention_days`, `max_notifications`), участники и приглашения, удаление. Участник расшаренного топика видит его настройки только для чтения и может из него выйти. Входящие приглашения показаны вверху страницы и принимаются или отклоняются там же, их число видно в меню настроек.
- **Webhooks:** исходящие вебхуки в Home Assistant, n8n, Discord, Slack и Telegram: фильтры по топикам и приоритету, тестовая отправка, ротация секрета подписи и очередь доставок. Подробности в [docs/webhooks.md](docs/webhooks.md).
- **Users** (только для админа): создание пользователей с паролем или только для SSO, роль, новый пароль, привязка и отвязка OIDC-идентичностей, отключение и удаление. Свою учётную запись админ меняет в Account.
- **Sign-in** (только для админа): провайдер OIDC, Redirect URL для регистрации, автосоздание и роли из групп, вход по паролю; Test connection проверяет настройки до сохранения.

Для локальной разработки: `make build`, затем `ACKSY_ADMIN_USERNAME=admin ACKSY_ADMIN_PASSWORD=... ./bin/acksy serve`. `make demo` заполнит сервер синтетическими демо-уведомлениями. `make dev` запускает Vite с hot reload на :5173 и проксирует API на :8080.

### CLI

То же самое, кроме выпуска токенов для других пользователей, админ может сделать в веб-интерфейсе: Settings → Users.

```bash
acksy user create --username alice [--admin]   # пароль читается из stdin
acksy user list
acksy user passwd --username alice
acksy user create --username bob --no-password  # только для SSO; потом link-oidc
acksy user link-oidc --username bob --subject <sub> [--issuer <url>]
acksy user unlink-oidc --username bob --subject <sub>
acksy user identities --username bob
acksy oidc allow-password                        # вернуть вход по паролю, если SSO недоступен
acksy token create --username alice --name backup --scopes notifications:publish [--topic backups] [--expires-in 720h]
acksy migrate up | status
```

## Дорожная карта

- [x] **M0 Скелет:** Go-модуль, конфиг, миграции SQLite/Postgres, Vite SPA + embed, Makefile, CI, Docker
- [x] **M1 Ядро:** пользователи, уведомления, read-state, топики, API-токены со scope-ами, REST v1
- [x] **M2 Realtime:** брокер (memory + Redis), WS + SSE с `since`, синк между устройствами
- [x] **M3 Умные уведомления:** приоритеты, группы, dedup, snooze, retention, действия-ссылки и HTTP-действия
- [x] **M4 OIDC:** вход, авто-провижининг, маппинг ролей
- [x] **M5 Адаптеры:** реестр, `ntfy`, `gotify` (входящие)
- [x] **M6 Доставка наружу:** Web Push + PWA, исходящие вебхуки (HMAC, ретраи, Discord и Slack)
- [x] **UI v1:** вход, инбокс со списком и панелью чтения, лотки групп, фильтры, массовые действия, горячие клавиши, realtime, темы Catppuccin Mocha и Latte
- [x] **M7 UI:** поиск и серверные фильтры, группы, настройки (аккаунт, push, токены, топики, вебхуки), администрирование пользователей и OIDC-идентичностей, настройка входа через OIDC из интерфейса
- [ ] **Позже:** мобильные клиенты (FCM / APNs / UnifiedPush), вложения (S3), метрики Prometheus

## Разработка

Правила, структура репозитория и команды описаны в [AGENTS.md](AGENTS.md). Архитектурные решения лежат в [docs/adr](docs/adr).

CI (`.github/workflows/ci.yml`) на каждый push и pull request:
- гоняет Go-тесты на SQLite, Postgres и Redis, golangci-lint, vitest, Biome и tsc;
- проверяет, что сгенерированные типы API совпадают с `api/openapi.yaml`;
- собирает Docker-образ и проверяет его Trivy.

Из `main` и тегов `v*` образ публикуется в GHCR с SBOM и provenance-аттестацией. Отдельный workflow `security.yml` (ещё и раз в неделю) запускает govulncheck, `pnpm audit`, CodeQL и gitleaks. Зависимости обновляет Dependabot.

## Безопасность

Об уязвимостях сообщайте приватно, как описано в [SECURITY.md](SECURITY.md).

## Лицензия

[Apache-2.0](LICENSE)
