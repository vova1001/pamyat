// sitemap.xml: страницы для поиска. lastmod — дата сборки сайта (выкладки).
import type { APIRoute } from 'astro';
import { siteUrl } from '../lib/seo';

declare const __BUILD_DATE__: string;
const PAGES = ['/', '/privacy'];

export const GET: APIRoute = ({ url }) => {
  const base = siteUrl(url);
  const urls = PAGES.map(p => `  <url><loc>${base}${p}</loc><lastmod>${__BUILD_DATE__}</lastmod></url>`).join('\n');
  return new Response(
    `<?xml version="1.0" encoding="UTF-8"?>\n<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">\n${urls}\n</urlset>\n`,
    { headers: { 'Content-Type': 'application/xml; charset=utf-8', 'Cache-Control': 'public, max-age=3600' } },
  );
};
