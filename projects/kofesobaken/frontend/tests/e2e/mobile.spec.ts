import { expect, test } from '@playwright/test';
import { login } from './helpers';

test('телефон: первый экран — заголовок и главная кнопка видны, без горизонтальной прокрутки @mobile', async ({ page }) => {
  await page.goto('/');
  await expect(page.locator('.hero h1')).toBeInViewport();
  await expect(page.locator('.hero__btns .btn--main')).toBeInViewport();
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - innerWidth);
  expect(overflow).toBeLessThanOrEqual(0);
  await page.locator('#burger').click();
  await expect(page.locator('.mnav')).toContainText('Мероприятия');
});

test('телефон: кабинет — нижнее меню с заказами, редактор меню шторкой @mobile', async ({ page }) => {
  await login(page);
  await expect(page.locator('.tabbar')).toBeVisible();
  await page.locator('.tabbar a[data-s="orders"]').click();
  await expect(page.locator('#s-orders')).toBeVisible();
  await page.locator('.tabbar a[data-s="menu"]').click();
  await page.locator('#miList .mi').first().click();
  await expect(page.locator('#menuEd')).toHaveClass(/open/);
});
