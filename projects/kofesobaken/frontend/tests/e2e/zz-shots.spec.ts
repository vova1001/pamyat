import { test, expect } from '@playwright/test';
const D = '/tmp/claude-0/shots/';
for (const [w, h] of [[390, 844], [1440, 900]]) {
  test(`map ${w}`, async ({ browser }) => {
    const ctx = await browser.newContext({ viewport: { width: w, height: h }, baseURL: 'http://localhost:8000' });
    const p = await ctx.newPage();
    await p.goto('/#find'); const m = p.locator('#map3d'); await m.scrollIntoViewIfNeeded();
    await expect(m).toHaveClass(/is-live/, { timeout: 30000 }); await p.waitForTimeout(1500);
    await m.screenshot({ path: `${D}map-live-${w}.png` });
    const p2 = await ctx.newPage(); await p2.route(/openfreemap/, r => r.abort());
    await p2.goto('/#find'); const m2 = p2.locator('#map3d'); await m2.scrollIntoViewIfNeeded(); await p2.waitForTimeout(2000);
    await m2.screenshot({ path: `${D}map-off-${w}.png` });
    // кабинет
    const a = await ctx.newPage();
    await a.goto('/admin#login');
    await a.locator('#loginForm input[name="email"]').fill(process.env.ADMIN_EMAIL!);
    await a.locator('#loginForm input[name="pass"]').fill(process.env.ADMIN_PASSWORD!);
    await a.locator('#loginForm button[type="submit"]').click();
    await a.goto('/admin#staff'); await a.waitForTimeout(800);
    await a.screenshot({ path: `${D}staff-${w}.png`, fullPage: true });
    await ctx.close();
  });
}
