import { expect, test } from '@playwright/test';
import { login, payTest, uniq } from './helpers';

// Сквозной кусок: кабинет → напиток в меню сайта → заказ с оплатой → заказ в кабинете и в уведомлениях.
test('кабинет: новый напиток → заказ на сайте → заказ и уведомление в кабинете', async ({ page }) => {
  const name = `Тыквенный раф ${uniq()}`;

  await login(page);
  await page.goto('/admin#menu');
  await page.locator('#s-menu .ph .js-menu-new').click();
  await page.locator('#menuEd input[name="name"]').fill(name);
  await page.locator('#menuEd textarea[name="desc"]').fill('Сезонный, с пряностями');
  await page.locator('#menuEd input[name="price"]').fill('520');
  await page.locator('#menuEd select[name="cat"]').selectOption('milk');
  await page.locator('#menuEd button[type="submit"]').click();
  await expect(page.locator('#toast')).toHaveText('Сохранено — на сайте уже новое');
  await expect(page.locator('#miList')).toContainText(name);

  // гость
  await page.goto('/');
  const row = page.locator('.mrow', { hasText: name });
  await expect(row).toBeVisible();
  await expect(row.locator('.price')).toHaveText('520 ₽');
  await row.locator('.add').click();
  await row.locator('.add').click();
  await expect(page.locator('#cart')).toContainText(name);
  await expect(page.locator('#total')).toHaveText('1 040 ₽');
  await page.locator('#orderPhone').fill('9001112233');
  await page.locator('#orderBtn').click();
  await expect(page).not.toHaveURL(/\/pay\//); // без согласия заказ не уходит
  await page.locator('#orderConsent').check();
  await page.locator('#orderBtn').click();
  await payTest(page);
  await expect(page.locator('#orderCard .success')).toBeVisible();
  const title = await page.locator('#orderDoneTitle').textContent();
  const num = title!.match(/№(\d+)/)![1];
  await page.locator('#orderAgain').click();
  await expect(page.locator('#cartEmpty')).toBeVisible();

  // владелец
  await page.goto('/admin#orders');
  const card = page.locator(`[data-order="${num}"]`);
  await expect(card).toContainText(`${name} × 2`);
  await expect(card).toContainText('1 040 ₽');
  await expect(card).toContainText('+7 (900) 111-22-33');
  await expect(card.locator('.st')).toHaveText('новый');
  await card.getByRole('button', { name: 'Готово' }).click();
  await expect(card.locator('.st')).toHaveText('готов');

  await page.goto('/admin#notify');
  await expect(page.locator('#nBubbles')).toContainText(`Новый заказ №${num}`);
  await expect(page.locator('#nBubbles')).toContainText(`${name} × 2`);
});

test('доставка — только от минимальной суммы, с подсказкой «добавьте ещё»', async ({ page }) => {
  await page.goto('/');
  await page.evaluate(() => localStorage.clear());
  await page.reload();
  const del = page.locator('#mDel');
  await expect(del).toBeDisabled();
  await page.locator('.mrow[data-name="Эспрессо"]').locator('.add').click();
  await expect(page.locator('#delivHint')).toHaveText('Доставка — от 1 000 ₽: добавьте ещё 700 ₽');
  await expect(del).toBeDisabled();
  for (let i = 0; i < 3; i++) await page.locator('.mrow[data-name="Латте"]').locator('.add').click();
  await expect(page.locator('#total')).toHaveText('1 500 ₽');
  await expect(del).toBeEnabled();
  await expect(page.locator('#delivHint')).toBeHidden();
  await page.locator('label[for="mDel"]').click();
  await expect(page.locator('#orderAddr')).toBeVisible();
  await page.locator('#orderPhone').fill('9001112244');
  await page.locator('#orderBtn').click();
  await expect(page.locator('#orderAddr')).toHaveClass(/err/);
  await page.locator('#orderAddr').fill('ул. Большая Садовая, 10, кв. 5');
  await page.locator('#orderConsent').check();
  await page.locator('#orderBtn').click();
  await payTest(page);
  await expect(page.locator('#orderCard .success')).toBeVisible();
});
