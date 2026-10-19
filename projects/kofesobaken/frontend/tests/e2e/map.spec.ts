import { expect, test } from '@playwright/test';

// Карта «Как нас найти»: живая — только когда улицы и подписи реально загрузились; иначе снимок с меткой.

test('без плиток карты — снимок улиц с меткой кафе, без пустой живой карты', async ({ page }) => {
  await page.route(/openfreemap\.org/, r => r.abort());
  await page.goto('/#find');
  const map = page.locator('#map3d');
  await map.scrollIntoViewIfNeeded();
  await expect(map.locator('.map__shot img')).toHaveAttribute('src', '/img/map-streets.jpg');
  await expect(map.locator('.map__shot img')).toBeVisible();
  await expect(map.locator('.mpin')).toBeVisible();
  await page.waitForTimeout(2500);
  await expect(map).not.toHaveClass(/is-live/);
});

test('с плитками — живая карта с улицами и подписями, кафе отмечено', async ({ page }) => {
  await page.goto('/#find');
  const map = page.locator('#map3d');
  await map.scrollIntoViewIfNeeded();
  await expect(map).toHaveClass(/is-live/, { timeout: 30_000 });
  const n = await page.evaluate(() => {
    const m = (window as unknown as { kofeMap: { getStyle(): { layers: { id: string; 'source-layer'?: string }[] }; queryRenderedFeatures(o: object): unknown[] } }).kofeMap;
    const ids = (sl: string) => m.getStyle().layers.filter(l => l['source-layer'] === sl).map(l => l.id);
    return { roads: m.queryRenderedFeatures({ layers: ids('transportation') }).length, names: m.queryRenderedFeatures({ layers: ids('transportation_name') }).length };
  });
  expect(n.roads).toBeGreaterThan(0);
  expect(n.names).toBeGreaterThan(0);
  await expect(map.locator('.maplibregl-marker.mpin')).toBeVisible();
});
