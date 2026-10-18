// Кабинет: поведение из согласованного эскиза admin.html, данные — настоящие, через API.
import { api, ApiFail } from '../lib/api';
import { dayLong, dayNum, monShort, plural, rub } from '../lib/format';
import type { AdminEvent, Booking, Content, Me, MenuItem, Notification, Order, Question, QuestionList, Settings, Signup, Staff } from '../lib/types';

const $ = <T extends Element = HTMLElement>(s: string, r: ParentNode = document) => r.querySelector(s) as T;
const $$ = <T extends Element = HTMLElement>(s: string, r: ParentNode = document) => [...r.querySelectorAll(s)] as T[];
const esc = (s: string) => s.replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c]!);
const toast = $('#toast'); let tt: ReturnType<typeof setTimeout>;
const say = (m: string) => { toast.textContent = m; toast.classList.add('show'); clearTimeout(tt); tt = setTimeout(() => toast.classList.remove('show'), 2800); };
const desk = () => matchMedia('(min-width:1024px)').matches;
$$('[data-desk]').forEach(el => { if (desk()) el.style.display = ''; });

/** Ошибка API: 401 — на вход, 403 — раздел не для этой роли или нужна смена пароля, иначе понятное сообщение. */
function oops(e: unknown, fieldsMap: Record<string, Element | null | undefined> = {}) {
  const f = e instanceof ApiFail ? e : new ApiFail(0, null);
  if (f.status === 401 && f.code !== 'bad_login') { me = null; location.hash = '#login'; route(); say('Войдите снова'); return; }
  if (f.code === 'password_change_required') { if (me) me.must_change_password = true; route(); return; }
  if (f.code === 'forbidden') { location.hash = '#bookings'; say(f.message); return; }
  Object.entries(fieldsMap).forEach(([k, el]) => el?.classList.toggle('err', k in f.fields));
  say(Object.values(f.fields)[0] || f.message);
}

