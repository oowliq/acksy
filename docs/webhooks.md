# Исходящие вебхуки

Вебхук пересылает уведомления пользователя на URL: в Home Assistant, n8n, свой сервис, канал Discord, Slack или чат Telegram. Он срабатывает на новое уведомление и на повтор по `dedup_key` и учитывает фильтры по топикам и минимальному приоритету. Управлять вебхуками можно только из login-сессии: в веб-интерфейсе (Settings → Webhooks) или через `/api/v1/webhooks` (см. OpenAPI). В интерфейсе есть тестовая отправка, ротация секрета и очередь ожидающих и упавших доставок. Полный URL после сохранения не показывается, видно только origin: в URL может быть токен (у Discord и Slack он там всегда).

```bash
curl -b cookies.txt -H 'Content-Type: application/json' http://localhost:8080/api/v1/webhooks -d '{
  "name": "Home Assistant", "url": "http://192.168.1.5:8123/api/webhook/acksy",
  "format": "acksy", "topics": ["backups"], "min_priority": 3
}'
# → {"webhook": {...}, "secret": "whsec_…"}  — секрет показывается один раз
```

## Форматы

| `format` | Для чего | Тело |
|---|---|---|
| `acksy` (по умолчанию) | Свои сервисы, Home Assistant, n8n | JSON ниже |
| `discord` | Incoming webhook канала Discord | Embed: заголовок, markdown-тело, ссылка, цвет по приоритету. Упоминания отключены (`allowed_mentions.parse = []`), так что `@everyone` в тексте никого не пингует |
| `slack` | Incoming webhook Slack | Блоки `header`, `section` и `context`, только `plain_text`: mrkdwn Slack отличается от markdown, а `<!channel>` превратился бы в упоминание |
| `telegram` | Бот Telegram | Текстовое сообщение с заголовком, топиком, приоритетом и текстом уведомления. Длинные сообщения сокращаются до 4096 символов. |

### Подключение Telegram

1. Создайте бота через @BotFather и скопируйте токен.
2. Для личных уведомлений откройте диалог с ботом и нажмите Start. Для группы добавьте в неё бота, для канала дайте ему право публиковать сообщения.
3. В Settings → Webhooks → New webhook выберите Telegram. Введите токен и числовой ID чата (для группы он обычно отрицательный) или `@username` публичного канала.
4. Сохраните вебхук и нажмите Send test. Для смены бота или чата откройте Destination и укажите новую пару значений.

При настройке через API используйте формат `telegram` и URL вида `https://api.telegram.org/bot<TOKEN>/sendMessage?chat_id=<CHAT_ID>`. Полный URL скрывается после сохранения. Описание метода — в [Telegram Bot API](https://core.telegram.org/bots/api#sendmessage).

Формат `acksy`:

```json
{
  "event": "notification.created",
  "delivery_id": "01J…",
  "timestamp": "2026-11-09T12:00:00Z",
  "notification": {
    "id": "01J…", "user_id": "01J…", "message_id": "01J…",
    "topic": {"id": "01J…", "name": "backups"},
    "title": "Backup", "body": "**nas** finished",
    "body_html": "<p><strong>nas</strong> finished</p>\n", "body_text": "nas finished",
    "priority": 3, "tags": [], "click_url": "", "actions": [],
    "dedup_key": "", "repeat_count": 1, "source": "acksy",
    "created_at": "…", "updated_at": "…",
    "url": "https://acksy.example/?n=01J…"
  }
}
```

`event` бывает `notification.created`, `notification.updated` (повтор по dedup) или `webhook.test`. `body_html` очищен (инвариант 8), удалённые картинки из него убраны. `url` присутствует, если задан `ACKSY_BASE_URL`. Состояние прочтения в payload не входит: оно у каждого клиента своё.

## Подпись

У каждого запроса есть заголовки:

| Заголовок | Значение |
|---|---|
| `X-Acksy-Event` | Тип события |
| `X-Acksy-Delivery` | ID доставки; при ретраях не меняется, удобно для идемпотентности |
| `X-Acksy-Timestamp` | Unix-время отправки, в секундах |
| `X-Acksy-Signature` | `sha256=` + hex HMAC-SHA256 от `<timestamp>.<тело>` с секретом вебхука |

Проверка на стороне получателя (Python):

```python
import hmac, hashlib, time

def verify(secret: str, headers, body: bytes, tolerance=300) -> bool:
    ts = headers["X-Acksy-Timestamp"]
    if abs(time.time() - int(ts)) > tolerance:  # защита от повторной отправки
        return False
    mac = hmac.new(secret.encode(), ts.encode() + b"." + body, hashlib.sha256).hexdigest()
    return hmac.compare_digest("sha256=" + mac, headers["X-Acksy-Signature"])
```

Секрет меняется через `POST /webhooks/{id}/rotate-secret`.

## Доставка и ретраи

- Доставка ставится в очередь в БД в той же транзакции, что и уведомление. Если процесс упадёт после публикации, она всё равно не потеряется. Несколько инстансов разбирают одну очередь без дублей: каждая доставка захватывается атомарно.
- **Успех:** любой 2xx, после него доставка удаляется из очереди.
- **Повтор:** сетевые ошибки, таймаут, 408, 425, 429 и 5xx. Интервалы: 30 с, 2 мин, 10 мин, 30 мин, 1 ч, 3 ч, 6 ч, всего 8 попыток, около 10 часов. `Retry-After` учитывается, но не больше часа.
- **Окончательный отказ:** остальные 4xx (ретраи там бесполезны), а также адрес в приватной сети без разрешения. Неудачные доставки видны в `GET /webhooks/{id}/deliveries` и удаляются через срок из Settings → Server → Webhooks (по умолчанию 7 дней).
- У вебхука видны `last_success_at`, `last_failure_at` и `last_error`. URL в ответах API показывается только до хоста: у Discord и Slack секрет зашит в путь.
- Порядок доставки не гарантирован. Используйте `notification.updated_at` и `X-Acksy-Delivery`.

## Сеть

По умолчанию вебхуки не ходят на приватные, loopback и link-local адреса: иначе пользователь мог бы заставить сервер обращаться к внутренней сети (SSRF). Для homelab, где Home Assistant живёт в LAN, разрешите адреса в локальной сети в Settings → Server → Webhooks (только админ из login-сессии). Делайте это, только если доверяете всем пользователям инстанса.

`POST /webhooks/{id}/test` сразу отправляет тестовый запрос и возвращает ответ получателя.
