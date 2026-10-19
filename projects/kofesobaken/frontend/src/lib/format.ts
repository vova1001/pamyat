// Общие форматтеры для сайта и кабинета.

export const rub = (kop: number): string => Math.round(kop / 100).toLocaleString('ru-RU') + ' ₽';

const MON = ['января', 'февраля', 'марта', 'апреля', 'мая', 'июня', 'июля', 'августа', 'сентября', 'октября', 'ноября', 'декабря'];
const MS = ['янв', 'фев', 'мар', 'апр', 'мая', 'июн', 'июл', 'авг', 'сен', 'окт', 'ноя', 'дек'];
const WD = ['вс', 'пн', 'вт', 'ср', 'чт', 'пт', 'сб'];
const WDC = ['Вс', 'Пн', 'Вт', 'Ср', 'Чт', 'Пт', 'Сб'];

/** YYYY-MM-DD → Date в полночь UTC (без сдвига часового пояса). */
export const day = (iso: string): Date => new Date(iso + 'T00:00:00Z');

export const dayNum = (iso: string) => day(iso).getUTCDate();
/** «окт, сб» — подпись на плашке даты мероприятия. */
export const dayBadge = (iso: string) => { const d = day(iso); return `${MS[d.getUTCMonth()]}, ${WD[d.getUTCDay()]}`; };
/** «Сб, 24 октября» */
export const dayLong = (iso: string) => { const d = day(iso); return `${WDC[d.getUTCDay()]}, ${d.getUTCDate()} ${MON[d.getUTCMonth()]}`; };
export const monShort = (iso: string) => MS[day(iso).getUTCMonth()];

export const plural = (n: number, one: string, few: string, many: string) => {
  const a = n % 10, b = n % 100;
  if (a === 1 && b !== 11) return one;
  if (a >= 2 && a <= 4 && (b < 12 || b > 14)) return few;
  return many;
};

const esc = (s: string) => s.replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c]!);

/**
 * Текст из кабинета → безопасный HTML: **жирный**, [УТОЧНИТЬ: …] — жёлтая плашка, как в эскизе.
 */
export const rich = (s: string): string =>
  esc(s)
    .replace(/\*\*(.+?)\*\*/g, '<b>$1</b>')
    .replace(/\[([^\]]+)\]/g, '<span class="todo">[$1]</span>');

export const cap = (s: string) => (s ? s[0].toUpperCase() + s.slice(1) : s);

/** Маска телефона из эскиза: +7 (900) 000-00-00. */
export const maskPhone = (raw: string): string => {
  let v = raw.replace(/\D/g, '');
  if (v.startsWith('8')) v = '7' + v.slice(1);
  if (!v.startsWith('7')) v = '7' + v;
  v = v.slice(0, 11);
  const p = v.slice(1);
  let s = '+7';
  if (p.length) s += ' (' + p.slice(0, 3);
  if (p.length >= 3) s += ')';
  if (p.length > 3) s += ' ' + p.slice(3, 6);
  if (p.length > 6) s += '-' + p.slice(6, 8);
  if (p.length > 8) s += '-' + p.slice(8, 10);
  return s;
};