// ---------- даты ----------
const pad = (n: number) => String(n).padStart(2, '0');
const isoLocal = (d: Date) => `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
const todayISO = () => isoLocal(new Date());
const addDays = (iso: string, n: number) => { const d = new Date(iso + 'T12:00:00'); d.setDate(d.getDate() + n); return isoLocal(d); };
const MON = ['января', 'февраля', 'марта', 'апреля', 'мая', 'июня', 'июля', 'августа', 'сентября', 'октября', 'ноября', 'декабря'];
function ago(ts: string, female = true) {
  const t = new Date(ts), now = new Date(), min = Math.round((now.getTime() - t.getTime()) / 60000);
  const hm = `${pad(t.getHours())}:${pad(t.getMinutes())}`;
  if (min < 1) return female ? 'пришла только что' : 'пришёл только что';
  if (min < 60) return `${female ? 'пришла' : 'пришёл'} ${min} ${plural(min, 'минуту', 'минуты', 'минут')} назад`;
  if (isoLocal(t) === todayISO()) return `сегодня, ${hm}`;
  if (isoLocal(t) === addDays(todayISO(), -1)) return `вчера, ${hm}`;
  return `${t.getDate()} ${MON[t.getMonth()]}, ${hm}`;
}
const whenLabel = (iso: string) => (iso === todayISO() ? `Сегодня, ${dayLong(iso).split(', ')[1]}` : iso === addDays(todayISO(), 1) ? `Завтра, ${dayLong(iso).split(', ')[1]}` : dayLong(iso));
const tel = (p: string) => 'tel:+' + p.replace(/\D/g, '');

// ---------- фото ----------
async function upload(file: File): Promise<string> {
  const form = new FormData(); form.append('file', file);
  const r = await api<{ url: string }>('/admin/uploads', { form });
  return r.url;
}
function setDrop(drop: HTMLElement, src: string) {
  const im = $<HTMLImageElement>('img', drop); im.hidden = !src; if (src) im.src = src;
  drop.dataset.url = src; drop.classList.toggle('has', !!src);
}

// ================= БРОНИ =================
let BK: Booking[] = [];
let bf = 'all';
const BST: Record<string, [string, string]> = { new: ['st--new', 'новая'], confirmed: ['st--ok', 'подтверждена'], cancelled: ['st--no', 'отменена'], awaiting_payment: ['st--no', 'ждёт оплаты'] };
function renderBk() {
  const t = todayISO(), wk = addDays(t, 7);
  const isToday = (b: Booking) => b.date === t, inWeek = (b: Booking) => b.date >= t && b.date <= wk;
  const list = BK.filter(b => bf === 'all' || (bf === 'new' && b.status === 'new') || (bf === 'today' && isToday(b)) || (bf === 'week' && inWeek(b)) || (bf === 'no' && b.status === 'cancelled'));
  const live = BK.filter(b => b.status !== 'cancelled' && b.status !== 'awaiting_payment');
  const n = BK.filter(b => b.status === 'new').length;
  $$('.js-new-n').forEach(e => (e.textContent = String(n))); $$('.js-new').forEach(e => { e.textContent = String(n); e.hidden = !n; });
  $('#stNewL').textContent = `${plural(n, 'новая', 'новые', 'новых')}, ${plural(n, 'ждёт', 'ждут', 'ждут')} ответа`;
  $('#stToday').textContent = String(live.filter(isToday).length);
  $('#stWeek').textContent = String(live.filter(inWeek).length);
  $('#stDog').textContent = String(live.filter(b => inWeek(b) && b.with_dog).length);
  $('#bkList').innerHTML = list.length ? list.map(b => `
      <article class="bk ${b.status === 'new' ? 'bk--new' : ''} ${b.status === 'cancelled' ? 'bk--no' : ''}">
        <div class="bk__who"><b>${esc(b.name)}</b><a href="${tel(b.phone)}">${esc(b.phone)}</a><span class="bk__ago">${ago(b.created_at)}</span></div>
        <div class="bk__when"><b>${b.time}</b><span>${whenLabel(b.date)}</span></div>
        <div class="bk__info"><span class="pill"><svg><use href="#users"/></svg>${b.guests} ${plural(b.guests, 'гость', 'гостя', 'гостей')}</span>${b.with_dog ? '<span class="pill"><svg><use href="#paw"/></svg>с собакой</span>' : ''}<em>${b.status === 'cancelled' ? 'Предоплата возвращена' : b.status === 'awaiting_payment' ? `Предоплата ${rub(b.prepay_kop)} ещё не поступила — гость на странице оплаты` : `Предоплата ${rub(b.prepay_kop)} оплачена — засчитать в счёт`}</em></div>
        <div class="bk__end"><span class="st ${BST[b.status][0]} bk__st">${BST[b.status][1]}</span>
          ${b.status === 'new' ? `<div class="bk__acts"><button class="btn btn--main btn--sm" data-ok="${b.id}">Подтвердить</button><button class="btn btn--soft btn--sm" data-no="${b.id}">Отменить</button><a class="ibtn" style="background:var(--cream)" href="${tel(b.phone)}" aria-label="Позвонить"><svg><use href="#phone"/></svg></a></div>`
            : b.status === 'confirmed' ? `<div class="bk__acts"><button class="btn btn--soft btn--sm" data-no="${b.id}">Отменить</button><a class="ibtn" style="background:var(--cream)" href="${tel(b.phone)}" aria-label="Позвонить"><svg><use href="#phone"/></svg></a></div>` : ''}
        </div>
      </article>`).join('') : '<div class="empty"><b>Пока пусто</b>Новые брони появятся здесь — и придут вам на почту и в Telegram.</div>';
}
async function loadBk() { try { BK = await api<Booking[]>('/admin/bookings'); renderBk(); } catch (e) { oops(e); } }
$('#bkList').addEventListener('click', async e => {
  const ok = (e.target as HTMLElement).closest<HTMLElement>('[data-ok]'), no = (e.target as HTMLElement).closest<HTMLElement>('[data-no]');
  if (!ok && !no) return;
  if (no && !confirm('Отменить бронь? Предоплата вернётся гостю.')) return;
  try {
    const id = (ok || no)!.dataset[ok ? 'ok' : 'no'];
    const b = await api<Booking>(`/admin/bookings/${id}/${ok ? 'confirm' : 'cancel'}`, { method: 'POST' });
    BK = BK.map(x => (x.id === b.id ? b : x)); renderBk();
    say(ok ? 'Бронь подтверждена' : 'Бронь отменена, предоплата возвращается');
  } catch (err) { oops(err); }
});
$$('[data-bf]').forEach(c => c.addEventListener('click', () => { $$('[data-bf]').forEach(x => x.classList.toggle('on', x === c)); bf = c.dataset.bf!; renderBk(); }));

// ================= ЗАКАЗЫ =================
let OR: Order[] = [];
let of = 'all';
const OST: Record<string, [string, string]> = { new: ['st--new', 'новый'], done: ['st--ok', 'готов'], cancelled: ['st--no', 'отменён'], awaiting_payment: ['st--no', 'ждёт оплаты'] };
function renderOr() {
  const list = OR.filter(o => of === 'all' || o.status === of);
  const n = OR.filter(o => o.status === 'new').length;
  $$('.js-new-on').forEach(e => (e.textContent = String(n))); $$('.js-new-o').forEach(e => { e.textContent = String(n); e.hidden = !n; });
  $('#orNewL').textContent = `${plural(n, 'новый', 'новых', 'новых')}, ${plural(n, 'ждёт', 'ждут', 'ждут')}`;
  const today = OR.filter(o => isoLocal(new Date(o.created_at)) === todayISO() && o.status !== 'cancelled' && o.status !== 'awaiting_payment');
  $('#orToday').textContent = String(today.length);
  $('#orSum').textContent = rub(today.reduce((s, o) => s + o.total_kop, 0));
  $('#orDeliv').textContent = String(today.filter(o => o.mode === 'delivery').length);
  $('#orList').innerHTML = list.length ? list.map(o => `
      <article class="bk ${o.status === 'new' ? 'bk--new' : ''} ${o.status === 'cancelled' ? 'bk--no' : ''}" data-order="${o.id}">
        <div class="bk__who"><b>Заказ №${o.id}</b><a href="${tel(o.phone)}">${esc(o.phone)}</a><span class="bk__ago">${ago(o.created_at, false)}</span></div>
        <div class="bk__when"><b>${rub(o.total_kop)}</b><span>${o.mode === 'delivery' ? 'доставка' : 'заберёт сам'}</span></div>
        <div class="bk__info">${o.items.map(i => `<span class="pill">${esc(i.name)} × ${i.qty}</span>`).join('')}${o.mode === 'delivery' ? `<em>${esc(o.address)}</em>` : ''}${o.status === 'awaiting_payment' ? '<em>Оплата ещё не поступила — не готовить</em>' : ''}</div>
        <div class="bk__end"><span class="st ${OST[o.status][0]} bk__st">${OST[o.status][1]}</span>
          ${o.status === 'new' ? `<div class="bk__acts"><button class="btn btn--main btn--sm" data-done="${o.id}">Готово</button><button class="btn btn--soft btn--sm" data-cancel="${o.id}">Отменить</button><a class="ibtn" style="background:var(--cream)" href="${tel(o.phone)}" aria-label="Позвонить"><svg><use href="#phone"/></svg></a></div>` : ''}
        </div>
      </article>`).join('') : '<div class="empty"><b>Пока пусто</b>Оплаченные заказы с собой и на доставку появятся здесь — и придут вам на почту и в Telegram.</div>';
}
async function loadOr() { try { OR = await api<Order[]>('/admin/orders'); renderOr(); } catch (e) { oops(e); } }
$('#orList').addEventListener('click', async e => {
  const done = (e.target as HTMLElement).closest<HTMLElement>('[data-done]'), cancel = (e.target as HTMLElement).closest<HTMLElement>('[data-cancel]');
  if (!done && !cancel) return;
  if (cancel && !confirm('Отменить заказ? Деньги вернутся гостю.')) return;
  try {
    const id = done ? done.dataset.done : cancel!.dataset.cancel;
    const o = await api<Order>(`/admin/orders/${id}/${done ? 'done' : 'cancel'}`, { method: 'POST' });
    OR = OR.map(x => (x.id === o.id ? o : x)); renderOr();
    say(done ? 'Заказ выполнен' : 'Заказ отменён, деньги возвращаются');
  } catch (err) { oops(err); }
});
$$('[data-of]').forEach(c => c.addEventListener('click', () => { $$('[data-of]').forEach(x => x.classList.toggle('on', x === c)); of = c.dataset.of!; renderOr(); }));

// ================= ВОПРОСЫ =================
let QS: Question[] = [];
let qf = 'wait';
const enc = encodeURIComponent;
const nick = (c: string) => c.replace(/^@/, '');
function qContact(q: Question) {
  if (q.contact_kind === 'email') return `<a href="mailto:${esc(q.contact)}">${esc(q.contact)}</a>`;
  if (q.contact) return `<a href="https://t.me/${enc(nick(q.contact))}" target="_blank" rel="noopener">${esc(q.contact)}</a>`;
  return '';
}
const QVIA: Record<string, string> = { telegram: 'отправлен в Telegram', email: 'отправлен письмом', manual: 'ответили сами' };
function qActions(q: Question) {
  const main = q.can_send ? `<button class="btn btn--main btn--sm" data-send>Отправить ${q.source === 'telegram' ? 'в Telegram' : 'письмом'}</button>`
    : q.contact_kind === 'email' ? '<button class="btn btn--main btn--sm" data-mailto>Ответить письмом</button>'
    : q.contact ? '<button class="btn btn--main btn--sm" data-tgme>Написать в Telegram</button>' : '';
  return `${main}<button class="btn btn--soft btn--sm" data-mark>Отвечено</button>`;
}
function renderQ() {
  const wait = (q: Question) => !q.answered_at;
  const n = QS.filter(wait).length;
  $$('.js-new-qn').forEach(e => (e.textContent = String(n))); $$('.js-new-q').forEach(e => { e.textContent = String(n); e.hidden = !n; });
  const list = QS.filter(q => qf === 'all' || (qf === 'wait') === wait(q));
  $('#qList').innerHTML = list.length ? list.map(q => `
      <article class="bk q ${wait(q) ? 'bk--new' : ''}" data-q="${q.id}">
        <div class="bk__who"><b>${esc(q.name)}</b>${qContact(q)}<span class="bk__ago">${ago(q.created_at, false)} · ${q.source === 'telegram' ? 'из Telegram' : 'с сайта'}</span></div>
        <span class="st ${wait(q) ? 'st--new' : 'st--ok'} bk__st">${wait(q) ? 'ждёт ответа' : 'отвечено'}</span>
        <p class="q__body">${esc(q.body)}</p>
        ${q.answered_at ? `<div class="q__ans"><small>Ответ · ${QVIA[q.answered_via] || ''}</small>${q.answer ? esc(q.answer) : '<i>без текста</i>'}</div>` : ''}
        <div class="q__form"${q.answered_at ? ' hidden' : ''}>
          <textarea class="inp" data-qa maxlength="3500" placeholder="${q.can_send ? 'Ваш ответ — уйдёт гостю' : 'Ваш ответ'}" aria-label="Ответ гостю"></textarea>
          <div class="bk__acts">${qActions(q)}</div>
        </div>
        ${q.answered_at ? '<button class="btn btn--text q__again" type="button" data-again>Ответить ещё раз</button>' : ''}
      </article>`).join('') : `<div class="empty"><b>${qf === 'wait' ? 'Все вопросы отвечены' : 'Пока пусто'}</b>Вопросы из формы на сайте и из Telegram-бота появятся здесь — и придут вам на почту и в Telegram.</div>`;
}
async function loadQ() { try { const r = await api<QuestionList>('/admin/questions'); QS = r.items; renderQ(); } catch (e) { oops(e); } }
async function answerQ(id: number, answer: string, send: boolean, done: string) {
  const q = await api<Question>(`/admin/questions/${id}/answer`, { body: { answer, send } });
  QS = QS.map(x => (x.id === q.id ? q : x)); renderQ(); say(done);
}
$('#qList').addEventListener('click', async e => {
  const t = e.target as HTMLElement, card = t.closest<HTMLElement>('[data-q]'), b = t.closest<HTMLButtonElement>('button');
  if (!card || !b) return;
  const q = QS.find(x => x.id === +card.dataset.q!); if (!q) return;
  if (b.dataset.again !== undefined) { $('.q__form', card).hidden = false; b.hidden = true; $<HTMLTextAreaElement>('[data-qa]', card).focus(); return; }
  const ta = $<HTMLTextAreaElement>('[data-qa]', card), text = ta.value.trim();
  try {
    b.disabled = true;
    if (b.dataset.send !== undefined) {
      if (!text) { ta.classList.add('err'); say('Напишите ответ'); return; }
      await answerQ(q.id, text, true, q.source === 'telegram' ? 'Ответ отправлен в Telegram' : 'Ответ отправлен письмом');
    } else if (b.dataset.mailto !== undefined) {
      // письмо из почты владелицы: тема и текст подставлены, вопрос гостя — цитатой
      location.href = `mailto:${q.contact}?subject=${enc('Ответ на ваш вопрос — КофеСобакен')}&body=${enc((text ? text + '\n\n' : '') + '— Ваш вопрос:\n' + q.body)}`;
      await answerQ(q.id, text, false, 'Открыли письмо — отправьте его из почты');
    } else if (b.dataset.tgme !== undefined) {
      if (text) await navigator.clipboard?.writeText(text).catch(() => undefined);
      window.open(`https://t.me/${enc(nick(q.contact))}`, '_blank', 'noopener');
      await answerQ(q.id, text, false, text ? 'Ответ скопирован — вставьте его в чат с гостем' : 'Отмечено: отвечено');
    } else if (b.dataset.mark !== undefined) {
      await answerQ(q.id, text, false, 'Отмечено: отвечено');
    }
  } catch (err) { oops(err, { answer: ta }); } finally { b.disabled = false; }
});
$$('[data-qf]').forEach(c => c.addEventListener('click', () => { $$('[data-qf]').forEach(x => x.classList.toggle('on', x === c)); qf = c.dataset.qf!; renderQ(); }));

