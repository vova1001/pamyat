# Выкладка на Render + Neon (бесплатный старт)

Сайт — один бесплатный веб-сервис Render из готового образа (`deploy/render/Dockerfile`: nginx + Astro + Go в одном
контейнере, порт из `$PORT`). База — Neon (PostgreSQL). Фото из кабинета — в PostgreSQL (`MEDIA_STORE=postgres`):
диска у бесплатного Render нет. Локально всё как раньше: `docker compose up` (фото — на томе `uploads`).
Ограничения тарифа (засыпание через 15 минут, 750 часов в месяц) — `infra/resources.md`, раздел «Бесплатно за рубежом».

## 1. Образ (студия, из `main`)
```sh
docker build --platform linux/amd64 -f deploy/render/Dockerfile -t ghcr.io/<организация>/kofesobaken:<версия> .
docker tag ghcr.io/<организация>/kofesobaken:<версия> ghcr.io/<организация>/kofesobaken:latest
docker push ghcr.io/<организация>/kofesobaken:<версия> && docker push ghcr.io/<организация>/kofesobaken:latest
```
Render запускает только `linux/amd64`. Пакет в GHCR — закрытый.

## 2. Neon
1. Проект, регион **Frankfurt (eu-central-1)** — рядом с сервисом Render; база `neondb`.
2. Строка подключения — **прямая** (без `-pooler` в адресе), с `?sslmode=require`. Её — только в переменную
   `DATABASE_URL` сервиса Render, ни в файлы, ни в переписку.
3. Схема создаётся сама: миграции применяются при каждом старте сервиса.

## 3. Render
1. Workspace Settings → **Registry Credentials** → добавить `ghcr-studio`: GitHub-логин студии и токен с правом
   только `read:packages`.
2. Создать сервис — один из двух способов (настройки одинаковые, они в `render.yaml`):
   - **Blueprint**: Render читает `render.yaml` только из Git-репозитория. Код к аккаунту заказчика не подключаем —
     нужен отдельный маленький репозиторий, где лежит один `render.yaml`. New → Blueprint → этот репозиторий;
   - **вручную**: New → Web Service → Existing Image → `ghcr.io/<организация>/kofesobaken:latest`, credential
     `ghcr-studio`, Instance Type **Free**, Region **Frankfurt**, Health Check Path `/healthz`, переменные — как в `render.yaml`.
3. Переменные, которые вводятся руками: `DATABASE_URL` (Neon), `PUBLIC_URL` (`https://<имя>.onrender.com`, позже — свой
   адрес). `PAYMENT_WEBHOOK_SECRET` Render создаёт сам. `ADMIN_PASSWORD` **не задаём**.

## 4. Первый вход администратора
Logs сервиса → строка `ссылка входа администратора` → поле `link` → открыть ссылку → придумать пароль.
Ссылка одноразовая, действует 72 часа, при перезапуске без заданного пароля — новая. Передать её заказчику — только
через хранилище доступов. Сотрудников администратор заводит сам в разделе «Сотрудники».

**Забыл пароль.** Консоли на бесплатном Render нет: Environment → `ADMIN_LINK_ON_START=true` → сервис перезапустится,
в логах новая ссылка → открыть → новый пароль → переменную **удалить** (иначе ссылка выдаётся при каждом старте).

## 5. Обновление сайта
Новый образ в GHCR → deploy hook сервиса (Settings → Deploy Hook; URL — секрет, только в хранилище и в CI):
`curl -fsS -X POST "$RENDER_DEPLOY_HOOK"`. Конкретная версия: `"$RENDER_DEPLOY_HOOK&imgURL=ghcr.io/<организация>/kofesobaken:<версия>"`.

## 6. Проверить после выкладки
- `https://<адрес>/healthz` → `{"status":"ok"}`; сайт и `/admin` открываются.
- Загрузить фото в кабинете → Manual Deploy → фото на месте.
- Адрес посетителя для ограничения попыток входа: в логах строки `"msg":"http"` поле `ip` у разных посетителей (с
  телефона и с компьютера) должно быть разным. nginx берёт **последний** адрес из `X-Forwarded-For` — его дописывает
  прокси Render, а то, что прислал сам посетитель, Render не очищает. Если у всех один и тот же адрес — сообщить
  техлиду (иначе 8 неверных паролей от любого закроют вход всем на 15 минут).

## 7. Переезд потом
База — обычный PostgreSQL: `pg_dump` из Neon → восстановление на новом сервере (фото переедут вместе с базой).
Сайт — `docker compose up -d` на сервере; для фото на диске — `MEDIA_STORE=disk` и перенос фото из таблицы `media`
(или оставить `postgres`). Хранилище S3 — ещё одна реализация `media.Store` в `backend/internal/media`.
