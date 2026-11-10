# Входящие адаптеры

## HTTP-адаптеры ntfy и gotify

Адаптеры принимают publish-API [ntfy](https://docs.ntfy.sh/publish/) и [gotify](https://gotify.net/docs/pushmsg). Существующим скриптам достаточно сменить базовый URL и токен. Адаптеры только входящие: подписаться на поток в формате ntfy или gotify нельзя, читать уведомления нужно через API Acksy (инвариант 6).

| Адаптер | Путь | По умолчанию |
|---|---|---|
| ntfy | `/ntfy/…` | включён |
| gotify | `/gotify/message` | включён |

Адаптеры включаются и выключаются в Settings → Server → Integrations без рестарта. Выключенный адаптер отвечает 404.

Общее для обоих:

- **Только API-токены** со scope `notifications:publish` (`acksy token create …` или `/api/v1/tokens`). Пароль пользователя и cookie сессии адаптеры не принимают, поэтому защита от CSRF им не нужна.
- Права проверяются в `service`, как у родного API: ограничение токена по топикам, членство в топике, автосоздание топика для неограниченных токенов.
- Тело всегда хранится как markdown и санитайзится при показе (инвариант 8). У ntfy `Markdown: yes` и у gotify `contentType` ничего не меняют.
- `source` у уведомления равен `ntfy` или `gotify`.
- Ошибки возвращаются в формате исходного сервиса, коды HTTP те же, что у родного API.
- Лимит публикаций общий с родным API: один бюджет на токен и на пользователя (Settings → Server → Publishing limits). Удаление и clear по sequence ID тоже его расходуют. При превышении возвращается 429 с `Retry-After` в формате ошибки протокола.

## ntfy

### Маршруты

| Запрос | Действие |
|---|---|
| `POST`/`PUT /ntfy/<topic>` | Публикация. Тело — текст сообщения, параметры в заголовках или query |
| `GET /ntfy/<topic>/publish` (`/send`, `/trigger`) | Публикация через query (`?message=…&title=…`) |
| `POST`/`PUT /ntfy/` с JSON | Публикация JSON-ом, топик в поле `topic` |
| `POST`/`PUT /ntfy/<topic>/<sid>` | Публикация или обновление по sequence ID |
| `DELETE /ntfy/<topic>/<sid>`, `GET /ntfy/<topic>/<sid>/delete` | Удаление у всех получателей |
| `PUT`/`POST`/`GET /ntfy/<topic>/<sid>/clear` | Отметить прочитанным у всех получателей |
| `GET /ntfy/<topic>/json`, `/sse`, `/ws`, … | 404: подписки не поддерживаются |

`<topic>` — имя топика Acksy: свой топик или единственный расшаренный с таким именем. Имя приводится к нижнему регистру.

### Аутентификация

`Authorization: Bearer ack_…` или Basic с токеном в качестве пароля и любым логином (`curl -u :ack_…`). Логин и пароль пользователя, а также `?auth=` в query не принимаются.

### Поля

| ntfy | Acksy | Примечание |
|---|---|---|
| тело, `X-Message` / `Message` / `m` | `body` | Пустое сообщение без заголовка превращается в `triggered`, как в ntfy |
| `X-Title` / `Title` / `t` | `title` | |
| `X-Priority` / `Priority` / `prio` / `p` | `priority` | `1`–`5` или `min`, `low`, `default`, `high`, `max`/`urgent`. Нет значения — дефолт топика |
| `X-Tags` / `Tags` / `tag` / `ta` | `tags` | Через запятую. Эмодзи-шорткоды остаются тегами |
| `X-Click` / `Click` | `click_url` | |
| `X-Actions` / `Actions` | `actions` | JSON и короткий формат. `view` и `http` (с `method`, `headers.*`, `body`, `clear`) сохраняются: `http` выполняет сервер, когда получатель нажимает кнопку, заголовки и тело получателям не показываются. `broadcast` и `copy` молча отбрасываются |
| `X-Attach` / `Attach` / `a` | `actions` | Ссылка-кнопка «Attachment», если есть место (до 4 действий) |
| `X-Sequence-ID` / `Sequence-ID` / `SID`, путь `/<topic>/<sid>` | `dedup_key` | Повторная публикация с тем же ID заменяет уведомление |
| `X-Markdown`, `X-Icon`, `X-Email`, `X-Call`, `X-Cache`, `X-Firebase`, `X-UnifiedPush` | — | Принимаются и игнорируются |
| `X-Delay` / `X-At` / `X-In` | — | **400**: отложенной доставки нет, а прийти сразу значило бы исказить смысл |
| загрузка файла (тело-файл, `Filename` без `Attach`, `message` вместе с телом, не-UTF-8 или тело больше 64 KiB) | — | **400**: вложения не поддерживаются |

Каждое ntfy-сообщение получает `dedup_key`: либо переданный sequence ID, либо сгенерированный `id` из ответа. Поэтому, как в ntfy, его можно обновить, удалить или погасить по возвращённому `id`.

Удаление и clear по sequence ID действуют на копии **всех** получателей сообщения в топике. Их может выполнить любой, кто вправе публиковать в этот топик. Это тот же круг, что уже может перезаписать сообщение публикацией с тем же ключом.

### Ответ

```json
{"id":"aB3dE6gH9jK1","time":1794219256,"event":"message","topic":"alerts","message":"Disk almost full","title":"Backup","priority":4,"tags":["warning"]}
```

Для удаления и clear `event` равен `message_delete` или `message_clear`, плюс `sequence_id`. Ошибка выглядит так: `{"code":40000,"http":400,"error":"…"}`.

## gotify

### Маршрут

`POST /gotify/message`, тело в JSON (`application/json`) или форме (`application/x-www-form-urlencoded`, `multipart/form-data`, как у `curl -F`).

### Аутентификация

Заголовок `X-Gotify-Key: ack_…`, query `?token=ack_…` или `Authorization: Bearer ack_…`. Токен в query разрешён только этому адаптеру, ради совместимости, и в логи запросов не попадает: логируется путь без query.

### Куда попадает сообщение

Аналог «приложения gotify» — токен, ограниченный ровно одним топиком (`acksy token create … --topic backups`). Сообщение уходит в этот топик всем его участникам. Неограниченный токен публикует во входящие владельца без топика. Токен с несколькими топиками получит 403: непонятно, какой выбрать.

### Поля

| gotify | Acksy | Примечание |
|---|---|---|
| `message` | `body` | Обязательно |
| `title` | `title` | Пустой заголовок остаётся пустым: в gotify это имя приложения |
| `priority` 0–10 | `priority` 1–5 | `0`→1, `1–3`→2, `4–7`→3, `8–9`→4, `10+`→5. Не задан — дефолт топика |
| `extras["client::notification"].click.url` | `click_url` | |
| `extras["client::display"].contentType` | — | Тело всегда markdown |
| `extras["client::notification"].bigImageUrl`, `android::action`, остальные extras | — | Игнорируются (удалённые картинки не загружаются) |
| `appid` | — | Игнорируется: «приложение» определяется токеном |

### Ответ

Формат сообщения gotify. В `message`, `title`, `priority` и `extras` возвращаются присланные значения. `id` — время в миллисекундах, а не ID Acksy: у gotify ID числовые. Ошибка выглядит так: `{"error":"Unauthorized","errorCode":401,"errorDescription":"…"}`.

## Тесты

Golden-тесты (`internal/adapter/*/testdata/*.http` → `*.golden`) построены на запросах из документации ntfy и gotify. После намеренного изменения разбора golden-файлы перегенерируют и **просматривают** diff:

```bash
go test ./internal/adapter/ntfy ./internal/adapter/gotify -update
```

Сквозные сценарии (публикация, обновление, clear, удаление, права токенов, формат ошибок) лежат в `internal/httpapi/adapters_test.go`.

## SMTP

Активируйте SMTP по инструкции в [README](../README.md#уведомления-по-smtp). Клиент подключается напрямую с TLS и проверкой сертификата сервера. В поле пароля SMTP AUTH укажите API-токен с правом `notifications:publish`. Имя пользователя произвольное. Домен получателей может отличаться от DNS-имени сервера.

Для проверки замените адреса примера и выполните команду. На запрос пароля введите API-токен.

```sh
curl --url smtps://smtp.example.org:465 --user acksy \
  --mail-from alerts@example.org --mail-rcpt backups@notify.example.org \
  --crlf --upload-file - <<'MAIL'
From: alerts@example.org
To: backups@notify.example.org
Subject: Backup completed

Snapshot saved successfully.
MAIL
```

Топик задаёт адрес из команды SMTP `RCPT TO`. Одна транзакция принимает одного получателя. Заголовки письма `To` и `From` не определяют топик и права. Acksy проверяет права на публикацию, ограничения токена и общий лимит отправки. Клиент передаёт письмо командой `DATA`. При обрыве до завершающей строки с точкой публикация отменяется. Успешный ответ после этой строки означает, что уведомление записано. Расширение `CHUNKING` отключено: используйте `DATA` для всех писем.

Источник уведомления — `smtp`. Поддерживаются обычный текст и MIME-письма с частью `text/plain` в UTF-8 или US-ASCII, включая base64 и quoted-printable. Берётся первая текстовая часть без имени файла. Вложения пропускаются. Письмо только с HTML отклоняется. Максимальный размер письма вместе с заголовками и вложениями — 256 КиБ, декодированный текст ограничен общим лимитом уведомлений в 64 КиБ.

В OPNsense Monit откройте **Services → Monit → Settings**. В **General Settings** укажите **Mail Server** (например, `smtp.example.org`), **Mail Server Port** `465`, **Username** `monit` и API-токен в **Password**. Включите **Secure Connection** и **Verify SSL Certificates**. В **Alert Settings** добавьте получателя `opnsense@notify.example.org`, выберите события и примените настройки. [Справка OPNsense](https://docs.opnsense.org/manual/monit.html)