// ================= МЕНЮ =================
let MI: MenuItem[] = [];
let mc = 'all', mSel = -1;
function renderMi() {
  $('#miCount').textContent = String(MI.length);
  $('#miList').innerHTML = MI.map((m, i) => (mc === 'all' || m.category === mc) ? `
      <button type="button" class="mi ${m.visible ? '' : 'off'} ${i === mSel ? 'on' : ''}" data-i="${i}">
        <span class="mi__ph">${m.photo ? `<img src="${esc(m.photo)}" alt="">` : '<svg><use href="#cup"/></svg>'}</span>
        <div><b>${esc(m.name)}${m.signature ? '<span class="tag">фирменный</span>' : ''}${m.visible ? '' : '<span class="tag tag--off">скрыто</span>'}</b><small>${m.photo ? esc(m.description) : 'нет фото · ' + esc(m.description)}</small></div>
        <span class="price">${rub(m.price_kop)}</span>
      </button>` : '').join('');
}
const menuEd = $<HTMLFormElement>('#menuEd');
const mf = (n: string) => menuEd.elements.namedItem(n) as HTMLInputElement;
function fillMenu(i: number) {
  const m = MI[i];
  $('.js-mode', menuEd).textContent = m ? 'изменить' : 'новая позиция'; $('.js-title', menuEd).textContent = m ? m.name : 'Новая позиция';
  mf('name').value = m?.name || ''; mf('desc').value = m?.description || ''; mf('price').value = m ? String(m.price_kop / 100) : '';
  mf('cat').value = m?.category || (mc === 'all' ? 'milk' : mc); mf('vis').checked = m ? m.visible : true; mf('sig').checked = !!m?.signature;
  ['name', 'price'].forEach(k => mf(k).classList.remove('err'));
  setDrop($('.drop', menuEd), m?.photo || ''); $('.js-del', menuEd).hidden = !m; counters();
}
async function loadMi() { try { MI = await api<MenuItem[]>('/admin/menu'); if (mSel >= MI.length) mSel = -1; renderMi(); fillMenu(mSel); } catch (e) { oops(e); } }
$('#miList').addEventListener('click', e => { const b = (e.target as HTMLElement).closest<HTMLElement>('.mi'); if (!b) return; mSel = +b.dataset.i!; renderMi(); fillMenu(mSel); openSheet('#menuEd'); });
$$('[data-mc]').forEach(c => c.addEventListener('click', () => { $$('[data-mc]').forEach(x => x.classList.toggle('on', x === c)); mc = c.dataset.mc!; renderMi(); }));
$$('.js-menu-new').forEach(b => b.addEventListener('click', () => { mSel = -1; renderMi(); fillMenu(-1); openSheet('#menuEd'); }));
menuEd.addEventListener('submit', async e => {
  e.preventDefault();
  const name = mf('name').value.trim(), price = Math.round(parseFloat(mf('price').value.replace(',', '.').replace(/\s/g, '')) * 100);
  mf('name').classList.toggle('err', !name); mf('price').classList.toggle('err', !(price > 0));
  if (!name || !(price > 0)) { say(!name ? 'Укажите название' : 'Укажите цену'); return; }
  const body = { name, description: mf('desc').value, price_kop: price, category: mf('cat').value, visible: mf('vis').checked, signature: mf('sig').checked, photo: $('.drop', menuEd).dataset.url || '' };
  const sub = $<HTMLButtonElement>('button[type="submit"]', menuEd); sub.disabled = true;
  try {
    const m = mSel >= 0 ? await api<MenuItem>(`/admin/menu/${MI[mSel].id}`, { method: 'PUT', body }) : await api<MenuItem>('/admin/menu', { body });
    if (mSel >= 0) MI[mSel] = m; else { MI.push(m); mSel = MI.length - 1; }
    renderMi(); fillMenu(mSel); closeSheet(); say('Сохранено — на сайте уже новое');
  } catch (err) { oops(err, { name: mf('name'), price_kop: mf('price'), description: mf('desc') }); } finally { sub.disabled = false; }
});
$('.js-del', menuEd).addEventListener('click', async () => {
  if (mSel < 0 || !confirm(`Удалить «${MI[mSel].name}» из меню?`)) return;
  try { await api(`/admin/menu/${MI[mSel].id}`, { method: 'DELETE' }); MI.splice(mSel, 1); mSel = -1; renderMi(); fillMenu(-1); closeSheet(); say('Позиция удалена'); } catch (err) { oops(err); }
});

