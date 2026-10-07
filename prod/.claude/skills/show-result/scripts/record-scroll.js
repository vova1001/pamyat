// Видео плавной прокрутки всего сайта: один снимок на кадр → ffmpeg.
// node record-scroll.js <page.html|url> <out.mp4> [pxPerSec=280] [fps=30] [clickSelector]
const { chromium } = require('playwright');
const { spawn } = require('child_process');
const path = require('path');
const [src, out, pxPerSec = '280', fps = '30', click] = process.argv.slice(2);
const url = /^https?:/.test(src) ? src : 'file://' + path.resolve(src);
(async () => {
  const b = await chromium.launch({ executablePath: process.env.CHROMIUM_PATH || undefined, args: ['--no-sandbox', '--autoplay-policy=no-user-gesture-required'] });
  const p = await b.newPage({ viewport: { width: 1280, height: 720 } });
  await p.goto(url, { waitUntil: 'networkidle' });
  await p.waitForTimeout(1800);
  if (click) { const el = await p.$(click); if (el) await el.click(); }
  await p.waitForTimeout(1200);
  const ff = spawn('ffmpeg', ['-loglevel', 'error', '-y', '-f', 'image2pipe', '-framerate', fps, '-i', '-',
    '-c:v', 'libx264', '-crf', '23', '-preset', 'medium', '-pix_fmt', 'yuv420p', '-movflags', '+faststart', out], { stdio: ['pipe', 'inherit', 'inherit'] });
  const shot = async () => ff.stdin.write(await p.screenshot({ type: 'jpeg', quality: 88 }));
  const step = +pxPerSec / +fps;
  for (let i = 0; i < +fps * 1.2; i++) await shot();
  let y = 0, frames = 0;
  while (true) {
    const H = await p.evaluate(() => document.documentElement.scrollHeight - innerHeight);
    if (y >= H) break;
    y = Math.min(H, y + step);
    await p.evaluate(y => scrollTo({ top: y, behavior: 'instant' }), y);
    await shot(); frames++;
  }
  for (let i = 0; i < +fps * 1.5; i++) await shot();
  ff.stdin.end();
  await new Promise(r => ff.on('close', r));
  console.log(JSON.stringify({ out, seconds: +((frames + +fps * 2.7) / +fps).toFixed(1) }));
  await b.close();
})();
