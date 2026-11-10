# AGENTS.md

Инструкции для AI-агентов и разработчиков, работающих в репозитории Acksy. Что это за продукт и зачем он нужен, описано в [README.md](README.md).

## Общение

- Общаться с пользователем только на русском. Код, идентификаторы, коммиты и комментарии в коде пишутся на английском.
- Если требование неоднозначно или затрагивает архитектуру (раздел «Инварианты»), сначала спросить, потом делать.

## Структура репозитория

```
cmd/acksy/              # точка входа: serve, migrate, user/token admin CLI
internal/
  config/               # стартовый конфиг из env + YAML (остальные настройки в БД, ADR 0008)
  app/                  # wiring зависимостей, жизненный цикл
  domain/               # сущности и чистые бизнес-правила (dedup, group, retention, scopes)
  service/              # use-cases: Publish, ReadState, Topics, Tokens, Users
  store/                # интерфейс Store + реализации
    migrations/{sqlite,postgres}/  # goose-миграции (различаются только типами)
    *.go                           # интерфейс Store + реализация на портируемом SQL (sqlx)
  broker/               # интерфейс Broker; memory/, redis/
  adapter/              # ingest-адаптеры: ntfy/, gotify/, smtp/, HTTP registry, adaptertest/ (golden-раннер)
  delivery/             # webpush/, webhook/ (service.Notifier, воркеры запускает app), action/ (HTTP-кнопки)
  auth/                 # сессии, OIDC, API-токены, проверка scope-ов
  markdown/             # goldmark + bluemonday: HTML и plain-text превью для push и вебхуков
  netguard/             # HTTP-клиент для пользовательских URL: запрет приватных адресов (SSRF)
  httpapi/              # роутер chi, хендлеры REST v1, middleware
api/                    # openapi.yaml (встраивается и отдаётся на /api/v1/openapi.yaml)
web/                    # React SPA (pnpm), собирается в web/dist и встраивается через embed
docs/                   # ADR (docs/adr/NNNN-*.md), adapters.md
```

## Слои и зависимости

Направление зависимостей: `httpapi / adapter / delivery → service → domain`. `service` работает со `store.Store` и `broker.Broker` только через интерфейсы.

- `domain` не импортирует ничего из `internal/` и не делает I/O.
- В `service` нет SQL, HTTP и конкретных реализаций брокера.
- HTTP-адаптеры разбирают запрос в `adapter.IncomingMessage`. SMTP-адаптер разбирает письмо в `domain.Message`. Проверка прав публикации, dedup и запись в базу происходят в `service.Publish`.
- Проверка scope-ов делается в сервисном слое, а не только в HTTP-middleware: тогда она одинаково срабатывает для всех адаптеров.
- Хендлеры тонкие: декодировать запрос, вызвать сервис, закодировать ответ.

## Инварианты (без обсуждения не менять)

1. Уведомление всегда принадлежит конкретному пользователю (`user_id NOT NULL`). Топик опционален.
2. При публикации в расшаренный топик создаётся по одному `Notification` на каждого получателя. Read-state у каждого свой.
3. Любое изменение уведомления или read-state пишется в одной транзакции с событием и следующим `seq` пользователя, и только после коммита публикуется в брокер.
4. Клиент должен уметь восстановиться после разрыва по `since=<seq>` без потерь. Поэтому события хранятся в БД (с собственным retention), брокер нужен только для fan-out.
5. Обе СУБД (SQLite и Postgres) поддерживаются на равных. Нельзя добавить фичу, которая работает только на одной из них.
6. Адаптеры ntfy и gotify только входящие. Отдавать потоки в формате ntfy или gotify не планируется.
7. Токены хранятся только в виде хеша. Открытое значение показывается один раз при создании или перевыпуске. Перевыпуск сразу заменяет секрет, сохраняя права, топики и срок действия.
   Управлять токенами (создавать, смотреть, перевыпускать, отзывать) можно только из login-сессии, так что утёкший токен не может выпустить новые. По той же причине только из сессии меняются пользователи, их пароли, роли и OIDC-идентичности, а также настройки входа: admin-токен их только читает.
8. Markdown санитайзится всегда: на сервере при рендере для вебхуков и push, на клиенте при отображении.
9. Права проверяются в `service` через `auth.Principal`: scope-ы, ограничение токена по топикам, владение. Чужие сущности выглядят как `not_found`, а не `forbidden`, чтобы нельзя было узнать об их существовании.

## Команды

Полный список: `make help`. Обновлять этот раздел вместе с Makefile.

