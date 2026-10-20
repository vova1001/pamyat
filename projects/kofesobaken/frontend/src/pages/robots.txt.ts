// robots.txt: сайт открыт для поиска, кроме кабинета, API и страницы оплаты. Адрес — из PUBLIC_URL / RENDER_EXTERNAL_URL.
import type { APIRoute } from 'astro';
import { siteUrl } from '../lib/seo';

export const GET: APIRoute = ({ url }) =>
  new Response(
    ['User-agent: *', 'Allow: /', 'Disallow: /admin', 'Disallow: /api/', 'Disallow: /pay/', '', `Sitemap: ${siteUrl(url)}/sitemap.xml`, ''].join('\n'),
    { headers: { 'Content-Type': 'text/plain; charset=utf-8', 'Cache-Control': 'public, max-age=3600' } },
  );
