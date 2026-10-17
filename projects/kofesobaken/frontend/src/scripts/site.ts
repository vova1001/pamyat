// Поведение сайта: анимации и интерактив из согласованного эскиза + настоящие заказ, бронь и запись через API.
import { api, ApiFail } from '../lib/api';
import { maskPhone, rub } from '../lib/format';
import type { Checkout, PaymentStatus } from '../lib/types';

const $ = <T extends Element = HTMLElement>(s: string, r: ParentNode = document) => r.querySelector(s) as T;
const $$ = <T extends Element = HTMLElement>(s: string, r: ParentNode = document) => [...r.querySelectorAll(s)] as T[];
const reduce = matchMedia('(prefers-reduced-motion: reduce)').matches;

// ---------- заголовок по буквам ----------
let d = 0;
$$('[data-split]').forEach(el => {
  el.innerHTML = [...(el.textContent || '')].map(c => `<span class="ch" style="animation-delay:${(0.15 + d++ * 0.04).toFixed(2)}s">${c}</span>`).join('');
});

// ---------- шапка, нижняя панель, параллакс ----------
const hdr = $('#hdr'), mbar = $('#mbar'), hero = $('#hero');
const phrase = $('#phrase'), words = $$('#phrase .w');
function phraseUpdate() {
  const r = phrase.getBoundingClientRect(), vh = innerHeight;
  const p = Math.min(1, Math.max(0, (vh * 0.85 - r.top) / (vh * 0.5)));
  words.forEach((w, i) => w.classList.toggle('on', i < Math.round(p * words.length)));
}
const onScroll = () => {
  const y = scrollY;
  hdr.classList.toggle('is-scrolled', y > 20);
  mbar.classList.toggle('show', y > hero.offsetHeight * 0.7);
  if (!reduce) $$('[data-speed]').forEach(el => { el.style.translate = `0 ${y * parseFloat(el.dataset.speed || '0') * -1}px`; });
  phraseUpdate();
};

// ---------- меню на телефоне ----------
const burger = $('#burger');
burger.addEventListener('click', () => { const o = document.body.classList.toggle('menu-open'); burger.setAttribute('aria-expanded', String(o)); });
$$('.mnav a').forEach(a => a.addEventListener('click', () => { document.body.classList.remove('menu-open'); burger.setAttribute('aria-expanded', 'false'); }));

// ---------- появление блоков ----------
const io = new IntersectionObserver(es => es.forEach(e => { if (e.isIntersecting) { e.target.classList.add('in'); io.unobserve(e.target); } }), { threshold: 0.12, rootMargin: '0px 0px -40px 0px' });
$$('.rv, .curtain').forEach(el => io.observe(el));

// ---------- следы лап (компьютер) ----------
const trail = $('#pawTrail'); let last = 0, flip = false;
if (!reduce && matchMedia('(pointer:fine)').matches) hero.addEventListener('mousemove', e => {
  const now = performance.now(); if (now - last < 140) return; last = now; flip = !flip;
  const r = hero.getBoundingClientRect();
  const s = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
  s.innerHTML = '<use href="#paw"/>';
  s.style.left = (e.clientX - r.left + (flip ? 10 : -10)) + 'px'; s.style.top = (e.clientY - r.top) + 'px';
  s.style.transform = `rotate(${Math.random() * 40 - 20}deg)`;
  trail.appendChild(s); setTimeout(() => s.remove(), 1400);
});

// ---------- вкладки меню и фото ----------
const photos = $$<HTMLImageElement>('#menuPhoto img');
const itemPhoto = photos.find(p => p.dataset.cat === 'item')!;
let cat = 'all';
const showPhoto = (c: string) => photos.forEach(p => p.classList.toggle('on', p.dataset.cat === c));
$$('.tab').forEach(t => t.addEventListener('click', () => {
  $$('.tab').forEach(x => x.setAttribute('aria-selected', String(x === t)));
  cat = t.dataset.f || 'all';
  $$('.mrow').forEach(r => r.classList.toggle('hide', cat !== 'all' && r.dataset.c !== cat));
  showPhoto(cat);
}));
// Фото позиции из кабинета — в большом фото слева, пока курсор на строке.
$$('.mrow[data-photo]').forEach(r => {
  r.addEventListener('mouseenter', () => { itemPhoto.src = r.dataset.photo!; itemPhoto.alt = r.dataset.name || ''; showPhoto('item'); });
  r.addEventListener('mouseleave', () => showPhoto(cat));
});

