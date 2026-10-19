import type { ApiError } from './types';

/** Ошибка API с полями формы. */
export class ApiFail extends Error {
  status: number;
  code: string;
  fields: Record<string, string>;
  constructor(status: number, body: ApiError | null) {
    super(body?.error?.message || 'Что-то пошло не так. Попробуйте ещё раз');
    this.status = status;
    this.code = body?.error?.code || 'internal';
    this.fields = body?.error?.fields || {};
  }
}

/** Запрос к API из браузера (тот же адрес сайта, cookie кабинета уходит сама). */
export async function api<T>(path: string, init: { method?: string; body?: unknown; form?: FormData } = {}): Promise<T> {
  const headers: Record<string, string> = { Accept: 'application/json' };
  let body: BodyInit | undefined;
  if (init.form) body = init.form;
  else if (init.body !== undefined) { headers['Content-Type'] = 'application/json'; body = JSON.stringify(init.body); }
  let res: Response;
  try {
    res = await fetch('/api' + path, { method: init.method || (body ? 'POST' : 'GET'), headers, body, credentials: 'same-origin' });
  } catch {
    throw new ApiFail(0, { error: { code: 'network', message: 'Нет связи — проверьте интернет и попробуйте ещё раз' } });
  }
  if (res.status === 204) return undefined as T;
  const data = await res.json().catch(() => null);
  if (!res.ok) throw new ApiFail(res.status, data);
  return data as T;
}
