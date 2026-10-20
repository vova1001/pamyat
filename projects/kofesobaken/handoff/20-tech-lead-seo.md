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

## Приёмка (пишет менеджер)
