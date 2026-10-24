import { expect, test } from '@playwright/test';

// Адрес сайта в тегах — из PUBLIC_URL (как у сервера); по умолчанию — адрес, на котором идут тесты.
const BASE = (process.env.PUBLIC_URL || process.env.E2E_BASE_URL || 'http://localhost:8000').replace(/\/+$/, '');

test('robots.txt: всё открыто, кроме кабинета, API и оплаты; ссылка на sitemap', async ({ request }) => {
  const res = await request.get('/robots.txt');
  expect(res.status()).toBe(200);
  expect(res.headers()['content-type']).toContain('text/plain');
  const txt = await res.text();
  expect(txt).toContain('User-agent: *');
  for (const p of ['/admin', '/api/', '/pay/']) expect(txt).toContain(`Disallow: ${p}`);
  expect(txt).toContain(`Sitemap: ${BASE}/sitemap.xml`);
});

test('sitemap.xml: главная и политика с lastmod', async ({ request }) => {
  const res = await request.get('/sitemap.xml');
  expect(res.status()).toBe(200);
  expect(res.headers()['content-type']).toContain('xml');
  const xml = await res.text();
  expect(xml).toContain(`<loc>${BASE}/</loc>`);
  expect(xml).toContain(`<loc>${BASE}/privacy</loc>`);
  expect(xml.match(/<lastmod>\d{4}-\d{2}-\d{2}<\/lastmod>/g)).toHaveLength(2);
  expect(xml).not.toContain('/admin');
});

test('главная: canonical, превью ссылок, иконки, разметка кофейни', async ({ page, request }) => {
  await page.goto('/?payment=x');
  const head = page.locator('head');
  await expect(head.locator('link[rel="canonical"]')).toHaveAttribute('href', `${BASE}/`);
  await expect(head.locator('meta[name="robots"]')).toHaveCount(0);
  const og = (p: string) => head.locator(`meta[property="${p}"]`);
  await expect(og('og:type')).toHaveAttribute('content', 'website');
  await expect(og('og:locale')).toHaveAttribute('content', 'ru_RU');
  await expect(og('og:site_name')).toHaveAttribute('content', 'КофеСобакен');
  await expect(og('og:url')).toHaveAttribute('content', `${BASE}/`);
  await expect(og('og:title')).toHaveAttribute('content', /КофеСобакен/);
  await expect(og('og:description')).toHaveAttribute('content', /.+/);
  await expect(og('og:image')).toHaveAttribute('content', `${BASE}/img/og.jpg`);
  await expect(head.locator('meta[name="twitter:card"]')).toHaveAttribute('content', 'summary_large_image');
  await expect(head.locator('meta[name="theme-color"]')).toHaveAttribute('content', /^#/);

  for (const href of ['/favicon.svg', '/favicon-32.png', '/apple-touch-icon.png', '/img/og.jpg']) {
    await expect(head.locator(`link[href="${href}"], meta[content$="${href}"]`).first()).toBeAttached();
    expect((await request.get(href)).status(), href).toBe(200);
  }

  const ld = JSON.parse((await page.locator('script[type="application/ld+json"]').textContent()) || '{}');
  expect(ld['@type']).toBe('CafeOrCoffeeShop');
  expect(ld.name).toBe('КофеСобакен');
  expect(ld.address.streetAddress).toBe(await page.locator('.hero__meta span').first().innerText());
  expect(ld.geo.latitude).toBeGreaterThan(47);
  expect(ld.openingHoursSpecification?.[0]).toMatchObject({ opens: '09:00', closes: '21:00' });
  expect(ld.openingHoursSpecification[0].dayOfWeek).toHaveLength(7);
  expect(ld).toMatchObject({ priceRange: '₽₽', servesCuisine: 'Кофе', acceptsReservations: true, hasMenu: `${BASE}/#menu` });
  expect(ld.email).toContain('@');
  expect(ld.hasMap).toMatch(/^https:\/\//);
});

test('подтверждение поисковиков: тег есть только при заданной переменной', async ({ page }) => {
  await page.goto('/');
  for (const [name, env] of [['google-site-verification', 'GOOGLE_SITE_VERIFICATION'], ['yandex-verification', 'YANDEX_VERIFICATION']]) {
    const tag = page.locator(`head meta[name="${name}"]`);
    if (process.env[env]) await expect(tag).toHaveAttribute('content', process.env[env]!);
    else await expect(tag).toHaveCount(0);
  }
});

// noindex страницы оплаты проверяет payTest (helpers.ts) в сценариях брони, заказа и записи
test('политика — canonical; кабинет — noindex', async ({ page }) => {
  await page.goto('/privacy');
  await expect(page.locator('head link[rel="canonical"]')).toHaveAttribute('href', `${BASE}/privacy`);
  await page.goto('/admin');
  await expect(page.locator('head meta[name="robots"]')).toHaveAttribute('content', /noindex/);
  await expect(page.locator('head link[rel="canonical"]')).toHaveCount(0);
  await expect(page.locator('head meta[property="og:title"]')).toHaveCount(0);
});

test('у всех картинок главной есть alt; декоративные скрыты от чтения с экрана', async ({ page }) => {
  await page.goto('/');
  await expect(page.locator('img:not([alt])')).toHaveCount(0);
  const decor = page.locator('.pill img');
  expect(await decor.count()).toBeGreaterThan(0);
  for (const img of await decor.all()) {
    await expect(img).toHaveAttribute('alt', '');
    await expect(img).toHaveAttribute('aria-hidden', 'true');
  }
});
