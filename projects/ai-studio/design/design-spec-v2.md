# Design spec v2 — «Студия ИИ»: тёмный/светлый фон + характер

Только **отличия от v1** (`design-spec.md`). Всё, что здесь не упомянуто, — как в v1.
Направление то же — «тихий редактор», но с ритмом «бумага / чернила»: разделы чередуются светлый ↔ тёмный,
первый экран — тёмный, с живой иллюстрацией продукта (окно чата на CSS).
Новых библиотек нет. Тексты, тарифы, порядок блоков, логика — без изменений.
Затрагиваются: `app/globals.css`, `app/layout.tsx`, `app/page.tsx`, `app/cabinet/layout.tsx`, новый `components/theme-toggle.tsx`.

**Детали стиля v2 (4 штуки, и всё):**
1. Чередование фонов: тёмные «чернильные» разделы (`.surface-ink`) — первый экран и подвал.
2. Сетка из точек на фоне первого экрана (тает к краям).
3. Мягкое акцентное свечение `brand` за окном чата.
4. Окно чата на CSS в первом экране — иллюстрация продукта, сообщения появляются по очереди.
5. Плюс: мягкое появление блоков при прокрутке (только CSS, только где браузер умеет, только без reduced-motion).

Моно-подписи и волосяные линии из v1 остаются.

---

## 1. Токены и темы — `app/globals.css`

Тема задаётся атрибутом `data-theme` на `<html>`: `light` | `dark` | нет атрибута = как в системе.
Блок `:root` и `@media (prefers-color-scheme: dark)` из v1 **заменить** на этот код (блок `@theme inline` из v1 — без изменений):

```css
/* ---------- светлая (по умолчанию) ---------- */
:root {
  color-scheme: light;
  --background: #f7f6f3;
  --foreground: #121212;
  --muted: #6b6a66;
  --card: #ffffff;
  --border: #e4e2dc;
  --brand: #2f45d8;
  --brand-soft: #eceffd;
  --support: #a35a00;
  --support-soft: #fbf3e6;
  --danger: #c2341b;
  --dot: rgb(18 18 18 / 0.10);
}

/* ---------- тёмная: явно выбрана ---------- */
:root[data-theme="dark"] {
  color-scheme: dark;
  --background: #0f0f0e;
  --foreground: #efeee9;
  --muted: #9a9890;
  --card: #171716;
  --border: #2a2927;
  --brand: #8c9bff;
  --brand-soft: #1a1f3d;
  --support: #f0b35a;
  --support-soft: #2a2114;
  --danger: #ff7a62;
  --dot: rgb(239 238 233 / 0.10);
}
/* ---------- тёмная: по системе (если пользователь не выбрал светлую) ---------- */
@media (prefers-color-scheme: dark) {
  :root:not([data-theme="light"]) {
    color-scheme: dark;
    --background: #0f0f0e;
    --foreground: #efeee9;
    --muted: #9a9890;
    --card: #171716;
    --border: #2a2927;
    --brand: #8c9bff;
    --brand-soft: #1a1f3d;
    --support: #f0b35a;
    --support-soft: #2a2114;
    --danger: #ff7a62;
    --dot: rgb(239 238 233 / 0.10);
  }
}

/* ---------- «чернильный» раздел: всегда тёмный, в любой теме ----------
   Обёртка переопределяет токены — все bg-background / text-foreground / ui.button
   внутри автоматически инвертируются. В тёмной теме чуть светлее фона страницы,
   чтобы чередование было видно. */
.surface-ink {
  color-scheme: dark;
  --background: #121212;
  --foreground: #efeee9;
  --muted: #a3a198;
  --card: #1b1b1a;
  --border: #2c2b28;
  --brand: #8c9bff;
  --brand-soft: #1a1f3d;
  --danger: #ff7a62;
  --dot: rgb(239 238 233 / 0.11);
  background: var(--background);
  color: var(--foreground);
}
:root[data-theme="dark"] .surface-ink { --background: #181816; --card: #20201e; --border: #32312d; }
@media (prefers-color-scheme: dark) {
  :root:not([data-theme="light"]) .surface-ink { --background: #181816; --card: #20201e; --border: #32312d; }
}

body { background: var(--background); color: var(--foreground); }
::selection { background: var(--brand); color: var(--background); } /* было #fff — в тёмных местах нечитаемо */
:focus-visible { outline: 2px solid var(--brand); outline-offset: 2px; }

/* ---------- детали стиля ---------- */
.bg-dots {               /* сетка точек; кладётся отдельным div aria-hidden */
  background-image: radial-gradient(var(--dot) 1px, transparent 1.5px);
  background-size: 22px 22px;
  mask-image: radial-gradient(ellipse 75% 65% at 50% 40%, #000 35%, transparent 100%);
}
.glow {                  /* акцентное свечение */
  background: radial-gradient(closest-side, color-mix(in oklab, var(--brand) 32%, transparent), transparent);
  pointer-events: none;
}

@keyframes msg-in { from { opacity: 0; translate: 0 8px; } to { opacity: 1; translate: 0 0; } }
@keyframes dot-pulse { 0%, 80%, 100% { opacity: .25; } 40% { opacity: 1; } }
@keyframes reveal { from { opacity: 0; translate: 0 24px; } to { opacity: 1; translate: 0 0; } }

@media (prefers-reduced-motion: no-preference) {
  .msg-in { animation: msg-in .5s cubic-bezier(.2,.7,.2,1) both; }
  .typing-dot { animation: dot-pulse 1.2s infinite; }
  .typing-dot:nth-child(2) { animation-delay: .15s; }
  .typing-dot:nth-child(3) { animation-delay: .3s; }
  @supports (animation-timeline: view()) {
    .reveal { animation: reveal linear both; animation-timeline: view(); animation-range: entry 0% entry 35%; }
  }
}
/* В браузерах без animation-timeline (Firefox) и при reduced-motion блоки просто видны — JS не нужен. */
```
Плюс плавная смена темы (только цвета, не layout): в `body` добавить `transition: background-color .2s, color .2s;`
внутри того же `@media (prefers-reduced-motion: no-preference)`.