// ---------- подсказка ----------
const toast = $('#toast'); let tt: ReturnType<typeof setTimeout>;
const say = (m: string) => { toast.textContent = m; toast.classList.add('show'); clearTimeout(tt); tt = setTimeout(() => toast.classList.remove('show'), 2800); };

// Ошибки API → подсветка полей и понятный текст.
function showFail(e: unknown, map: Record<string, HTMLElement | null> = {}) {
  const f = e instanceof ApiFail ? e : new ApiFail(0, null);
  Object.entries(map).forEach(([k, el]) => el?.classList.toggle('err', k in f.fields));
  const first = Object.values(f.fields)[0];
  say(first || f.message);
}

// Переход на страницу оплаты.
function pay(c: Checkout) {
  if (c.confirmation_url) location.href = c.confirmation_url;
}

// ---------- корзина ----------
type Line = { name: string; price: number; q: number };
const CART_KEY = 'ks_cart';
const cart = new Map<string, Line>();
const menuIds = new Set($$('.mrow').map(r => r.dataset.id!));
try {
  const saved = JSON.parse(localStorage.getItem(CART_KEY) || '[]') as [string, Line][];
  saved.forEach(([id, it]) => {
    const row = $(`.mrow[data-id="${id}"]`);
    if (menuIds.has(id) && row) cart.set(id, { ...it, name: row.dataset.name!, price: +row.dataset.price! });
  });
} catch { /* пустая корзина */ }
const orderCard = $('#orderCard'), mDel = $<HTMLInputElement>('#mDel'), mPick = $<HTMLInputElement>('#mPick');
const minKop = +(orderCard.dataset.min || 0);
const hint = $('#delivHint');
const sum = () => [...cart.values()].reduce((s, it) => s + it.price * it.q, 0);

function renderCart() {
  const ul = $('#cart'); ul.innerHTML = '';
  cart.forEach((it, id) => {
    const li = document.createElement('li');
    li.innerHTML = `<div><b></b><small>${rub(it.price)}</small></div><div class="qty"><button aria-label="Убрать" data-id="${id}" data-d="-1">−</button><span>${it.q}</span><button aria-label="Добавить" data-id="${id}" data-d="1">+</button></div><b>${rub(it.price * it.q)}</b>`;
    li.querySelector('div b')!.textContent = it.name;
    ul.appendChild(li);
  });
  $('#cartEmpty').style.display = cart.size ? 'none' : 'block';
  const total = sum();
  $('#total').textContent = rub(total);
  // Доставка — только от минимальной суммы (решение заказчика), ниже — только с собой.
  const short = minKop - total;
  const canDeliver = short <= 0;
  mDel.disabled = !canDeliver;
  orderCard.classList.toggle('no-delivery', !canDeliver);
  if (!canDeliver && mDel.checked) { mPick.checked = true; setMode(); }
  hint.textContent = canDeliver ? '' : total > 0 ? `Доставка — от ${rub(minKop)}: добавьте ещё ${rub(short)}` : `Доставка — бесплатно, при заказе от ${rub(minKop)}`;
  localStorage.setItem(CART_KEY, JSON.stringify([...cart.entries()]));
}
$('#cart').addEventListener('click', e => {
  const b = (e.target as HTMLElement).closest('button'); if (!b) return;
  const it = cart.get(b.dataset.id!); if (!it) return;
  it.q = Math.min(20, it.q + +b.dataset.d!);
  if (it.q <= 0) cart.delete(b.dataset.id!);
  renderCart();
});
$$('.mrow .add').forEach(b => b.addEventListener('click', () => {
  const r = b.closest<HTMLElement>('.mrow')!, id = r.dataset.id!;
  const it = cart.get(id) || { name: r.dataset.name!, price: +r.dataset.price!, q: 0 };
  it.q = Math.min(20, it.q + 1); cart.set(id, it); renderCart();
  b.classList.remove('pop'); void (b as HTMLElement).offsetWidth; b.classList.add('pop');
  say(`${r.dataset.name} — в заказе. Итого ${$('#total').textContent}`);
}));
const setMode = () => orderCard.classList.toggle('is-delivery', mDel.checked);
$$('input[name="mode"]').forEach(i => i.addEventListener('change', setMode));
const orderPhone = $<HTMLInputElement>('#orderPhone'), orderAddr = $<HTMLInputElement>('#orderAddr');
orderPhone.addEventListener('input', () => { orderPhone.value = maskPhone(orderPhone.value); });
const orderBtn = $<HTMLButtonElement>('#orderBtn');
orderBtn.addEventListener('click', async () => {
  if (!cart.size) { say('Сначала добавьте напитки из меню'); return; }
  const delivery = mDel.checked;
  const badPhone = orderPhone.value.replace(/\D/g, '').length < 11, badAddr = delivery && orderAddr.value.trim().length < 5;
  orderPhone.classList.toggle('err', badPhone); orderAddr.classList.toggle('err', badAddr);
  if (badPhone || badAddr) { say(badAddr ? 'Укажите адрес доставки' : 'Укажите телефон'); return; }
  if (!$<HTMLInputElement>('#orderConsent').checked) { say('Нужно согласие на обработку данных'); return; }
  orderBtn.disabled = true;
  try {
    const c = await api<Checkout>('/orders', { body: {
      mode: delivery ? 'delivery' : 'pickup', phone: orderPhone.value, address: delivery ? orderAddr.value : '',
      items: [...cart.entries()].map(([id, it]) => ({ menu_item_id: +id, qty: it.q })),
    } });
    pay(c);
  } catch (e) {
    showFail(e, { phone: orderPhone, address: orderAddr });
    orderBtn.disabled = false;
  }
});
$('#orderAgain').addEventListener('click', () => { orderCard.classList.remove('is-sent'); });
renderCart();