```bash
make deps              # go mod download + pnpm install
make dev               # Go-сервер (:8080) + Vite dev server (:5173, проксирует /api)
make test              # go test -race (SQLite) + vitest
make test-integration  # поднимает Postgres и Redis из compose.dev.yaml и гоняет Go-тесты на них
make lint              # golangci-lint + Biome (линт и проверка формата) + tsc
make fmt               # gofumpt + goimports через golangci-lint, Biome для web
make build             # SPA в web/dist, затем бинарь bin/acksy со встроенным UI
make docker            # Docker-образ
```

- Тесты Postgres и Redis включаются переменными `ACKSY_TEST_POSTGRES_URL` и `ACKSY_TEST_REDIS_URL`. Без них они пропускаются (skip), а не падают.
- `web/dist/.gitkeep` нужен, чтобы `go build` работал до сборки фронта. Его восстанавливает плагин в `vite.config.ts`.
- `web/src/routeTree.gen.ts` генерируется плагином TanStack Router и коммитится. Руками не править.
- `web/src/gen/` генерирует Kubb (`make generate`, конфиг `web/kubb.config.ts`) и коммитится. Руками не править: CI проверяет, что файлы совпадают с `api/openapi.yaml`.

## Go

- Go последней стабильной версии. Форматирование: `gofumpt` + `goimports` (`make fmt`). Линтер: `golangci-lint` v2, конфиг в `.golangci.yml`. `//nolint` разрешён только с комментарием-причиной.
- `context.Context` — первый аргумент у всего, что делает I/O.
- Ошибки оборачиваются через `fmt.Errorf("...: %w", err)`. Доменные ошибки (`domain.ErrNotFound`, `ErrForbidden`, `ErrConflict`, …) мапятся в HTTP-коды в одном месте, в `httpapi`.
- Логирование только через `slog` с контекстом (request id, user id). Токены, пароли и тела уведомлений в логи не попадают.
- SQL живёт только в `internal/store`. Запросы пишутся один раз на портируемом подмножестве SQL для SQLite и Postgres (см. [ADR 0001](docs/adr/0001-portable-sql.md)). Каждый новый метод store покрывается контрактным тестом в `store_test.go`.
- Новая миграция пишется сразу для обоих диалектов с одинаковым номером.
- ID: ULID. Время: UTC, `time.Time` в домене, в БД хранится как timestamp/INTEGER по диалекту.
- Конкурентность: каждая горутина получает контекст отмены, все долгоживущие процессы (janitor, hubs, воркеры вебхуков) регистрируются в `app` для graceful shutdown.

## Frontend (`web/`)

- pnpm, Vite, React, TypeScript 7 (нативный `tsc`) в strict-режиме.
- Линтер и форматтер — Biome (`web/biome.json`), ESLint и Prettier не используются. `biome-ignore` допустим только с причиной, как `//nolint` в Go.
- Роутинг: TanStack Router (file-based). Серверное состояние: TanStack Query. Глобальный клиентский стор добавлять только при явной необходимости.
- Realtime-события из WS/SSE обновляют кэш Query (`setQueryData` / `invalidateQueries`). Отдельного параллельного состояния для них нет.
- UI: Tailwind, примитивы Radix (`radix-ui`) и свои компоненты в `web/src/components/ui.tsx` (кнопки, Segmented) и `web/src/components/form.tsx` (поля, диалог, подтверждение, секции настроек). Цвета берутся только из семантических токенов в `web/src/styles.css` (Catppuccin Mocha и Latte), сырые значения палитры в компонентах запрещены. Правила визуального мира описаны в `DESIGN.md`, бриф инбокса и контракт направления лежат в `.impeccable/surfaces/`.
- Markdown: `react-markdown` + `remark-gfm` + `rehype-sanitize`. Удалённые картинки не загружаются, чтобы отправитель не узнал IP и время прочтения.
- Типы API генерируются Kubb из `api/openapi.yaml` в `web/src/gen/api-types.ts` и вручную не дублируются. Компоненты импортируют их через `@/lib/api`.
- Обязательно: доступность (клавиатура, aria, контраст), адаптивность, светлая и тёмная темы.

## Тестирование

- Бизнес-логика (`domain`, `service`) покрывается table-driven юнит-тестами. Для правил dedup, group, retention и scope-ов полезен TDD.
- `store`: одни и те же контрактные тесты прогоняются на SQLite (in-memory) и на Postgres (`make test-integration` локально, services в CI).
- `broker`: контрактные тесты для memory и Redis.
- Адаптеры: golden-тесты «HTTP-запрос → `IncomingMessage`» на реальных примерах из документации ntfy и gotify.
- Frontend: Vitest + Testing Library для логики и компонентов.
- OIDC тестируется на фейковом провайдере `internal/auth/oidctest`: discovery, JWKS, token и userinfo отдаются через `http.Client` без открытия портов. `IdP.Authorize` играет роль браузера на странице провайдера.
- Стриминг (SSE, WebSocket, graceful shutdown) тестируется на настоящем `http.Server` поверх in-memory listener на `net.Pipe`, без открытия портов. Пример — `internal/httpapi/stream_test.go`. Клиент должен читать тело ответа: у `net.Pipe` нет буфера.
- Нельзя говорить, что тесты, линтер или сборка проходят, если они не были запущены.