// ================= МЕРОПРИЯТИЯ =================
let EV: AdminEvent[] = [];
let eSel = -1;
const group = (e: AdminEvent) => (e.past ? 'past' : e.status === 'draft' ? 'draft' : 'pub');
function renderEv() {
  const grp: Record<string, string> = { pub: 'Скоро на сайте', draft: 'Черновики — гости не видят', past: 'Прошли' };
  let h = '';
  (['pub', 'draft', 'past'] as const).forEach((k, gi) => {
    let items = EV.map((e, i) => [e, i] as const).filter(([e]) => group(e) === k);
    if (k === 'past') items = items.reverse();
    if (!items.length) return;
    h += `<p class="glabel" ${gi === 0 || !h ? 'style="margin-top:0"' : ''}>${grp[k]}</p>`;
    h += items.map(([e, i]) => {
      const left = e.seats_left;
      return `<button type="button" class="evc ${i === eSel ? 'on' : ''} ${k === 'past' ? 'past' : ''}" data-i="${i}">
          <span class="evc__ph">${e.photo ? `<img src="${esc(e.photo)}" alt="">` : ''}<span class="evc__d"><b>${dayNum(e.date)}</b><small>${monShort(e.date)}</small></span></span>
          <div><b class="t">${esc(e.title)}</b><span class="evc__row"><span>${esc(e.kind)} · ${e.time}</span>${k === 'draft' ? '<span class="st st--draft">черновик</span>' : k === 'past' ? '<span class="st st--no">прошло</span>' : left ? `<span>осталось ${left}</span>` : '<span class="st st--new">мест нет</span>'}</span>
          ${k === 'pub' ? `<div class="bar"><i style="width:${Math.min(100, (e.taken / e.seats) * 100)}%"></i></div><span class="evc__row"><span>записались: ${e.taken} из ${e.seats}</span>${e.price_kop ? `<span>${rub(e.price_kop)}</span>` : '<span>бесплатно</span>'}</span>` : k === 'past' ? `<span class="evc__row"><span>записались: ${e.taken}</span></span>` : ''}</div>
        </button>`;
    }).join('');
  });
  $('#evList').innerHTML = h || '<div class="empty"><b>Пока нет мероприятий</b>Нажмите «Создать мероприятие» — оно появится на сайте после «Опубликовать».</div>';
}
const evEd = $<HTMLFormElement>('#evEd');
const ef = (n: string) => evEd.elements.namedItem(n) as HTMLInputElement;
const seatsOut = $<HTMLOutputElement>('#evSeats output');
let who: Signup[] = [];
function renderWho(e: AdminEvent | undefined) {
  const w = $('#evWho');
  if (!e || e.status === 'draft' && !who.length) { w.hidden = true; return; }
  w.hidden = false;
  const tk = e.taken, total = +(seatsOut.textContent || e.seats), left = Math.max(0, total - tk);
  w.innerHTML = `<div class="who__head"><b>Записались</b><span class="who__seats">осталось <strong>${left}</strong> из ${total} ${plural(total, 'места', 'мест', 'мест')}</span></div>
      <ul>${who.length ? who.map(x => `<li><b>${esc(x.name)}</b><span><i>${x.people} чел.</i>${x.with_dog ? '<i><svg><use href="#paw"/></svg>с собакой</i>' : ''}</span><a href="${x.via === 'tg' ? 'https://t.me/' + encodeURIComponent(x.contact.slice(1)) : tel(x.contact)}" target="_blank" rel="noopener">${esc(x.contact)}</a></li>`).join('')
        : `<li><span style="grid-row:auto;color:var(--soft)">${e.past ? 'Мероприятие прошло, записей не было' : 'Пока никто не записался'}</span></li>`}</ul>`;
}
async function loadWho(e: AdminEvent | undefined) {
  who = [];
  renderWho(e);
  if (!e) return;
  try { who = await api<Signup[]>(`/admin/events/${e.id}/signups`); if (EV[eSel] === e) renderWho(e); } catch (err) { oops(err); }
}
function fillEv(i: number) {
  const e = EV[i];
  $('.js-mode', evEd).textContent = e ? (e.status === 'draft' ? 'черновик' : 'изменить') : 'новое мероприятие'; $('.js-title', evEd).textContent = e ? e.title : 'Новое мероприятие';
  ef('name').value = e?.title || ''; ef('type').value = e?.kind || 'встреча'; ef('date').value = e?.date || ''; ef('time').value = e?.time || '';
  ef('desc').value = e?.description || ''; ef('price').value = e?.price_kop ? String(e.price_kop / 100) : '';
  ef('dogs').checked = e ? e.dogs_allowed : true; ef('signup').checked = e ? e.signup_enabled : true;
  ef('date').min = todayISO();
  ['name', 'date', 'time'].forEach(k => ef(k).classList.remove('err'));
  seatsOut.textContent = String(e?.seats || 10); setDrop($('.drop', evEd), e?.photo || ''); $('.js-del', evEd).hidden = !e;
  counters(); loadWho(e);
}
async function loadEv() { try { EV = await api<AdminEvent[]>('/admin/events'); if (eSel >= EV.length) eSel = -1; renderEv(); fillEv(eSel); } catch (e) { oops(e); } }
$('#evList').addEventListener('click', e => { const b = (e.target as HTMLElement).closest<HTMLElement>('.evc'); if (!b) return; eSel = +b.dataset.i!; renderEv(); fillEv(eSel); openSheet('#evEd'); });
$$('.js-ev-new').forEach(b => b.addEventListener('click', () => { eSel = -1; renderEv(); fillEv(-1); openSheet('#evEd'); }));
$$('#evSeats button').forEach(b => b.addEventListener('click', () => { seatsOut.textContent = String(Math.min(500, Math.max(1, +(seatsOut.textContent || 1) + +b.dataset.d!))); renderWho(EV[eSel]); }));
async function saveEv(status: 'published' | 'draft') {
  let bad = false;
  ['name', 'date', 'time'].forEach(k => { const x = !ef(k).value; ef(k).classList.toggle('err', x); bad = bad || x; });
  if (bad) { say('Заполните название, дату и время'); return; }
  const price = ef('price').value.trim() ? Math.round(parseFloat(ef('price').value.replace(',', '.').replace(/\s/g, '')) * 100) : 0;
  if (!(price >= 0)) { ef('price').classList.add('err'); say('Проверьте цену'); return; }
  const body = { title: ef('name').value, kind: ef('type').value, date: ef('date').value, time: ef('time').value, description: ef('desc').value,
    seats: +(seatsOut.textContent || 10), price_kop: price, dogs_allowed: ef('dogs').checked, signup_enabled: ef('signup').checked, status, photo: $('.drop', evEd).dataset.url || '' };
  try {
    const e = eSel >= 0 ? await api<AdminEvent>(`/admin/events/${EV[eSel].id}`, { method: 'PUT', body }) : await api<AdminEvent>('/admin/events', { body });
    await loadEv(); eSel = EV.findIndex(x => x.id === e.id); renderEv(); fillEv(eSel);
    closeSheet(); say(status === 'published' ? 'Опубликовано — уже на сайте' : 'Сохранено в черновик');
  } catch (err) { oops(err, { title: ef('name'), date: ef('date'), time: ef('time'), seats: $('#evSeats'), price_kop: ef('price'), description: ef('desc') }); }
}
evEd.addEventListener('submit', e => { e.preventDefault(); saveEv('published'); });
$('.js-draft', evEd).addEventListener('click', () => saveEv('draft'));
$('.js-del', evEd).addEventListener('click', async () => {
  if (eSel < 0) return;
  const e = EV[eSel];
  if (!confirm(e.taken ? `Удалить «${e.title}»? Записались ${e.taken} чел. — предупредите их.` : `Удалить «${e.title}»?`)) return;
  try { await api(`/admin/events/${e.id}`, { method: 'DELETE' }); eSel = -1; await loadEv(); closeSheet(); say('Мероприятие удалено'); } catch (err) { oops(err); }
});

