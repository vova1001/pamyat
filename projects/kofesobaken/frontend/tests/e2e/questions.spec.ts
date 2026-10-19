import { expect, test } from '@playwright/test';
import { login, uniq } from './helpers';

test('вопрос с сайта: форма → спасибо → в кабинете «Вопросы» → отметка «отвечено»', async ({ page }) => {
  const q = `Можно прийти с большой собакой? ${uniq()}`;
  await page.goto('/');
  await page.locator('#askOpen').click();
  await expect(page.locator('#askModal')).toHaveClass(/open/);
  await page.locator('#askSubmit').click(); // пустая форма — ошибки, ничего не ушло
  await expect(page.locator('#askForm [name="name"]')).toHaveClass(/err/);
  await page.locator('#askForm [name="name"]').fill('Анна');
  await page.locator('#askContact').fill('anna@example.com');
  await page.locator('#askForm [name="question"]').fill(q);
  await page.locator('#askForm [name="consent"]').check();
  await page.locator('#askSubmit').click();
  await expect(page.locator('#askModal .success')).toBeVisible();
  await expect(page.locator('#askDoneText')).toContainText('anna@example.com');

  await login(page);
  await page.goto('/admin#questions');
  const card = page.locator('#qList [data-q]', { hasText: q });
  await expect(card).toContainText('ждёт ответа');
  await expect(card.locator('[data-mailto]')).toBeVisible(); // без своего домена — письмо из своей почты
  await card.locator('[data-qa]').fill('Да, ждём!');
  await card.locator('[data-mark]').click();
  await page.locator('[data-qf="done"]').click();
  await expect(page.locator('#qList [data-q]', { hasText: q })).toContainText('Да, ждём!');
});

test('телефон: кнопка «Задать вопрос» не перекрывает нижнюю панель @mobile', async ({ page }) => {
  await page.goto('/');
  const fab = page.locator('#askOpen');
  await expect(fab).toBeInViewport();
  await page.evaluate(() => scrollTo(0, 2500));
  await expect(page.locator('#mbar')).toHaveClass(/show/);
  await page.waitForTimeout(700);
  const a = (await fab.boundingBox())!, b = (await page.locator('#mbar').boundingBox())!;
  expect(a.y + a.height).toBeLessThanOrEqual(b.y);
});
