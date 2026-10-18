import { expect, test, type Page } from '@playwright/test';
import { login, uniq } from './helpers';

async function loginAs(page: Page, email: string, pass: string) {
  await page.goto('/admin#login');
  await page.locator('#loginForm input[name="email"]').fill(email);
  await page.locator('#loginForm input[name="pass"]').fill(pass);
  await page.locator('#loginForm button[type="submit"]').click();
}

// Пароль администратора из .env не трогаем (на нём держатся остальные e2e) — смену проверяем на сотруднике:
// «Профиль» один для всех ролей.
test('профиль: смена пароля — неверный текущий не подходит, другие устройства выходят, новый пароль работает', async ({ page, browser }) => {
  const email = `prof-${uniq()}@example.com`;
  await login(page);
  await page.locator('.snav a[data-s="staff"]').click();
  await page.locator('#staffForm input[name="name"]').fill('Проверка профиля');
  await page.locator('#staffForm input[name="email"]').fill(email);
  await page.locator('#staffForm input[name="pass"]').fill('temp-pass-1');
  await page.locator('#staffForm button[type="submit"]').click();
  await expect(page.locator('#toast')).toHaveText('Вход создан. Передайте сотруднику почту и временный пароль');

  const ctx = await browser.newContext();
  const sp = await ctx.newPage();
  await loginAs(sp, email, 'temp-pass-1');
  await sp.locator('#changeForm input[name="pass"]').fill('own-pass-1');
  await sp.locator('#changeForm input[name="pass2"]').fill('own-pass-1');
  await sp.locator('#changeForm button[type="submit"]').click();
  await expect(sp.locator('#s-bookings')).toBeVisible();

  // второе устройство того же человека
  const ctx2 = await browser.newContext();
  const sp2 = await ctx2.newPage();
  await loginAs(sp2, email, 'own-pass-1');
  await expect(sp2.locator('#s-bookings')).toBeVisible();

  await sp.locator('.side__foot a[data-s="profile"]').click();
  await expect(sp.locator('#s-profile')).toBeVisible();
  await expect(sp.locator('#profileCard input[name="email"]')).toHaveValue(email);
  await expect(sp.locator('#profileCard input[name="name"]')).toHaveValue('Проверка профиля');

  const f = sp.locator('#passForm');
  await f.locator('input[name="cur"]').fill('wrong-pass');
  await f.locator('input[name="pass"]').fill('new-pass-22');
  await f.locator('input[name="pass2"]').fill('new-pass-22');
  await f.locator('button[type="submit"]').click();
  await expect(sp.locator('#toast')).toHaveText('Неверный текущий пароль');
  await expect(f.locator('input[name="cur"]')).toHaveClass(/err/);
  await expect(sp.locator('#s-profile')).toBeVisible();

  await f.locator('input[name="cur"]').fill('own-pass-1');
  await f.locator('input[name="pass2"]').fill('new-pass-23');
  await f.locator('button[type="submit"]').click();
  await expect(sp.locator('#toast')).toHaveText('Пароли не совпадают');
  await f.locator('input[name="pass2"]').fill('new-pass-22');
  await f.locator('button[type="submit"]').click();
  await expect(sp.locator('#toast')).toHaveText('Пароль изменён — на других устройствах вход закрыт');
  await expect(f.locator('input[name="cur"]')).toHaveValue('');

  // это устройство осталось в кабинете, второе — вышло
  expect((await sp.request.get('/api/admin/me')).status()).toBe(200);
  expect((await sp2.request.get('/api/admin/me')).status()).toBe(401);
  await sp2.locator('.snav a[data-s="menu"]').click();
  await expect(sp2.locator('#login')).toBeVisible();
  await loginAs(sp2, email, 'own-pass-1');
  await expect(sp2.locator('#toast')).toHaveText('Неверная почта или пароль');
  await loginAs(sp2, email, 'new-pass-22');
  await expect(sp2.locator('#s-bookings')).toBeVisible();

  // отключить тестовый вход
  await page.reload();
  const card = page.locator('#staffList .bk', { hasText: email });
  await card.getByRole('button', { name: 'Отключить' }).click();
  await expect(card).toContainText('отключён');
  await ctx.close(); await ctx2.close();
});

test('профиль на телефоне: открывается из шапки @mobile', async ({ page }) => {
  await login(page);
  await page.locator('.top a[href="#profile"]').click();
  await expect(page.locator('#s-profile')).toBeVisible();
  await expect(page.locator('#emailForm')).toBeVisible();
  await expect(page.locator('#passForm')).toBeVisible();
});