// ================= ФОТО И ТЕКСТЫ =================
let CT: Content | null = null;
let ST: Settings | null = null;
const cf = $('#contentForm');
function whyTile(w: Content['why'][number], i: number) {
  return `<div class="tile" data-why="${i}"><label class="drop ${w.photo ? 'has' : ''}" data-url="${esc(w.photo)}"><input type="file" accept="image/*" aria-label="Фото"><img src="${esc(w.photo)}" alt="" ${w.photo ? '' : 'hidden'}><span class="drop__empty"><svg><use href="#upload"/></svg>Фото</span><span class="drop__acts"><span><svg><use href="#upload"/></svg>Заменить</span></span></label><div class="fld"><input class="inp" data-f="title" maxlength="60" value="${esc(w.title)}" aria-label="Заголовок"><textarea class="inp" data-f="text" maxlength="300" aria-label="Текст">${esc(w.text)}</textarea></div></div>`;
}
function dogCard(d: Content['dogs'][number]) {
  const div = document.createElement('div'); div.className = 'dg';
  div.innerHTML = `<label class="drop ${d.photo ? 'has' : ''}" data-url="${esc(d.photo)}"><input type="file" accept="image/*" aria-label="Фото собаки"><img src="${esc(d.photo)}" alt="" ${d.photo ? '' : 'hidden'}><span class="drop__empty"><svg><use href="#upload"/></svg>Фото</span><span class="drop__acts"><span><svg><use href="#upload"/></svg>Заменить</span></span></label><input class="inp" data-f="name" maxlength="40" placeholder="Имя" aria-label="Имя собаки" value="${esc(d.name)}"><input class="inp" data-f="about" maxlength="80" placeholder="Порода, характер" aria-label="О собаке" value="${esc(d.about)}">`;
  return div;
}
function fillContent() {
  if (!CT || !ST) return;
  $$<HTMLInputElement>('[data-k]', cf).forEach(i => { const k = i.dataset.k as keyof Content; if (i.tagName === "IMG") { (i as unknown as HTMLImageElement).src = String(CT![k]); i.dataset.url = String(CT![k]); } else i.value = String(CT![k] ?? ''); });
  $('#whyTiles').innerHTML = CT.why.map(whyTile).join('');
  $$('.dg:not(.dg--add)', $('#dogsEd')).forEach(x => x.remove());
  CT.dogs.forEach(d => $('#addDog').before(dogCard(d)));
  $<HTMLInputElement>('#delivMin').value = String(ST.delivery_min_kop / 100);
  $<HTMLInputElement>('#delivRadius').value = String(ST.delivery_radius_km);
  clean(); counters();
}
async function loadContent() {
  try { [CT, ST] = await Promise.all([api<Content>('/admin/content'), api<Settings>('/admin/settings')]); fillContent(); fillNotify(); } catch (e) { oops(e); }
}
function collectContent(): Content {
  const c = { ...CT! };
  $$<HTMLInputElement>('input[data-k], textarea[data-k]', cf).forEach(i => { (c as Record<string, unknown>)[i.dataset.k!] = i.value; });
  c.hero_photo = $<HTMLImageElement>('img[data-k="hero_photo"]', cf).dataset.url || c.hero_photo;
  c.why = $$('[data-why]', cf).map(t => ({ photo: $('.drop', t).dataset.url || '', title: $<HTMLInputElement>('[data-f="title"]', t).value, text: $<HTMLTextAreaElement>('[data-f="text"]', t).value }));
  c.dogs = $$('.dg:not(.dg--add)', cf).map(t => ({ photo: $('.drop', t).dataset.url || '', name: $<HTMLInputElement>('[data-f="name"]', t).value, about: $<HTMLInputElement>('[data-f="about"]', t).value }));
  return c;
}
const bar = $('#saveBar');
function dirty() { if (!$('#s-content').hidden) { bar.classList.add('dirty'); $('#saveTxt').textContent = 'Есть несохранённые изменения'; } }
function clean() { bar.classList.remove('dirty'); $('#saveTxt').textContent = 'Всё сохранено'; }
$('#saveBtn').addEventListener('click', async () => {
  if (!CT || !ST) return;
  const minRub = parseFloat($<HTMLInputElement>('#delivMin').value.replace(/\s/g, '').replace(',', '.'));
  const radius = parseInt($<HTMLInputElement>('#delivRadius').value, 10);
  try {
    const [c, s] = await Promise.all([
      api<Content>('/admin/content', { method: 'PUT', body: collectContent() }),
      api<Settings>('/admin/settings', { method: 'PUT', body: { ...ST, delivery_min_kop: Math.round(minRub * 100), delivery_radius_km: radius } }),
    ]);
    CT = c; ST = s; fillContent(); say('Сохранено — на сайте уже новое');
  } catch (err) { oops(err, { delivery_min_kop: $('#delivMin'), delivery_radius_km: $('#delivRadius') }); }
});
$('#addDog').addEventListener('click', () => {
  if ($$('.dg:not(.dg--add)', cf).length >= 12) { say('Не больше 12 собак'); return; }
  $('#addDog').before(dogCard({ name: '', about: '', photo: '' })); dirty();
});