**Контраст (AA, обычный текст ≥ 4.5:1, посчитано):** ink `muted #a3a198` на `#121212` 7.2:1, на `#181816` 6.9:1, на `card #20201e` 6.3:1;
`foreground` на ink 16:1; `brand #8c9bff` на ink 7.4 / 7.0:1; выделение (`::selection`) — 6.6:1 и 7.5:1; светлая и тёмная темы — как в v1 (проверены).
Точки (`--dot`) и свечение — декор, к тексту не относятся.

## 2. Без мигания при загрузке — `app/layout.tsx`
В `<html>` добавить `suppressHydrationWarning`; первым ребёнком `<html>` — `<head>` с inline-скриптом
(выполняется до отрисовки, поэтому тема не «моргает»):
```tsx
<html lang="ru" suppressHydrationWarning className={`${onest.variable} ${mono.variable} h-full antialiased`}>
  <head>
    <script
      dangerouslySetInnerHTML={{
        __html: `try{var t=localStorage.getItem("theme");if(t==="light"||t==="dark")document.documentElement.dataset.theme=t}catch(e){}`,
      }}
    />
  </head>
  <body className="min-h-full flex flex-col font-sans">{children}</body>
</html>
```
Если «как в системе» — атрибута нет, работает `prefers-color-scheme` из CSS. Если в проекте строгий CSP с nonce — скрипт получает тот же nonce.

