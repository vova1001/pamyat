// Запросы к API со стороны сервера Astro (внутренняя сеть Docker).
import type { Site } from './types';

const BACKEND = process.env.BACKEND_URL || 'http://localhost:8080';

export async function getSite(): Promise<Site> {
  const res = await fetch(BACKEND + '/api/site', { headers: { Accept: 'application/json' } });
  if (!res.ok) throw new Error('api /site: ' + res.status);
  return (await res.json()) as Site;
}

export async function getPayment(id: string): Promise<import('./types').PaymentStatus | null> {
  if (!/^[0-9a-f-]{36}$/i.test(id)) return null;
  const res = await fetch(BACKEND + '/api/payments/' + id, { headers: { Accept: 'application/json' } });
  if (!res.ok) return null;
  return (await res.json()) as import('./types').PaymentStatus;
}
