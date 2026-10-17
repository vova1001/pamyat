import { expect, test, type Page } from '@playwright/test';
import { login, uniq } from './helpers';

async function loginAs(page: Page, email: string, pass: string) {
  await page.goto('/admin#login');
  await page.locator('#loginForm input[name="email"]').fill(email);
  await page.locator('#loginForm input[name="pass"]').fill(pass);
  await page.locator('#loginForm button[type="submit"]').click();
}

test('сотрудники: администратор создаёт вход, сотрудник меняет временный пароль и видит только свои разделы; отключение закрывает доступ', async ({ page, browser }) => {
  const email = `staff-${uniq()}@example.com`;
  await login(page);
  await expect(page.locator('.snav a[data-s="staff"]')).toBeVisible();
  await page.locator('.snav a[data-s="staff"]').click();
  await expect(page.locator('#s-staff')).toBeVisible();
  await page.locator('#staffForm input[name="name"]').fill('Анна, бариста');
  await page.locator('#staffForm input[name="email"]').fill(email);
  await page.locator('#staffForm input[name="pass"]').fill('temp-pass-1');
  await page.locator('#staffForm button[type="submit"]').click();
  await expect(page.locator('#toast')).toHaveText('Вход создан. Передайте сотруднику почту и временный пароль');
  const card = page.locator('#staffList .bk', { hasText: email });
  await expect(card).toContainText('ждёт первого входа');

  // сотрудник — в отдельном браузере
  const ctx = await browser.newContext();
  const sp = await ctx.newPage();
  await loginAs(sp, email, 'temp-pass-1');
  await expect(sp.locator('#changeForm')).toBeVisible();
  await expect(sp.locator('#app')).toBeHidden();
  await sp.locator('#changeForm input[name="pass"]').fill('anna-own-1');
  await sp.locator('#changeForm input[name="pass2"]').fill('anna-own-2');
  await sp.locator('#changeForm button[type="submit"]').click();
  await expect(sp.locator('#toast')).toHaveText('Пароли не совпадают');
  await sp.locator('#changeForm input[name="pass2"]').fill('anna-own-1');
  await sp.locator('#changeForm button[type="submit"]').click();
  await expect(sp.locator('#s-bookings')).toBeVisible();
  await expect(sp.locator('#whoRole')).toHaveText('сотрудник');
  for (const s of ['bookings', 'orders', 'menu', 'events']) await expect(sp.locator(`.snav a[data-s="${s}"]`)).toBeVisible();
  for (const s of ['content', 'notify', 'staff']) await expect(sp.locator(`.snav a[data-s="${s}"]`)).toBeHidden();
  // админский раздел по адресу не открывается, API — 403
  await sp.goto('/admin#staff');
  await expect(sp.locator('#s-bookings')).toBeVisible();
  for (const p of ['/api/admin/staff', '/api/admin/content', '/api/admin/settings', '/api/admin/notifications']) {
    expect((await sp.request.get(p)).status(), p).toBe(403);
  }
  expect((await sp.request.get('/api/admin/bookings')).status()).toBe(200);

  // администратор отключает — сотрудника выкидывает, войти нельзя
  await page.reload();
  await card.getByRole('button', { name: 'Отключить' }).click();
  await expect(page.locator('#toast')).toHaveText('Сотрудник отключён — войти больше не сможет');
  await expect(card).toContainText('отключён');
  expect((await sp.request.get('/api/admin/bookings')).status()).toBe(401);
  await sp.locator('.snav a[data-s="menu"]').click();
  await expect(sp.locator('#login')).toBeVisible();
  await loginAs(sp, email, 'anna-own-1');
  await expect(sp.locator('#toast')).toHaveText('Неверная почта или пароль');

  // включить и выдать новый временный пароль
  await card.getByRole('button', { name: 'Включить' }).click();
  await expect(card).toContainText('активен');
  page.once('dialog', d => d.accept('temp-pass-2'));
  await card.getByRole('button', { name: 'Новый пароль' }).click();
  await expect(page.locator('#toast')).toHaveText('Новый пароль задан — при входе сотрудник сменит его');
  await loginAs(sp, email, 'temp-pass-2');
  await expect(sp.locator('#changeForm')).toBeVisible();
  // отключить в конце, чтобы тестовый вход не оставался рабочим
  await card.getByRole('button', { name: 'Отключить' }).click();
  await expect(card).toContainText('отключён');
  await ctx.close();
});

test('без входа: кабинет и API закрыты', async ({ page }) => {
  await page.goto('/admin#staff');
  await expect(page.locator('#loginForm')).toBeVisible();
  await expect(page.locator('#app')).toBeHidden();
  for (const p of ['/api/admin/me', '/api/admin/bookings', '/api/admin/menu', '/api/admin/staff', '/api/admin/content']) {
    expect((await page.request.get(p)).status(), p).toBe(401);
  }
});

test('первый вход по устаревшей ссылке — понятное сообщение и обычный вход', async ({ page }) => {
  await page.goto('/admin#setup/old-link-code');
  await expect(page.locator('#toast')).toContainText('Ссылка устарела');
  await expect(page.locator('#loginForm')).toBeVisible();
  expect(page.url()).not.toContain('old-link-code');
});