## 3. Переключатель темы — `components/theme-toggle.tsx` (новый, клиентский)
Вид: «капсула» из трёх иконок 16px — солнце / луна / монитор, 28×28 каждая.
Активный пункт подсвечивается **через CSS по атрибуту `<html>`** — так нет рассинхрона при гидрации.
```tsx
"use client";
import { useSyncExternalStore } from "react";

type Mode = "light" | "dark" | "system";
const KEY = "theme";
const EVT = "themechange";

function read(): Mode {
  try { const t = localStorage.getItem(KEY); return t === "light" || t === "dark" ? t : "system"; } catch { return "system"; }
}
function setMode(m: Mode) {
  const el = document.documentElement;
  try {
    if (m === "system") { delete el.dataset.theme; localStorage.removeItem(KEY); }
    else { el.dataset.theme = m; localStorage.setItem(KEY, m); }
  } catch {}
  window.dispatchEvent(new Event(EVT));
}
function subscribe(cb: () => void) {
  window.addEventListener(EVT, cb); window.addEventListener("storage", cb);
  return () => { window.removeEventListener(EVT, cb); window.removeEventListener("storage", cb); };
}

const items: { value: Mode; label: string; icon: React.ReactNode }[] = [
  { value: "light", label: "Светлая тема", icon: <path d="M12 4V2M12 22v-2M4 12H2M22 12h-2M5.6 5.6 4.2 4.2M19.8 19.8l-1.4-1.4M5.6 18.4l-1.4 1.4M19.8 4.2l-1.4 1.4M12 16a4 4 0 1 0 0-8 4 4 0 0 0 0 8Z" /> },
  { value: "dark", label: "Тёмная тема", icon: <path d="M20 14.5A8 8 0 0 1 9.5 4 8 8 0 1 0 20 14.5Z" /> },
  { value: "system", label: "Как в системе", icon: <path d="M4 5h16v11H4zM9 20h6M12 16v4" /> },
];

export function ThemeToggle({ className = "" }: { className?: string }) {
  const mode = useSyncExternalStore(subscribe, read, () => "system" as Mode);
  return (
    <div role="group" aria-label="Тема" className={`theme-toggle inline-flex shrink-0 items-center gap-0.5 rounded-full border border-border p-0.5 ${className}`}>
      {items.map((it) => (
        <button
          key={it.value}
          type="button"
          data-tt={it.value}
          aria-label={it.label}
          title={it.label}
          aria-pressed={mode === it.value}
          onClick={() => setMode(it.value)}
          className="inline-flex size-7 items-center justify-center rounded-full text-muted transition-colors hover:text-foreground"
        >
          <svg viewBox="0 0 24 24" className="size-4" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round" aria-hidden>{it.icon}</svg>
        </button>
      ))}
    </div>
  );
}
```
CSS активного пункта — в `globals.css`:
```css
:root[data-theme="light"] .theme-toggle [data-tt="light"],
:root[data-theme="dark"]  .theme-toggle [data-tt="dark"],
:root:not([data-theme])   .theme-toggle [data-tt="system"] {
  background: var(--card); color: var(--foreground); box-shadow: inset 0 0 0 1px var(--border);
}
```
Состояния: hover — `text-foreground`; focus-visible — глобальное кольцо `brand`; активный — заливка `card` + тонкая рамка.
Зона нажатия 28px + промежутки — на телефоне допустимо в шапке; если QA сочтёт мелким — `size-8` (капсула станет 100px).

**Где стоит:**
- Главная, шапка: справа, перед «Войти»: `<div className="flex items-center gap-3 sm:gap-5">… «Тарифы» … <ThemeToggle /> … «Войти»</div>`.
- Кабинет (`app/cabinet/layout.tsx`), шапка: справа, перед email, `ml-auto` переносится на обёртку `flex items-center gap-4`
  (`<ThemeToggle />`, email, «Выйти»). На 375 навигация скроллится, а правая группа — `shrink-0`, не скроллится.
- Админка — по желанию тем же компонентом (не обязательно в этой задаче); тема общая через localStorage.

## 4. Главная (`app/page.tsx`) — что меняется
Ритм фонов сверху вниз: **шапка (тема) → первый экран (ink) → «Как это работает» (тема) → «Тарифы» (тема) → подвал (ink)**.
Тёмное обрамляет страницу сверху и снизу, контент с ценами — на «бумаге», где лучше читается.

