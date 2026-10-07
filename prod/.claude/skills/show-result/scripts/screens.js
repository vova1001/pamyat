// Скриншоты для заказчика: первый экран компьютера и телефона + вся страница.
// node screens.js <page.html|url> <outDir> [clickSelector]
const { chromium } = require('playwright');
const path = require('path');
const [src, dir, click] = process.argv.slice(2);
const url = /^https?:/.test(src) ? src : 'file://' + path.resolve(src);
(async () => {
  const b = await chromium.launch({ executablePath: process.env.CHROMIUM_PATH || undefined, args: ['--no-sandbox'] });
  for (const [n, w, h, d] of [['desktop', 1440, 900, 1], ['mobile', 390, 844, 2]]) {
    const p = await b.newPage({ viewport: { width: w, height: h }, deviceScaleFactor: d });
    await p.goto(url, { waitUntil: 'networkidle' }); await p.waitForTimeout(1800);
    if (click) { const el = await p.$(click); if (el) await el.click(); }
    await p.waitForTimeout(1200);
    await p.screenshot({ path: path.join(dir, `first-${n}.png`) });
    if (n === 'desktop') {
      // проявить все анимации появления, затем снять страницу целиком
      const H = await p.evaluate(() => document.body.scrollHeight);
      for (let y = 0; y < H; y += 600) { await p.evaluate(y => scrollTo({ top: y, behavior: 'instant' }), y); await p.waitForTimeout(60); }
      await p.evaluate(() => { scrollTo({ top: 0, behavior: 'instant' }); document.querySelectorAll('.rv,.rvo,.clip,.line').forEach(e => e.classList.add('in')); });
      await p.waitForTimeout(1200);
      await p.screenshot({ path: path.join(dir, 'full-desktop.jpg'), fullPage: true, type: 'jpeg', quality: 80 });
    }
    await p.close();
  }
  await b.close();
})();
