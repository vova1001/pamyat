# 20 · tech-lead · SEO-минимум + подготовка к Google Search Console

## Задача (пишет менеджер)
- Цель: сайт готов к индексации Google на текущем адресе `https://kofesobaken.onrender.com` (адрес — из `PUBLIC_URL` /
  `RENDER_EXTERNAL_URL`, не жёстко).
- Сделать:
  1. `robots.txt` (всё открыто, кроме `/admin`, `/api`, `/pay`), ссылка на sitemap; `sitemap.xml` (`/`, `/privacy`) с lastmod.
  2. `<link rel="canonical">` на каждой странице; кабинет и `/pay` — `noindex`.
  3. Open Graph и Twitter-карточка: `og:title`, `og:description`, `og:image` (1200×630 — из фото первого экрана),
     `og:url`, `og:type=website`, `og:locale=ru_RU`, `og:site_name`.
  4. Разметка JSON-LD `CafeOrCoffeeShop` (LocalBusiness): название, адрес, гео-координаты, часы 9–21 ежедневно,
     контакт (почта), цены «₽₽», `servesCuisine` «Кофе», `acceptsReservations`, ссылка на меню (`#menu`), `hasMap`.
     Данные — из настроек кабинета (адрес, часы), не жёстко.
  5. favicon (svg + 32 png + apple-touch-icon 180) из логотипа сайта; `theme-color`.
  6. alt у всех картинок (сейчас 4 без alt); у декоративных — `alt=""` + `aria-hidden`.
  7. Мета-тег подтверждения Google: `<meta name="google-site-verification" content="…">` из переменной окружения
     `GOOGLE_SITE_VERIFICATION` (пусто — тега нет). То же для Яндекса: `YANDEX_VERIFICATION`.
  8. Проверка: Lighthouse SEO ≥ 95 на мобильном, Rich Results (структурные данные без ошибок — валидатор schema.org).
- Сдать: коммит в ветку проекта (не пушить), тесты зелёные, e2e на robots/sitemap/мета.

## Отчёт (пишет исполнитель)
Сделал сам (задача на ~150 строк — быстрее, чем раздавать). Код — коммиты `e885965`, `e8e01ed` (попали в общие
коммиты по ходу) и финальный коммит задачи; не запушено, на живом сайте пока старая версия.
1. `robots.txt` и `sitemap.xml` — `frontend/src/pages/robots.txt.ts`, `sitemap.xml.ts`; закрыты `/admin`, `/api/`,
   `/pay/`; адрес — `PUBLIC_URL` → `RENDER_EXTERNAL_URL` → адрес запроса; `lastmod` — дата сборки.
2–3. Общий `<head>` — `frontend/src/components/Seo.astro`: title, description, canonical, OG (type, locale, site_name,
   title, description, url, image 1200×630 `public/img/og.jpg` из фото первого экрана), Twitter `summary_large_image`.
   Кабинет и `/pay` — `noindex, nofollow`, **без canonical и OG** (canonical + noindex — противоречивый сигнал для
   Google, поэтому canonical только у индексируемых `/` и `/privacy`).
4. JSON-LD `CafeOrCoffeeShop` — `frontend/src/lib/seo.ts`: адрес и часы — из настроек кабинета (часы разбираются из
   «ежедневно 9:00–21:00»; если в кабинете напишут иначе — часы в разметку не попадут, остальное останется),
   почта, Telegram (`sameAs`), координаты, город, `₽₽`, «Кофе», `acceptsReservations`, `hasMenu` `/#menu`, `hasMap`.
5. `favicon.svg` + `favicon-32.png` + `apple-touch-icon.png` (180, на кремовом фоне) из логотипа; `theme-color` #E4572E.
6. alt: 2 декоративные «пилюли» в «Почему мы» — `alt=""` + `aria-hidden`; фото в окне мероприятия и фото позиции
   меню получают alt при показе (название). `img` без alt на главной — 0.
7. `GOOGLE_SITE_VERIFICATION` / `YANDEX_VERIFICATION` — тег только если задано; в `.env.example`, `docker-compose.yml`,
   `render.yaml` (sync: false). Заказчик решил без Search Console — переменные пустые, тегов нет; в
   `deploy/render.md` — раздел «Поиск» (как подключить, если понадобится).
8. Проверки (локально, docker compose): Lighthouse мобильный SEO **100** (`/` и `/privacy`), Best Practices 100;
   validator.schema.org — 0 ошибок, 0 предупреждений. Rich Results Test — только по публичному адресу, после выкладки
   (шаг в `deploy/render.md`). `make test` зелёный (84.7%), e2e 25/25, новые — `frontend/tests/e2e/seo.spec.ts`
   (robots, sitemap, canonical, OG, иконки, JSON-LD, теги подтверждения, noindex, alt); noindex `/pay` — в `payTest`.
   `astro check` — 0 ошибок. Описание — `architecture.md`, «Поиск и превью ссылок».

Вопросы к заказчику: нет (телефона в брифе нет — в разметке его нет; Google его рекомендует, но не требует).

Сверх задачи (на одобрение, не делал): Lighthouse «Доступность» 86 — у окна мероприятия нет подписи для чтения с экрана,
порядок заголовков, контраст части текста, ссылки в тексте только цветом. Правка контраста меняет вид — через дизайн.
Ещё: og.jpg — снимок текущего главного фото; если в кабинете сменят главное фото, превью ссылок останется прежним.

## Приёмка (пишет менеджер)