// загрузка фото везде: меню, мероприятия, тексты
document.addEventListener('change', async e => {
  const inp = e.target as HTMLInputElement; if (inp.type !== 'file' || !inp.files?.[0]) return;
  const drop = inp.closest<HTMLElement>('.drop'), arch = inp.closest<HTMLElement>('.arch-ed');
  const box = drop || arch; if (!box) return;
  box.classList.add('busy');
  try {
    const url = await upload(inp.files[0]);
    if (drop) setDrop(drop, url);
    if (arch) { const im = $<HTMLImageElement>('img', arch); im.src = url; im.dataset.url = url; }
    if (inp.closest('#contentForm')) dirty();
  } catch (err) { oops(err); } finally { box.classList.remove('busy'); inp.value = ''; }
});

// счётчики символов
function counters() { $$<HTMLTextAreaElement>('textarea[maxlength]').forEach(t => { const c = t.closest('.fld')?.querySelector('.cnt-txt'); if (c) c.textContent = `${t.value.length}/${t.maxLength}`; }); }
document.addEventListener('input', e => { const t = e.target as HTMLElement; if (t.matches('textarea[maxlength]')) counters(); if (t.closest('#contentForm')) dirty(); });

// ================= УВЕДОМЛЕНИЯ =================
function fillNotify() {
  if (!ST) return;
  const n = ST.notify;
  $<HTMLInputElement>('#nEmailOn').checked = n.email_enabled; $<HTMLInputElement>('#nEmail').value = n.email;
  $('#tgCard').classList.toggle('connected', n.telegram_connected); $('#tgChat').textContent = n.telegram_chat;
  $$<HTMLInputElement>('[data-n]').forEach(i => { i.checked = !!n[i.dataset.n as keyof typeof n]; });
}
async function saveNotify(msg = 'Сохранено') {
  if (!ST) return;
  const n = { ...ST.notify, email_enabled: $<HTMLInputElement>('#nEmailOn').checked, email: $<HTMLInputElement>('#nEmail').value.trim() };
  $$<HTMLInputElement>('[data-n]').forEach(i => { (n as Record<string, unknown>)[i.dataset.n!] = i.checked; });
  try { ST = await api<Settings>('/admin/settings', { method: 'PUT', body: { ...ST, notify: n } }); fillNotify(); $('#nEmail').classList.remove('err'); say(msg); }
  catch (err) { oops(err, { email: $('#nEmail') }); }
}
$('#nEmailOn').addEventListener('change', () => saveNotify());
$('#nEmail').addEventListener('change', () => saveNotify('Почта сохранена'));
$$('[data-n]').forEach(i => i.addEventListener('change', () => saveNotify()));
async function loadNotes() {
  try {
    const list = await api<Notification[]>('/admin/notifications');
    if (!list.length) return;
    $('#nLastTitle').textContent = 'Последние сообщения';
    const hm = (ts: string) => { const t = new Date(ts); return `${t.getDate()} ${MON[t.getMonth()]}, ${pad(t.getHours())}:${pad(t.getMinutes())}`; };
    $('#nBubbles').innerHTML = list.slice(0, 4).map(n => `<div class="bubble"><b>${esc(n.title)}</b><span class="bubble__body">${esc(n.body)}</span><small>${n.channel === 'email' ? 'почта' : 'Telegram'} · ${hm(n.created_at)}</small></div>${n.kind === 'booking' ? '<div class="bubble-btns"><span>Подтвердить</span><span>Позвонить</span></div>' : ''}`).join('');
  } catch (e) { oops(e); }
}
$$<HTMLButtonElement>('.js-test').forEach(b => b.addEventListener('click', async () => {
  try { await api('/admin/notify/test', { body: { channel: b.dataset.ch } }); say('Проверочное письмо отправлено'); loadNotes(); } catch (err) { oops(err); }
}));
// Подключение Telegram: ссылка на бота со «Старт»; пока владелица жмёт «Старт», раз в 3 секунды спрашиваем сервер.
// Без бота на сервере (заглушка) ссылки нет — подключено сразу.
let tgWait = 0;
const tgDone = (s: Settings) => { clearInterval(tgWait); ST = s; fillNotify(); say('Telegram подключён — проверочное сообщение отправлено'); loadNotes(); };
function waitTg() {
  clearInterval(tgWait);
  const until = Date.now() + 10 * 60_000;
  tgWait = window.setInterval(async () => {
    if (Date.now() > until) { clearInterval(tgWait); say('Не дождались «Старт» в Telegram — нажмите «Подключить Telegram» ещё раз'); return; }
    try { const s = await api<Settings>('/admin/telegram/check', { method: 'POST' }); if (s.notify.telegram_connected) tgDone(s); } catch { /* следующая попытка */ }
  }, 3000);
}
$('#tgConnect').addEventListener('click', async () => {
  const win = window.open('', '_blank'); // окно — сразу по нажатию, иначе браузер его заблокирует
  try {
    const r = await api<Settings & { telegram_link?: string }>('/admin/telegram/connect', { method: 'POST' });
    if (!r.telegram_link) { win?.close(); tgDone(r); return; }
    if (win) { win.opener = null; win.location.href = r.telegram_link; } else location.href = r.telegram_link;
    say('В Telegram нажмите «Старт» — подождём здесь');
    waitTg();
  } catch (err) { win?.close(); oops(err); }
});
$('#tgOff').addEventListener('click', async () => {
  try { ST = await api<Settings>('/admin/telegram/disconnect', { method: 'POST' }); fillNotify(); say('Telegram отключён'); } catch (err) { oops(err); }
});