// ---------- бронь столика ----------
const form = $<HTMLFormElement>('#bookForm');
const el = (n: string) => form.elements.namedItem(n) as HTMLInputElement;
const dateIn = el('date');
const today = new Date(); today.setMinutes(today.getMinutes() - today.getTimezoneOffset());
dateIn.min = today.toISOString().slice(0, 10); dateIn.value = dateIn.min;
const max = new Date(today); max.setDate(max.getDate() + 60); dateIn.max = max.toISOString().slice(0, 10);
const out = $<HTMLOutputElement>('#guests output');
$$('#guests button').forEach(b => b.addEventListener('click', () => { out.value = out.textContent = String(Math.min(10, Math.max(1, +(out.textContent || 1) + +b.dataset.d!))); }));
const phone = el('phone');
phone.addEventListener('input', () => { phone.value = maskPhone(phone.value); });
const bookBtn = $<HTMLButtonElement>('button[type="submit"]', form);
form.addEventListener('submit', async e => {
  e.preventDefault();
  let ok = true;
  [el('name'), phone].forEach(i => { const bad = i === phone ? i.value.replace(/\D/g, '').length < 11 : !i.value.trim(); i.classList.toggle('err', bad); if (bad) ok = false; });
  if (!ok) { say('Заполните имя и телефон'); return; }
  if (!el('consent').checked) { say('Нужно согласие на обработку данных'); return; }
  bookBtn.disabled = true;
  try {
    const c = await api<Checkout>('/bookings', { body: {
      name: el('name').value, phone: phone.value, date: dateIn.value, time: el('time').value,
      guests: +(out.textContent || 2), with_dog: $<HTMLInputElement>('#dogY').checked,
    } });
    pay(c);
  } catch (err) {
    showFail(err, { name: el('name'), phone, date: dateIn, time: el('time') });
    bookBtn.disabled = false;
  }
});
$('#bookAgain').addEventListener('click', () => { form.reset(); dateIn.value = dateIn.min; out.textContent = '2'; $('#bookCard').classList.remove('is-sent'); bookBtn.disabled = false; });

