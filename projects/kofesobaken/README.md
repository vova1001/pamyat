# КофеСобакен — сайт и кабинет

Сайт кофейни с собаками (меню, бронь столика с предоплатой, заказ с собой и доставка, мероприятия с записью и оплатой)
и кабинет владельца (брони, заказы, меню, мероприятия и записавшиеся, фото и тексты, уведомления).
Устройство — `architecture.md`.

## Запуск
Нужен только Docker.
```sh
make dev            # создаст .env со случайными паролями и поднимет всё
# или без make:
docker compose up --build
```
- Сайт: http://localhost:8000
- Кабинет: http://localhost:8000/admin — почта `ADMIN_EMAIL`, пароль `ADMIN_PASSWORD` из `.env`.
  Без `.env` пароль создаётся при первом запуске и пишется в лог: `docker compose logs backend | grep пароль`.

Базовые образы берутся с зеркала `mirror.gcr.io` (Docker Hub ограничивает частоту). Другое — `REGISTRY=docker.io/library`.
За корпоративным прокси с подменой сертификатов: положите его сертификат `*.crt` в `backend/certs/` и `frontend/certs/`.

## Проверки
```sh
make test   # тесты бэка: модульные + интеграционные на PostgreSQL (в Docker), покрытие
make e2e    # e2e главных сценариев (Playwright) против поднятого make dev
make lint   # gofmt, go vet, astro check
```

## Что заглушено (до шага регистраций)
| Что | Сейчас | Как подключить |
|---|---|---|
| Оплата | тестовая страница `/pay/<id>`: «Оплатить» шлёт подписанное уведомление сайту; деньги не списываются | адаптер платёжной системы в `backend/internal/payments` |
| Почта, Telegram | сообщения пишутся в лог backend и в журнал «Уведомления» в кабинете | адаптеры в `backend/internal/notify` (SMTP, Telegram-бот) |
| Подключение Telegram | кнопка сразу «подключает» чат @ludi_bydte_dobree | бот кофейни, «Старт», привязка чата |
| Восстановление пароля | подсказка «напишите в студию» | после подключения почты |

## Структура
```
backend/    Go: api/openapi.yaml, cmd/server, internal/{service,store,payments,notify,httpapi}, migrations, queries
frontend/   Astro: src/pages (index, admin, pay/[id]), src/components/site, src/styles (CSS эскиза), src/scripts, tests/e2e
deploy/     nginx.conf — единый вход на :8000
```
Изменили SQL или контракт — `make gen` (sqlc и типы фронта).