// ================= ШТОРКА НА ТЕЛЕФОНЕ =================
let sheet: HTMLElement | null = null;
function openSheet(sel: string) { if (desk()) return; const el = $<HTMLElement>(sel); sheet = el; el.classList.add('open'); document.body.classList.add('sheet'); }
function closeSheet() { if (sheet) sheet.classList.remove('open'); document.body.classList.remove('sheet'); sheet = null; }
$('#ovl').addEventListener('click', closeSheet);
$$('.js-close').forEach(b => b.addEventListener('click', closeSheet));
addEventListener('keydown', e => { if (e.key === 'Escape') closeSheet(); });

// ================= СОТРУДНИКИ (только администратор) =================
let SF: Staff[] = [];
function renderStaff() {
  $('#staffList').innerHTML = SF.length ? SF.map(u => `<article class="bk ${u.active ? '' : 'bk--no'}">
      <div class="bk__who"><b>${esc(u.name)}</b><span>${esc(u.email)}</span></div>
      <div class="bk__info"><span class="pill">${u.role === 'admin' ? 'администратор' : 'сотрудник'}</span>${u.must_change_password ? '<span class="pill">ждёт первого входа</span>' : ''}</div>
      <div class="bk__end"><span class="st ${u.active ? 'st--ok' : 'st--no'} bk__st">${u.active ? 'активен' : 'отключён'}</span>
        ${me && u.id !== me.id ? `<div class="bk__acts"><button class="btn btn--soft btn--sm" data-reset="${u.id}">Новый пароль</button><button class="btn btn--soft btn--sm" data-toggle="${u.id}">${u.active ? 'Отключить' : 'Включить'}</button></div>` : '<small style="color:var(--soft)">это вы</small>'}
      </div></article>`).join('') : '<div class="empty"><b>Пока никого</b>Создайте вход первому сотруднику.</div>';
}
async function loadStaff() { try { SF = await api<Staff[]>('/admin/staff'); renderStaff(); } catch (e) { oops(e); } }
const staffForm = $<HTMLFormElement>('#staffForm');
const sf = (n: string) => staffForm.elements.namedItem(n) as HTMLInputElement;
const emailOk = (v: string) => /^\S+@\S+\.\S+$/.test(v.trim());
staffForm.addEventListener('submit', async e => {
  e.preventDefault(); let bad = false;
  ([['name', !sf('name').value.trim()], ['email', !emailOk(sf('email').value)], ['pass', sf('pass').value.length < 8]] as const)
    .forEach(([k, x]) => { sf(k).classList.toggle('err', x); bad = bad || x; });
  if (bad) { say('Имя, почта и пароль не короче 8 символов'); return; }
  const btn = $<HTMLButtonElement>('button[type="submit"]', staffForm); btn.disabled = true;
  try {
    await api<Staff>('/admin/staff', { body: { name: sf('name').value, email: sf('email').value, password: sf('pass').value, role: sf('role').value } });
    staffForm.reset(); say('Вход создан. Передайте сотруднику почту и временный пароль'); loadStaff();
  } catch (err) { oops(err, { name: sf('name'), email: sf('email'), password: sf('pass') }); } finally { btn.disabled = false; }
});
$('#staffList').addEventListener('click', async e => {
  const t = (e.target as HTMLElement).closest<HTMLElement>('[data-toggle]'), r = (e.target as HTMLElement).closest<HTMLElement>('[data-reset]');
  const u = SF.find(x => String(x.id) === (t?.dataset.toggle || r?.dataset.reset)); if (!u) return;
  try {
    if (t) {
      await api<Staff>(`/admin/staff/${u.id}`, { method: 'PUT', body: { name: u.name, email: u.email, active: !u.active } });
      say(u.active ? 'Сотрудник отключён — войти больше не сможет' : 'Сотрудник снова может войти');
    } else {
      const p = prompt('Новый временный пароль (не короче 8 символов). Передайте его сотруднику:'); if (!p) return;
      if (p.length < 8) { say('Не короче 8 символов'); return; }
      await api<Staff>(`/admin/staff/${u.id}/password`, { body: { password: p } });
      say('Новый пароль задан — при входе сотрудник сменит его');
    }
    loadStaff();
  } catch (err) { oops(err); }
});