// ---------- запись на мероприятие ----------
const m = $('#evModal'), f = $<HTMLFormElement>('#evForm'), c = $<HTMLInputElement>('#evContact');
const evSubmit = $<HTMLButtonElement>('#evSubmit');
const people = $<HTMLOutputElement>('#evPeople output');
let current: HTMLElement | null = null;
const arrow = '<svg><use href="#arrow"/></svg>';
function submitLabel() {
  if (!current) return;
  const price = +(current.dataset.price || 0), n = +(people.textContent || 1);
  evSubmit.innerHTML = (price > 0 ? `Записаться · ${rub(price * n)}` : 'Записаться') + arrow;
}
function openEvent(card: HTMLElement) {
  current = card;
  $('#evTitle').textContent = card.dataset.ev || ''; $('#evWhen').textContent = card.dataset.when || '';
  $<HTMLImageElement>('#evImg').src = card.dataset.img || '';
  $('#evDogFld').style.visibility = card.dataset.dogs === '1' ? '' : 'hidden';
  people.textContent = '1'; submitLabel();
  m.classList.remove('is-sent'); m.classList.add('open'); document.body.style.overflow = 'hidden';
  setTimeout(() => (f.elements.namedItem('name') as HTMLInputElement).focus(), 300);
}
const closeEvent = () => { m.classList.remove('open'); document.body.style.overflow = ''; };
$$('.js-signup').forEach(b => b.addEventListener('click', () => openEvent(b.closest<HTMLElement>('.ev')!)));
$$('[data-close]', m).forEach(b => b.addEventListener('click', closeEvent));
m.addEventListener('click', e => { if (e.target === m) closeEvent(); });
addEventListener('keydown', e => { if (e.key === 'Escape') closeEvent(); });
const viaT = $<HTMLInputElement>('#viaT');
const setVia = () => { const tg = viaT.checked; c.value = ''; c.type = tg ? 'text' : 'tel'; c.inputMode = tg ? 'text' : 'tel'; c.placeholder = tg ? '@ваш_ник' : '+7 (900) 000-00-00'; c.classList.remove('err'); };
$$('input[name="via"]').forEach(i => i.addEventListener('change', setVia));
c.addEventListener('input', () => {
  if (viaT.checked) { if (c.value && !c.value.startsWith('@')) c.value = '@' + c.value.replace(/@/g, ''); return; }
  c.value = maskPhone(c.value);
});
$$('#evPeople button').forEach(b => b.addEventListener('click', () => {
  const left = Math.max(1, Math.min(6, +(current?.dataset.left || 6)));
  people.value = people.textContent = String(Math.min(left, Math.max(1, +(people.textContent || 1) + +b.dataset.d!)));
  submitLabel();
}));
f.addEventListener('submit', async e => {
  e.preventDefault();
  if (!current) return;
  const nameIn = f.elements.namedItem('name') as HTMLInputElement;
  const nameBad = !nameIn.value.trim();
  const cBad = viaT.checked ? c.value.replace('@', '').length < 3 : c.value.replace(/\D/g, '').length < 11;
  nameIn.classList.toggle('err', nameBad); c.classList.toggle('err', cBad);
  if (nameBad || cBad) return;
  if (!(f.elements.namedItem('consent') as HTMLInputElement).checked) { say('Нужно согласие на обработку данных'); return; }
  evSubmit.disabled = true;
  try {
    const r = await api<Checkout>(`/events/${current.dataset.evId}/signups`, { body: {
      name: nameIn.value, via: viaT.checked ? 'tg' : 'phone', contact: c.value, people: +(people.textContent || 1),
      with_dog: current.dataset.dogs === '1' && $<HTMLInputElement>('#evDogY').checked,
    } });
    if (r.status === 'confirmed') { m.classList.add('is-sent'); f.reset(); setVia(); people.textContent = '1'; }
    else pay(r);
  } catch (err) {
    showFail(err, { name: nameIn, contact: c });
  } finally { evSubmit.disabled = false; }
});

// ---------- возврат с оплаты ----------
async function afterPayment() {
  const q = new URLSearchParams(location.search), id = q.get('payment');
  if (!id) return;
  history.replaceState(null, '', location.pathname + location.hash);
  let p: PaymentStatus;
  try { p = await api<PaymentStatus>('/payments/' + id); } catch { return; }
  if (p.status !== 'succeeded') { say('Оплата не прошла — можно попробовать ещё раз'); return; }
  if (p.kind === 'booking') { $('#bookCard').classList.add('is-sent'); $('#booking').scrollIntoView(); }
  if (p.kind === 'order') {
    cart.clear(); renderCart();
    $('#orderDoneTitle').textContent = `Заказ №${p.target_id} оплачен!`;
    orderCard.classList.add('is-sent'); $('#order').scrollIntoView();
  }
  if (p.kind === 'signup') {
    const card = $$<HTMLElement>('.ev').find(x => p.description.startsWith(x.dataset.ev || '\u0000'));
    if (card) { current = card; $('#evTitle').textContent = card.dataset.ev || ''; $('#evWhen').textContent = card.dataset.when || ''; $<HTMLImageElement>('#evImg').src = card.dataset.img || ''; }
    m.classList.add('open', 'is-sent'); document.body.style.overflow = 'hidden';
  }
}
afterPayment();

addEventListener('scroll', onScroll, { passive: true }); onScroll();
