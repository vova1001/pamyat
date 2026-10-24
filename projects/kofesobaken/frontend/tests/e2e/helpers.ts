import { expect, type Page } from '@playwright/test';

export const ADMIN_EMAIL = process.env.ADMIN_EMAIL || 'vovaoleshko05@mail.ru';
export const ADMIN_PASSWORD = process.env.ADMIN_PASSWORD || '';

export async function login(page: Page) {
  if (!ADMIN_PASSWORD) throw new Error('Задайте ADMIN_PASSWORD (как в .env) для e2e');
  await page.goto('/admin#login');
  await page.locator('#loginForm input[name="email"]').fill(ADMIN_EMAIL);
  await page.locator('#loginForm input[name="pass"]').fill(ADMIN_PASSWORD);
  await page.locator('#loginForm button[type="submit"]').click();
  await expect(page.locator('#s-bookings')).toBeVisible();
}

/** Страница тестовой оплаты → «Оплатить» → возврат на сайт. */
export async function payTest(page: Page) {
  await page.waitForURL(/\/pay\//);
  await expect(page.getByText('Тестовая оплата — деньги не списываются')).toBeVisible();
  await expect(page.locator('head meta[name="robots"]')).toHaveAttribute('content', /noindex/); // страница оплаты не в поиске
  await page.locator('#payOk').click();
  await page.waitForURL(u => u.pathname === '/');
}

export const uniq = () => Date.now().toString(36).slice(-5);

/** Дата через n дней (YYYY-MM-DD) по местному времени. */
export function inDays(n: number) {
  const d = new Date(); d.setDate(d.getDate() + n);
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}