// ================= ВХОД И РАЗДЕЛЫ =================
let me: Me | null = null;
const ADMIN_ONLY = ['content', 'notify', 'staff'];
const loginForm = $<HTMLFormElement>('#loginForm');
const setupForm = $<HTMLFormElement>('#setupForm');
const changeForm = $<HTMLFormElement>('#changeForm');
const lf = (n: string) => loginForm.elements.namedItem(n) as HTMLInputElement;
const field = (f: HTMLFormElement, n: string) => f.elements.namedItem(n) as HTMLInputElement;
const LOGIN_SUB = $('#loginSub').innerHTML;
function forms(which: 'login' | 'setup' | 'change') {
  loginForm.hidden = which !== 'login'; setupForm.hidden = which !== 'setup'; changeForm.hidden = which !== 'change';
  $('#loginSub').innerHTML = which === 'setup' ? 'Создайте вход администратора.' : which === 'change' ? 'Придумайте свой пароль.' : LOGIN_SUB;
}
/** Разделы по роли: сотруднику — брони, заказы, меню, мероприятия. Права проверяет и сервер. */
function applyRole() {
  if (!me) return;
  const admin = me.role === 'admin';
  $$('[data-s]').forEach(a => { a.hidden = !admin && ADMIN_ONLY.includes(a.dataset.s!); });
  $('#whoName').textContent = me.name || me.email; $('#whoRole').textContent = admin ? 'администратор' : 'сотрудник';
}
function pw2(f: HTMLFormElement) {
  const a = field(f, 'pass'), b = field(f, 'pass2');
  const bad = a.value.length < 8 ? 'Пароль — не короче 8 символов' : a.value !== b.value ? 'Пароли не совпадают' : '';
  a.classList.toggle('err', a.value.length < 8); b.classList.toggle('err', !!bad && a.value.length >= 8);
  if (bad) say(bad);
  return !bad;
}
function entered(m: Me) { me = m; applyRole(); if (!m.must_change_password) { loadBk(); loadOr(); loadQ(); } }

$('#showPass').addEventListener('click', () => { const p = lf('pass'); p.type = p.type === 'password' ? 'text' : 'password'; });
$('#forgot').addEventListener('click', async e => {
  e.preventDefault();
  const email = lf('email').value.trim();
  if (!email) { lf('email').classList.add('err'); say('Введите почту — пришлём на неё ссылку'); return; }
  try {
    const r = await api<{ mail: boolean }>('/admin/forgot', { body: { email } });
    say(r.mail ? 'Если эта почта есть в кабинете, на неё пришла ссылка. Она действует 1 час' : 'Попросите администратора сбросить пароль в разделе «Сотрудники»');
  } catch (err) { oops(err); }
});
loginForm.addEventListener('submit', async e => {
  e.preventDefault(); let bad = false;
  [lf('email'), lf('pass')].forEach(i => { const x = !i.value.trim(); i.classList.toggle('err', x); bad = bad || x; });
  if (bad) { say('Введите почту и пароль'); return; }
  const btn = $<HTMLButtonElement>('button[type="submit"]', loginForm); btn.disabled = true;
  try {
    const m = await api<Me>('/admin/login', { body: { email: lf('email').value, password: lf('pass').value } });
    lf('pass').value = ''; entered(m); location.hash = '#bookings'; route();
  } catch (err) { lf('pass').classList.add('err'); oops(err); } finally { btn.disabled = false; }
});

// первый вход администратора: /admin#setup/<код> — код убираем из адресной строки сразу
let setupCode = '';
async function openSetup(code: string) {
  if (code) { setupCode = code; history.replaceState(null, '', '#setup'); }
  forms('setup');
  try { const r = await api<{ email: string }>('/admin/setup/info', { body: { token: setupCode } }); field(setupForm, 'email').value = r.email; }
  catch (err) { forms('login'); history.replaceState(null, '', '#login'); oops(err); }
}
setupForm.addEventListener('submit', async e => {
  e.preventDefault(); if (!pw2(setupForm)) return;
  const btn = $<HTMLButtonElement>('button[type="submit"]', setupForm); btn.disabled = true;
  try {
    const m = await api<Me>('/admin/setup', { body: { token: setupCode, password: field(setupForm, 'pass').value } });
    setupForm.reset(); setupCode = ''; entered(m); say('Готово — вы администратор'); location.hash = '#bookings'; route();
  } catch (err) { oops(err, { password: field(setupForm, 'pass') }); } finally { btn.disabled = false; }
});
changeForm.addEventListener('submit', async e => {
  e.preventDefault(); if (!me || !pw2(changeForm)) return;
  const btn = $<HTMLButtonElement>('button[type="submit"]', changeForm); btn.disabled = true;
  try {
    const m = await api<Me>('/admin/password', { body: { new_password: field(changeForm, 'pass').value } });
    changeForm.reset(); entered(m); say('Пароль сохранён'); location.hash = '#bookings'; route();
  } catch (err) { oops(err, { new_password: field(changeForm, 'pass') }); } finally { btn.disabled = false; }
});
$$('.js-logout').forEach(b => b.addEventListener('click', async () => { try { await api('/admin/logout', { method: 'POST' }); } catch { /* всё равно выходим */ } me = null; location.hash = '#login'; route(); }));

const loaders: Record<string, () => void> = { bookings: loadBk, orders: loadOr, questions: loadQ, menu: loadMi, events: loadEv, content: loadContent, notify: () => { loadContent(); loadNotes(); }, staff: loadStaff };
function route() {
  const [s, sub] = (location.hash.slice(1) || 'bookings').split('/');
  closeSheet();
  const showLogin = (which: 'login' | 'setup' | 'change') => { $('#login').hidden = false; $('#app').hidden = true; if (which !== 'setup') forms(which); };
  if (s === 'setup' && (sub || setupCode)) { showLogin('setup'); openSetup(sub || ''); return; }
  if (!me) { showLogin('login'); return; }
  if (me.must_change_password) { field(changeForm, 'email').value = me.email; showLogin('change'); return; }
  if (s === 'login' || s === 'setup' || !$('#s-' + s) || (ADMIN_ONLY.includes(s) && me.role !== 'admin')) { history.replaceState(null, '', '#bookings'); route(); return; }
  $('#login').hidden = true; $('#app').hidden = false;
  $$('.scr').forEach(x => (x.hidden = x.id !== 's-' + s));
  $$('[data-s]').forEach(a => a.classList.toggle('on', a.dataset.s === s));
  scrollTo(0, 0);
  loaders[s]?.();
  if (s === 'menu' && sub === 'new') { mSel = -1; fillMenu(-1); openSheet('#menuEd'); }
  if (s === 'events' && sub === 'new') { eSel = -1; fillEv(-1); openSheet('#evEd'); }
}
addEventListener('hashchange', route);
// счётчики новых в меню разделов — сразу после входа
async function boot() {
  try { entered(await api<Me>('/admin/me')); } catch { me = null; }
  route();
}
boot();
// раз в минуту — свежие брони, заказы и вопросы (вопросы — не трогаем, пока владелица пишет ответ)
setInterval(() => { if (me && !me.must_change_password && !document.hidden) { loadBk(); loadOr(); if (!$$<HTMLTextAreaElement>('#qList [data-qa]').some(t => t.value)) loadQ(); } }, 60000);
