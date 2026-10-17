import { expect, test } from '@playwright/test';
import { login, uniq } from './helpers';

test('фото и тексты: изменения из кабинета сразу на сайте', async ({ page }) => {
  const chip = `Кофейня с собаками · ${uniq()}`;
  await login(page);
  await page.goto('/admin#content');
  const inp = page.locator('[data-k="hero_chip"]');
  await expect(inp).not.toHaveValue('');
  const old = await inp.inputValue();
  await inp.fill(chip);
  await expect(page.locator('#saveBar')).toHaveClass(/dirty/);
  await page.locator('#saveBtn').click();
  await expect(page.locator('#toast')).toHaveText('Сохранено — на сайте уже новое');
  await page.goto('/');
  await expect(page.locator('.hero .chip')).toHaveText(chip);
  // вернуть как было
  await page.goto('/admin#content');
  await page.locator('[data-k="hero_chip"]').fill(old);
  await page.locator('#saveBtn').click();
  await expect(page.locator('#toast')).toHaveText('Сохранено — на сайте уже новое');
});

test('кабинет закрыт без входа, неверный пароль не пускает', async ({ page }) => {
  await page.goto('/admin#bookings');
  await expect(page.locator('#login')).toBeVisible();
  await page.locator('#loginForm input[name="email"]').fill('vovaoleshko05@mail.ru');
  await page.locator('#loginForm input[name="pass"]').fill('неверный-пароль');
  await page.locator('#loginForm button[type="submit"]').click();
  await expect(page.locator('#toast')).toHaveText('Неверная почта или пароль');
  await expect(page.locator('#app')).toBeHidden();
  const r = await page.request.get('/api/admin/bookings');
  expect(r.status()).toBe(401);
});
