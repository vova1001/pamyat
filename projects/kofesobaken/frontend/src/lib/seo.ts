// SEO: адрес сайта, теги подтверждения поисковиков, разметка schema.org.
// Адрес и часы — из настроек кабинета (Content); остальные факты — из брифа (brief.md, п. 7).
import type { Content } from './types';

export const SITE_NAME = 'КофеСобакен';
export const CITY = 'Ростов-на-Дону';
export const EMAIL = 'vovaoleshko05@mail.ru';
export const TELEGRAM = 'https://t.me/ludi_bydte_dobree';
export const GEO = { lat: 47.229234, lng: 39.77196 }; // ул. Сарьяна, 34 — та же точка, что на карте
export const OG_IMAGE = { path: '/img/og.jpg', width: 1200, height: 630 };
export const THEME_COLOR = '#E4572E';

/** Адрес сайта без «/» в конце: PUBLIC_URL → RENDER_EXTERNAL_URL → адрес запроса. */
export function siteUrl(reqUrl?: URL): string {
  const env = process.env.PUBLIC_URL || process.env.RENDER_EXTERNAL_URL || '';
  return (env || reqUrl?.origin || 'http://localhost:8000').replace(/\/+$/, '');
}

/** Коды подтверждения Search Console и Яндекс.Вебмастера; пусто — тега нет. */
export function verification() {
  return {
    google: (process.env.GOOGLE_SITE_VERIFICATION || '').trim(),
    yandex: (process.env.YANDEX_VERIFICATION || '').trim(),
  };
}

const DAYS = ['Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday', 'Sunday'];

/** «ежедневно 9:00–21:00» → часы для schema.org; не распознали — null (лучше без часов, чем с неверными). */
export function parseHours(text: string) {
  const m = text.match(/(\d{1,2})[:.](\d{2})\s*[–—-]\s*(\d{1,2})[:.](\d{2})/);
  if (!m || !/ежедневно|без выходных|каждый день/i.test(text)) return null;
  const t = (h: string, mm: string) => `${h.padStart(2, '0')}:${mm}`;
  return { '@type': 'OpeningHoursSpecification', dayOfWeek: DAYS, opens: t(m[1], m[2]), closes: t(m[3], m[4]) };
}

export function cafeJsonLd(c: Content, base: string) {
  const hours = parseHours(c.hours);
  return {
    '@context': 'https://schema.org',
    '@type': 'CafeOrCoffeeShop',
    '@id': base + '/#cafe',
    name: SITE_NAME,
    url: base + '/',
    image: base + OG_IMAGE.path,
    logo: base + '/apple-touch-icon.png',
    email: EMAIL,
    sameAs: [TELEGRAM],
    priceRange: '₽₽',
    servesCuisine: 'Кофе',
    acceptsReservations: true,
    hasMenu: base + '/#menu',
    address: {
      '@type': 'PostalAddress',
      streetAddress: c.address,
      addressLocality: CITY,
      addressCountry: 'RU',
    },
    geo: { '@type': 'GeoCoordinates', latitude: GEO.lat, longitude: GEO.lng },
    hasMap: `https://yandex.ru/maps/?pt=${GEO.lng},${GEO.lat}&z=17&l=map`,
    ...(hours ? { openingHoursSpecification: [hours] } : {}),
  };
}

/** JSON внутри <script>: «</» не должен закрыть тег. */
export const jsonForScript = (v: unknown) => JSON.stringify(v).replace(/</g, '\\u003c');