## Безопасность

- Все входящие пути (родной API, адаптеры, WS, SSE) проходят одну аутентификацию в `auth` и одну проверку scope-ов в `service`.
- Токен в query (`?token=`) принимается только адаптером gotify, потому что этого требует совместимость. Значение токена маскируется в логах.
- Rate limiting на публикацию по токену и пользователю.
- Исходящие вебхуки подписываются HMAC, есть защита от SSRF (запрет приватных сетей по умолчанию).
- Любые запросы на URL, присланные пользователем (push endpoint, вебхуки, HTTP-действия), делаются только клиентом из `internal/netguard`. Ошибки таких запросов логируются через `netguard.Describe`, без URL.
- Секреты HTTP-действий (заголовки, тело, query в URL) не попадают ни в какой JSON-вывод: `domain.Action.MarshalJSON` их скрывает, в БД они пишутся только через `domain.ActionsForStorage`.
- В env только то, что нужно для старта (адрес, БД, Redis, `BASE_URL`, cookie, логи, бутстрап админа, VAPID-ключи). Остальные runtime-настройки живут в базе (`instance_settings`, ключ `server`, ADR 0008), меняются только админом из login-сессии и читаются через снимок `Service.Settings`. Внутри транзакции настройки не перечитываются: значения берутся до `InTx`.
- Настройки OIDC живут в базе (`instance_settings`, ключ `oidc`, ADR 0006). Секрет клиента только принимается API: в ответах есть лишь `client_secret_set`. Запросы к issuer идут через `internal/netguard` (частные адреса разрешены: URL задаёт админ). Менять настройки входа можно только админу из login-сессии.
- Cookie сессий: `HttpOnly`, `Secure`, `SameSite=Lax`. Для cookie-аутентификации включена CSRF-защита.
- Все ответы идут с `nosniff`, `X-Frame-Options: DENY` и CSP (`internal/httpapi/headers.go`). У SPA скрипты разрешены только со своего origin, inline-скрипты `index.html` — по хешу, который считается при старте. Новый inline-скрипт или внешний ресурс требует правки CSP.
- `dedup_key` действует в пределах топика (или среди уведомлений без топика): публикация в один топик не может перезаписать уведомление получателя в другом.

SMTP принимает подключения на отдельном порту в `internal/app`. По умолчанию используется TLS. Режим `ACKSY_SMTP_MODE=plain` разрешает SMTP без шифрования в доверенной сети. Аутентификация API-токеном обязательна в обоих режимах. Адрес, домен и PEM-файлы задаются через `ACKSY_SMTP_LISTEN`, `ACKSY_SMTP_DOMAIN`, `ACKSY_SMTP_CERT_FILE` и `ACKSY_SMTP_KEY_FILE`. Приём публикаций включается в Settings → Server. SMTP-сессии проверяют API-токены через `service.AuthenticateToken` и публикуют через `service.Publish` ([ADR 0009](docs/adr/0009-smtp-ingest.md)).

## Как добавить HTTP-адаптер

1. Создать `internal/adapter/<name>/` с реализацией интерфейса `adapter.Adapter` (маршруты и разбор запроса в `IncomingMessage`).
2. Зарегистрировать адаптер в `adapter.NewRegistry` в `internal/app` и добавить флаг включения в `domain.AdapterSettings` (Settings → Server), в OpenAPI и на страницу настроек. Адаптер монтируется под своим `Prefix()` вне `/api/v1` и принимает только API-токены.
3. Описать маппинг полей (приоритет, теги, actions, markdown) в `docs/adapters.md`.
4. Добавить golden-тесты на реальные запросы клиентов: `testdata/*.http` → `*.golden`, общий раннер `internal/adapter/adaptertest`. После намеренного изменения разбора выполнить `go test ./internal/adapter/ntfy ./internal/adapter/gotify -update` и просмотреть diff golden-файлов.
5. Убедиться, что адаптер не трогает store и broker напрямую.

## Definition of Done

- Код соответствует слоям и инвариантам выше.
- Есть тесты на новое поведение. `make test` и `make lint` проходят.
- Если менялась схема, миграции добавлены для обоих диалектов.
- Если менялось API, обновлён `api/openapi.yaml`. `TestOpenAPICoversRoutes` упадёт, если роут не задокументирован или в спеке остался лишний. Типы для web перегенерированы.
- README и AGENTS.md обновлены, если поменялись команды, конфиг или архитектура. Решения, существенно влияющие на архитектуру, оформлены как ADR в `docs/adr/`.
