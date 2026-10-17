import { expect, test } from '@playwright/test';
import { inDays, login, payTest, uniq } from './helpers';

test('мероприятие: создать в кабинете → запись с оплатой на сайте → записавшиеся в кабинете', async ({ page }) => {
  const title = `Мастер-класс ${uniq()}`;
  const guest = `Мария ${uniq()}`;

  await login(page);
  await page.goto('/admin#events');
  await page.locator('#s-events .js-ev-new').first().click();
  const ed = page.locator('#evEd');
  await ed.locator('input[name="name"]').fill(title);
  await ed.locator('select[name="type"]').selectOption('мастер-класс');
  await ed.locator('input[name="date"]').fill(inDays(5));
  await ed.locator('input[name="time"]').fill('19:00');
  await ed.locator('textarea[name="desc"]').fill('Рисуем на пенке');
  for (let i = 0; i < 6; i++) await page.locator('#evSeats button[data-d="-1"]').click(); // 10 → 4
  await ed.locator('input[name="price"]').fill('700');
  await ed.locator('button[type="submit"]').click();
  await expect(page.locator('#toast')).toHaveText('Опубликовано — уже на сайте');

  await page.goto('/#events');
  const card = page.locator('.ev', { hasText: title });
  await expect(card).toContainText('700 ₽ с человека');
  await expect(card.locator('.seats')).toContainText('осталось 4 из 4 мест');
  await card.locator('.js-signup').click();
  const m = page.locator('#evModal');
  await expect(m).toHaveClass(/open/);
  await m.locator('input[name="name"]').fill(guest);
  await m.locator('label[for="viaT"]').click();
  await m.locator('#evContact').fill('maria_corgi');
  await m.locator('#evPeople button[data-d="1"]').click();
  await expect(m.locator('#evSubmit')).toContainText('Записаться · 1 400 ₽');
  await m.locator('#evSubmit').click();
  await payTest(page);
  await expect(page.locator('#evModal')).toHaveClass(/is-sent/);
  await expect(page.locator('#evModal .success')).toContainText('Вы записаны!');
  await page.locator('#evModal .success [data-close]').click();
  await page.reload();
  await expect(page.locator('.ev', { hasText: title }).locator('.seats')).toContainText('осталось 2 из 4 мест');

  await page.goto('/admin#events');
  await page.locator('.evc', { hasText: title }).click();
  await expect(page.locator('#evWho')).toContainText(guest);
  await expect(page.locator('#evWho')).toContainText('@maria_corgi');
  await expect(page.locator('#evWho')).toContainText('2 чел.');
  await expect(page.locator('#evWho .who__seats')).toContainText('осталось 2 из 4 мест');
});

test('черновик мероприятия гости не видят', async ({ page }) => {
  const title = `Черновик ${uniq()}`;
  await login(page);
  await page.goto('/admin#events');
  await page.locator('#s-events .js-ev-new').first().click();
  const ed = page.locator('#evEd');
  await ed.locator('input[name="name"]').fill(title);
  await ed.locator('input[name="date"]').fill(inDays(9));
  await ed.locator('input[name="time"]').fill('18:00');
  await ed.locator('.js-draft').click();
  await expect(page.locator('#toast')).toHaveText('Сохранено в черновик');
  await page.goto('/');
  await expect(page.locator('#events')).not.toContainText(title);
});