### 4.1 Шапка
Как v1 + `ThemeToggle` (п.3). `border-b border-border` остаётся — на стыке с тёмным героем это чёткий край.

### 4.2 Первый экран — тёмный, с иллюстрацией
```tsx
<section className="surface-ink relative overflow-hidden">
  <div aria-hidden className="bg-dots absolute inset-0" />
  <div className={`${container} relative grid items-center gap-14 pb-20 pt-16 sm:pb-28 sm:pt-24 lg:grid-cols-[1.15fr_0.85fr] lg:gap-16`}>
    <div>{/* eyebrow, H1, подзаголовок, кнопки — как v1, без изменений классов */}</div>
    <div className="relative">
      <div aria-hidden className="glow absolute -inset-16 sm:-inset-24" />
      <HeroChat />
    </div>
  </div>
</section>
```
- Классы текста v1 работают как есть: `text-muted`, `ui.button` (станет светлой кнопкой с тёмным текстом), `ui.buttonGhost` (светлая рамка).
- H1: на lg колонка ~640px — `max-w-[14ch]` из v1 оставить, 76px помещается в 3 строки.
- **375 / 768:** одна колонка, окно чата **после кнопок** (`mt` за счёт `gap-14`); заголовок и главная кнопка остаются в первом экране телефона.
  На 768 окно чата `max-w-md` (добавить `sm:max-w-md lg:max-w-none` на обёртку `relative`). На 1440 — справа от текста.

**`HeroChat`** — серверный компонент в том же файле, `aria-hidden` целиком (декор; продукт описан текстом слева).
Реплики — **примерные, для иллюстрации** (пометка в коде `{/* примерные реплики, иллюстрация */}`; согласовать с заказчиком):
```tsx
function HeroChat() {
  return (
    <div aria-hidden className="relative rounded-2xl border border-border bg-card/90 shadow-[0_30px_80px_-30px_rgb(0_0_0/0.6)] backdrop-blur">
      {/* «окно»: три точки + моно-подпись */}
      <div className="flex items-center gap-1.5 border-b border-border px-4 py-3">
        <span className="size-2.5 rounded-full bg-border" /><span className="size-2.5 rounded-full bg-border" /><span className="size-2.5 rounded-full bg-border" />
        <span className="ml-3 font-mono text-[11px] uppercase tracking-[0.08em] text-muted">чат · ИИ-агент</span>
        <span className="ml-auto inline-flex items-center gap-1.5 font-mono text-[11px] uppercase tracking-[0.08em] text-muted">
          <span className="size-1.5 rounded-full bg-brand" />онлайн
        </span>
      </div>
      {/* примерные реплики, иллюстрация */}
      <div className="space-y-3 p-4 text-sm leading-relaxed sm:p-5">
        <p className="msg-in ml-auto max-w-[85%] rounded-2xl rounded-br-md bg-foreground px-4 py-2.5 text-background" style={{ animationDelay: ".2s" }}>
          Нужен сайт для кофейни: меню, адрес и запись на мероприятия.
        </p>
        <p className="msg-in max-w-[85%] rounded-2xl rounded-bl-md border border-border bg-background px-4 py-2.5" style={{ animationDelay: ".9s" }}>
          Понял. Предлагаю четыре раздела: главная, меню, события, контакты. Начну с эскиза главной.
        </p>
        <div className="msg-in max-w-[85%] rounded-2xl rounded-bl-md border border-border bg-background p-3" style={{ animationDelay: "1.6s" }}>
          <div className="font-mono text-[11px] uppercase tracking-[0.08em] text-muted">Эскиз · главная</div>
          <div className="mt-2 grid grid-cols-3 gap-1.5">
            <span className="col-span-3 h-10 rounded-md bg-brand-soft" />
            <span className="h-6 rounded-md bg-border" /><span className="h-6 rounded-md bg-border" /><span className="h-6 rounded-md bg-border" />
          </div>
        </div>
        <div className="msg-in inline-flex gap-1 rounded-full border border-border bg-background px-3 py-2" style={{ animationDelay: "2.2s" }}>
          <span className="typing-dot size-1.5 rounded-full bg-brand" /><span className="typing-dot size-1.5 rounded-full bg-brand" /><span className="typing-dot size-1.5 rounded-full bg-brand" />
        </div>
      </div>
    </div>
  );
}
```
Пузыри повторяют стиль кабинета из v1 (свои — `bg-foreground`, агент — с рамкой), чтобы главная «обещала» тот же интерфейс.
Анимация разовая при загрузке (~2.5 с), без цикла. При reduced-motion всё видно сразу.

