# КофеСобакен — сайт и кабинет

Сайт кофейни с собаками (меню, бронь столика с предоплатой, заказ с собой и доставка, мероприятия с записью и оплатой)
и кабинет для администратора и сотрудников (брони, заказы, меню, мероприятия и записавшиеся; администратору — ещё
фото и тексты, уведомления, сотрудники).
Устройство — `architecture.md`.

## Запуск
Нужен только Docker.
```sh
make dev            # создаст .env со случайными паролями и поднимет всё
# или без make:
docker compose up --build
```
- Сайт: http://localhost:8000
- Кабинет: http://localhost:8000/admin — почта `ADMIN_EMAIL`, пароль `ADMIN_PASSWORD` из `.env` (`make dev` создаёт
  случайный — для локальной работы и e2e).
- **На сервере `ADMIN_PASSWORD` оставляем пустым**: при первом запуске в лог пишется одноразовая ссылка (72 часа),
  по ней администратор сам придумывает пароль — пароль нигде не хранится открыто и не пересылается:
  `docker compose logs backend | grep link`. Забыл пароль — новая ссылка: `docker compose exec backend server admin-link`.
  `ADMIN_PASSWORD` действует только как первый пароль: сменённый в кабинете при перезапуске не сбрасывается.
- Сотрудников заводит администратор в разделе «Сотрудники» (почта + временный пароль, при первом входе — смена).

Базовые образы берутся с зеркала `mirror.gcr.io` (Docker Hub ограничивает частоту). Другое — `REGISTRY=docker.io/library`.
За корпоративным прокси с подменой сертификатов: положите его сертификат `*.crt` в `backend/certs/` и `frontend/certs/`.

## Бесплатный старт: Render + Neon
Один образ `deploy/render/Dockerfile`, Blueprint `render.yaml`, пошагово — `deploy/render.md`.
Собрать и проверить локально: `docker build --platform linux/amd64 -f deploy/render/Dockerfile -t kofesobaken-render .`

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
| Восстановление пароля | сотруднику — администратор в «Сотрудниках»; администратору — ссылка `server admin-link` | письмо со ссылкой — после подключения почты |

## Структура
```
backend/    Go: api/openapi.yaml, cmd/server, internal/{service,store,payments,notify,httpapi}, migrations, queries
frontend/   Astro: src/pages (index, admin, pay/[id]), src/components/site, src/styles (CSS эскиза), src/scripts, tests/e2e
deploy/     nginx.conf — единый вход на :8000
```
Изменили SQL или контракт — `make gen` (sqlc и типы фронта).
