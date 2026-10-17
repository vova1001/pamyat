import { expect, test } from '@playwright/test';
import { inDays, login, payTest, uniq } from './helpers';

test('бронь столика с предоплатой 500 ₽ → в кабинете → подтверждение', async ({ page }) => {
  const name = `Анна ${uniq()}`;
  await page.goto('/');
  const f = page.locator('#bookForm');
  await f.locator('input[name="name"]').fill(name);
  await f.locator('input[name="phone"]').fill('9003334455');
  await f.locator('input[name="date"]').fill(inDays(2));
  await f.locator('input[name="time"]').fill('18:00');
  await page.locator('#guests button[data-d="1"]').click();
  await page.locator('label[for="dogY"]').click();
  await expect(f.locator('button[type="submit"]')).toContainText('Забронировать · 500 ₽');
  await f.locator('button[type="submit"]').click();
  await payTest(page);
  await expect(page.locator('#bookCard .success')).toBeVisible();
  await expect(page.locator('#bookCard .success')).toContainText('Бронь оплачена!');

  await login(page);
  const card = page.locator('#bkList .bk', { hasText: name });
  await expect(card).toContainText('18:00');
  await expect(card).toContainText('3 гостя');
  await expect(card).toContainText('с собакой');
  await expect(card).toContainText('Предоплата 500 ₽ оплачена');
  await card.getByRole('button', { name: 'Подтвердить' }).click();
  await expect(card.locator('.st')).toHaveText('подтверждена');

  await page.goto('/admin#notify');
  await expect(page.locator('#nBubbles')).toContainText(name);
});

test('бронь: неверные поля подсвечиваются, без оплаты в кабинет не попадает', async ({ page }) => {
  await page.goto('/');
  const f = page.locator('#bookForm');
  await f.locator('button[type="submit"]').click();
  await expect(f.locator('input[name="name"]')).toHaveClass(/err/);
  await expect(f.locator('input[name="phone"]')).toHaveClass(/err/);

  const name = `Отказ ${uniq()}`;
  await f.locator('input[name="name"]').fill(name);
  await f.locator('input[name="phone"]').fill('9003334466');
  await f.locator('input[name="date"]').fill(inDays(3));
  await f.locator('button[type="submit"]').click();
  await page.waitForURL(/\/pay\//);
  await page.locator('#payNo').click();
  await page.waitForURL(u => u.pathname === '/');
  await expect(page.locator('#toast')).toHaveText('Оплата не прошла — можно попробовать ещё раз');

  await login(page);
  await expect(page.locator('#bkList')).not.toContainText(name);
});