### 4.3 «Как это работает» и «Тарифы» — фон темы
- `border-t` у `#how` убрать (стык с ink — уже граница); у `#pricing` `border-t border-border` оставить.
- Мягкое появление: класс `reveal` на заголовочный блок раздела (eyebrow + H2) и на каждый шаг / каждую карточку тарифа.
  Больше никуда (не на героя, не на подвал, не на кнопки отдельно).
- Шаги: номер `0{i+1}` из v1 крупнее — `font-mono text-sm text-brand` (было `text-xs`), чтобы перекликался с моно-подписями окна чата.

### 4.4 Подвал — тёмный
`<footer className="surface-ink">` вместо `border-t border-border`; внутри как v1 (`text-muted`, `ui.link` — токены подхватятся).
Над строкой копирайта — тонкая линия не нужна; отступ `py-10` (было `py-8`), чтобы тёмная полоса не выглядела обрезанной.

## 5. Кабинет
Только переключатель темы в шапке (п.3). Фоны кабинета — по теме, **ink-разделов в кабинете нет** (рабочий интерфейс, без декора).
Вход (`/login`) — без изменений, тема подхватывается автоматически.

## 6. Ширины — что отличается от v1
| | 375 | 768 | 1440 |
|---|---|---|---|
| Первый экран | ink; текст → кнопки → окно чата | ink; окно чата под текстом, `max-w-md` | ink; две колонки 1.15 : 0.85, чат справа |
| Свечение | `-inset-16` | `-inset-24` | `-inset-24` |
| Шапка главной | логотип · тема · «Войти» | + «Тарифы» | как 768 |
| Шапка кабинета | навигация скроллится; тема + «Выйти» закреплены справа | email скрыт | всё в строку |
| Подвал | ink | ink | ink |

## 7. Проверка перед сдачей (фронтенд + QA)
- [ ] Три режима переключателя × 375/768/1440: главная, кабинет (чат), вход. Скриншоты в обеих темах.
- [ ] Перезагрузка с выбранной «тёмной» при светлой системе — нет вспышки светлого фона (и наоборот).
- [ ] `prefers-reduced-motion: reduce` — ни одной анимации, весь контент виден.
- [ ] Firefox — блоки `reveal` видимы сразу (нет `animation-timeline`).
- [ ] Контраст: текст в ink-разделах и в обеих темах ≥ 4.5:1 (значения в п.1).
- [ ] Первый экран 375: H1 и «Начать проект» видны без прокрутки.

## 8. Новое — на одобрение заказчику
1. Тёмный первый экран и тёмный подвал (светлые — «Как это работает» и «Тарифы»).
2. Сетка из точек на фоне первого экрана.
3. Синее свечение за окном чата.
4. Окно чата в первом экране с примерными репликами (про кофейню) — тексты реплик можно заменить.
5. Мягкое появление блоков при прокрутке.
6. Переключатель темы (светлая / тёмная / как в системе) в шапке главной и кабинета.

## 9. Спорные места
- В тёмной теме ink-разделы лишь чуть светлее фона (#181816 против #0f0f0e) — чередование тише, чем в светлой. Это сознательно:
  инвертировать их в светлые в тёмной теме — резко и «прыгает». Если заказчик хочет контрастнее — поднять до `#1e1e1c`.
- Пример реплик (кофейня) — нейтральный; если у заказчика есть типичный клиент, лучше взять его пример.
