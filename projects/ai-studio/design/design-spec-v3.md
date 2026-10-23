# Design spec v3 — Lumen: бренд, искра-мотив, живой чат, свои элементы

Только **отличия от v2** (`design-spec-v2.md`). Всё, что не упомянуто, — как в v2 (темы, ink-разделы, токены, переключатель темы).
Тексты сайта и тарифы не меняются; меняется только название (`SITE_NAME = "Lumen"`) и реплики чата в первом экране.
Новых библиотек нет. Затрагиваются: `app/globals.css`, `app/layout.tsx`, `app/page.tsx`, `app/icon.svg` (новый, вместо `favicon.ico`),
новые `components/brand.tsx`, `components/hero-chat.tsx`, `components/forms.tsx`, `components/confirm.tsx`;
правки форм: `login/login-form.tsx`, `cabinet/chat.tsx`, `cabinet/profile/forms.tsx`, `admin/(panel)/plans/page.tsx`,
`admin/(panel)/users/[id]/page.tsx`, `admin/(panel)/finance/refund-button.tsx`, `admin/2fa/totp-form.tsx`, `admin/(panel)/agent/page.tsx`.

**Идея v3.** Lumen = свет. Искра ✶ из логотипа — единственный «фирменный жест»: она отмечает, где что-то происходит
(агент печатает, активный шаг, выбранный пункт, главный тариф). Искра тёплая (`--spark`), всё остальное — как в v2:
графитовый словознак, синий `brand` для ссылок/фокуса. Искра не больше чем в 1–2 местах на экран — тогда она «светит».

**Новые акценты стиля (3):**
1. **Искра-мотив** — маркеры, номера, индикатор «печатает», курсор, разделители, мерцание у логотипа.
2. **Гигантский словознак в подвале** — «lumen» во всю ширину, обрезан снизу, с искрой, которая мерцает.
3. **Световой край** — тонкая градиентная рамка (brand → spark) у окна чата и у выделенного тарифа + тёплый «блик» в свечении героя.

---

## 1. Токены — `app/globals.css` (добавить к v2)

```css
:root                              { --spark: #c2410c; --spark-soft: #fbeee6; } /* 4.8:1 на #f7f6f3 — годится и для текста */
:root[data-theme="dark"]           { --spark: #ffb454; --spark-soft: #2a1f12; } /* 10.9:1 на #0f0f0e */
@media (prefers-color-scheme: dark) { :root:not([data-theme="light"]) { --spark: #ffb454; --spark-soft: #2a1f12; } }
.surface-ink                       { --spark: #ffb454; --spark-soft: #2a1f12; } /* 10.1–10.6:1 */
```
В `@theme inline` добавить `--color-spark: var(--spark); --color-spark-soft: var(--spark-soft);` (классы `text-spark`, `bg-spark` и т.д.).
Контраст посчитан: все пары ≥ 4.5:1; искра в графике ≥ 3:1 везде.

## 2. Логотип в коде — `components/brand.tsx`

Словознак нарисован **путями-штрихами** (моноширинная линия со скруглёнными концами — как на картинке: жирный строчный
скруглённый гротеск, плотный трекинг). Шрифт не нужен, рендер одинаков везде. Цвет — `currentColor`, поэтому
светлая/тёмная версия получается сама (графит на бумаге, светлый на ink). Искра — отдельный путь, цвет `--spark`.

```tsx
// components/brand.tsx — серверный, без JS
export const SPARK_PATH =
  "M306 10 L307 23.6 L313.1 18.9 L308.4 25 L322 26 L308.4 27 L313.1 33.1 L307 28.4 L306 42 L305 28.4 L298.9 33.1 L303.6 27 L290 26 L303.6 25 L298.9 18.9 L305 23.6Z";

export function LumenLogo({ className = "", title = "Lumen", twinkle = true }: { className?: string; title?: string; twinkle?: boolean }) {
  return (
    <svg viewBox="0 0 326 92" className={className} role="img" aria-label={title}>
      <g fill="none" stroke="currentColor" strokeWidth="17" strokeLinecap="round" strokeLinejoin="round">
        <path d="M10 10V82" />                                             {/* l */}
        <path d="M34 44V64a18 18 0 0 0 36 0V44M70 44V82" />               {/* u */}
        <path d="M94 44V82M94 62a18 18 0 0 1 36 0V82M130 62a18 18 0 0 1 36 0V82" /> {/* m */}
        <path d="M191 63H228A19 19 0 1 0 222.4 76.4" />                    {/* e */}
        <path d="M252 44V82M252 62a18 18 0 0 1 36 0V82" />                 {/* n */}
      </g>
      <path d={SPARK_PATH} className={`fill-spark ${twinkle ? "spark-twinkle" : ""}`} style={{ transformOrigin: "306px 26px" }} />
    </svg>
  );
}

// Одиночная искра — маркер, иконка, индикатор. viewBox 32×32, центр 16/16.
export function Spark({ className = "size-3", ...rest }: React.SVGProps<SVGSVGElement>) {
  return (
    <svg viewBox="0 0 32 32" aria-hidden className={`shrink-0 fill-current ${className}`} {...rest}>
      <path d="M16 3L16.8 14L21.7 10.3L18 15.2L29 16L18 16.8L21.7 21.7L16.8 18L16 29L15.2 18L10.3 21.7L14 16.8L3 16L14 15.2L10.3 10.3L15.2 14Z" />
    </svg>
  );
}
```
- Сетка словознака: высота строчных 38 (центры штрихов 44→82), выносной «l» до 10, толщина 17, просвет между штрихами ~7 —
  как на картинке. Искра касается плеча «n» справа сверху, как в оригинале. **Сверить с PNG наложением** (фронтенд/QA:
  скриншот 600px поверх `assets/lumen-logo.png` при 50% прозрачности; допуск — «на глаз то же»; если «e» или «m» шире —
  менять только `a18` радиусы и x-координаты, толщину не трогать).
- Размеры: шапка — `h-6 w-auto` (24px, ~85px ширина); вход/кабинет — `h-5`; подвал — гигант (п.6.6). Минимум — 16px высоты.
- Доступность: в ссылке-логотипе `aria-label="Lumen — на главную"` на `<Link>`, у SVG тогда `aria-hidden`.

**Фавикон — `app/icon.svg`** (Next подхватит сам; `favicon.ico` удалить):
```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32">
  <style>.b{fill:#121212}.s{fill:#ffb454}@media (prefers-color-scheme:dark){.b{fill:#efeee9}.s{fill:#c2410c}}</style>
  <rect class="b" width="32" height="32" rx="8"/>
  <path class="s" transform="translate(16 16) scale(1.15) translate(-16 -16)" d="M16 3L16.8 14L21.7 10.3L18 15.2L29 16L18 16.8L21.7 21.7L16.8 18L16 29L15.2 18L10.3 21.7L14 16.8L3 16L14 15.2L10.3 10.3L15.2 14Z"/>
</svg>
```
`apple-icon.png` 180×180 — тот же рисунок (графитовый квадрат, тёплая искра), экспорт из SVG.
`metadata.title`: шаблон `` `${SITE_NAME} — …` `` остаётся, подставится «Lumen».

## 3. Анимации-искры — `app/globals.css`

```css
@keyframes spark-twinkle { 0%, 86%, 100% { scale: 1; rotate: 0deg; opacity: 1; } 90% { scale: 1.25; rotate: 22deg; opacity: .85; } 94% { scale: .9; rotate: 45deg; } }
@keyframes spark-pulse   { 0%, 70%, 100% { opacity: .3; scale: .7; } 35% { opacity: 1; scale: 1; } }
@keyframes caret-blink   { 0%, 45% { opacity: 1; } 50%, 95% { opacity: 0; } }
@keyframes spin          { to { rotate: 360deg; } }
@keyframes chat-in       { from { opacity: 0; translate: 0 10px; scale: .98; } to { opacity: 1; translate: 0 0; scale: 1; } }

@media (prefers-reduced-motion: no-preference) {
  .spark-twinkle { animation: spark-twinkle 6s ease-in-out infinite; transform-box: fill-box; }   /* у логотипа: раз в 6 с короткий «блик» */
  a:hover .spark-twinkle { animation-duration: 1.2s; }                                              /* навели на лого — мерцает чаще */
  .spark-typing > * { animation: spark-pulse 1.1s ease-in-out infinite; }
  .spark-typing > *:nth-child(2) { animation-delay: .15s; }
  .spark-typing > *:nth-child(3) { animation-delay: .3s; }
  .caret { animation: caret-blink 1.1s steps(1) infinite; }
  .spin { animation: spin .9s linear infinite; }
  .chat-in { animation: chat-in .45s cubic-bezier(.2,.7,.2,1) both; }
}
```
`.msg-in` / `.typing-dot` из v2 больше не используются — удалить.

## 4. Свои элементы вместо браузерных (правило: ни одного стандартного вида)

Общий принцип: нативный элемент, если он нужен для формы и клавиатуры, остаётся в DOM **визуально скрытым**
(`peer sr-only`) — фокус, Tab, Space, отправка формы и экранные дикторы работают «из коробки»; видно только наше.
Всплывашки браузера, системные `confirm/alert`, `title`-подсказки, нативные спиннеры и полосы — убраны полностью.

### 4.1 Глобально — `app/globals.css`
```css
/* полосы прокрутки */
* { scrollbar-width: thin; scrollbar-color: color-mix(in oklab, var(--foreground) 22%, transparent) transparent; }
::-webkit-scrollbar { width: 10px; height: 10px; }
::-webkit-scrollbar-track { background: transparent; }
::-webkit-scrollbar-thumb { background: color-mix(in oklab, var(--foreground) 22%, transparent); border-radius: 99px; border: 3px solid transparent; background-clip: padding-box; }
::-webkit-scrollbar-thumb:hover { background-color: color-mix(in oklab, var(--foreground) 40%, transparent); }
::-webkit-scrollbar-corner { background: transparent; }
.scroll-none { scrollbar-width: none; } .scroll-none::-webkit-scrollbar { display: none; } /* горизонтальная навигация кабинета на 375 */

/* поля */
input, textarea, select { caret-color: var(--spark); }
input[type="number"] { -moz-appearance: textfield; appearance: textfield; }
input::-webkit-outer-spin-button, input::-webkit-inner-spin-button { -webkit-appearance: none; margin: 0; }
input[type="search"]::-webkit-search-cancel-button { -webkit-appearance: none; }
input:-webkit-autofill, input:-webkit-autofill:hover, input:-webkit-autofill:focus {
  -webkit-text-fill-color: var(--foreground); caret-color: var(--spark);
  box-shadow: 0 0 0 1000px var(--card) inset; transition: background-color 9999s;
}
textarea { resize: none; field-sizing: content; }               /* без уголка-ресайза; высота по тексту там, где браузер умеет */
html { -webkit-tap-highlight-color: transparent; }
summary { list-style: none; } summary::-webkit-details-marker { display: none; }

/* своя подсказка вместо title */
[data-tip] { position: relative; }
[data-tip]::after {
  content: attr(data-tip); position: absolute; bottom: calc(100% + 8px); left: 50%; translate: -50% 4px;
  white-space: nowrap; padding: 6px 10px; border-radius: 8px; font: 500 12px/1.2 var(--font-onest);
  background: var(--foreground); color: var(--background); opacity: 0; pointer-events: none;
  transition: opacity .15s, translate .15s; z-index: 30;
}
[data-tip]:hover::after, [data-tip]:focus-visible::after { opacity: 1; translate: -50% 0; }
@media (hover: none) { [data-tip]::after { display: none; } }  /* на телефоне подсказок нет — у кнопок есть aria-label */
```
Все `title="…"` в проекте → `data-tip="…"` + `aria-label` (переключатель темы, скрепка в чате и т.п.).

### 4.2 Компоненты — `components/forms.tsx` (клиентский)

**Checkbox** — квадрат 20px, радиус 6, внутри галочка-штрих; выбранный — заливка `foreground`, галочка `background`;
при отметке галочка «прорисовывается» (stroke-dashoffset, 180 мс).
```tsx
export function Checkbox({ children, error, id, ...input }: React.InputHTMLAttributes<HTMLInputElement> & { children: React.ReactNode; error?: string }) {
  const errId = error ? `${id}-err` : undefined;
  return (
    <div>
      <label htmlFor={id} className="group flex cursor-pointer gap-3 text-sm text-muted">
        <input id={id} type="checkbox" className="peer sr-only" aria-invalid={!!error} aria-describedby={errId} {...input} />
        <span aria-hidden className="mt-px grid size-5 shrink-0 place-items-center rounded-md border-[1.5px] border-border bg-card transition-[background,border-color,box-shadow]
          group-hover:border-foreground/40 peer-checked:border-foreground peer-checked:bg-foreground
          peer-focus-visible:ring-4 peer-focus-visible:ring-brand/20 peer-focus-visible:border-brand
          peer-aria-[invalid=true]:border-danger peer-active:scale-95
          [&_path]:[stroke-dasharray:16] [&_path]:[stroke-dashoffset:16] peer-checked:[&_path]:[stroke-dashoffset:0]">
          <svg viewBox="0 0 16 16" className="size-3.5 text-background"><path d="M3.5 8.5 6.5 11.5 12.5 4.5" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="transition-[stroke-dashoffset] duration-200" /></svg>
        </span>
        <span className="leading-relaxed">{children}</span>
      </label>
      <FieldError id={errId} message={error} />
    </div>
  );
}
```
Состояния: обычное / hover (рамка темнее) / focus-visible (синее кольцо) / нажатие (scale .95) / отмечено / ошибка (красная рамка + сообщение) / disabled (opacity .4, cursor-not-allowed).

**Radio** — тот же каркас, `type="radio"`, круг 20px; выбранный — рамка `foreground` 1.5px и **искра 10px** `text-spark` в центре
(появляется scale 0 → 1, 150 мс). Группа — `<fieldset>` + `<legend className={ui.label}>`. Сейчас в проекте радио нет — компонент
на будущее (тарифы в кабинете, если станут выбором).

**Switch** (для «Активен» в админке тарифов вместо чекбокса) — дорожка 36×20, ручка 16; вкл — дорожка `foreground`, ручка `background`
с маленькой искрой `text-spark`; `input type="checkbox" role="switch" className="peer sr-only"`, ручка `peer-checked:translate-x-4`.

**Select** — свой список (`Select`), вместо всех `<select>` (язык, часовой пояс, валюта, этап проекта):
```tsx
export function Select({ name, options, defaultValue, label }: { name: string; options: { value: string; label: string }[]; defaultValue?: string; label: string }) {
  const [open, setOpen] = useState(false);
  const [value, setValue] = useState(defaultValue ?? options[0]?.value);
  const [active, setActive] = useState(() => Math.max(0, options.findIndex((o) => o.value === value)));
  const btn = useRef<HTMLButtonElement>(null); const list = useRef<HTMLUListElement>(null);
  const id = useId(); const current = options.find((o) => o.value === value);
  useEffect(() => { if (!open) return; const close = (e: MouseEvent) => { if (!btn.current?.parentElement?.contains(e.target as Node)) setOpen(false); };
    document.addEventListener("mousedown", close); list.current?.focus(); return () => document.removeEventListener("mousedown", close); }, [open]);
  const pick = (i: number) => { setValue(options[i].value); setOpen(false); btn.current?.focus(); };
  return (
    <div className="relative">
      <span id={`${id}-l`} className={ui.label}>{label}</span>
      <input type="hidden" name={name} value={value} />
      <button ref={btn} type="button" aria-haspopup="listbox" aria-expanded={open} aria-labelledby={`${id}-l ${id}-v`}
        onClick={() => setOpen((o) => !o)}
        onKeyDown={(e) => { if (["ArrowDown", "ArrowUp", "Enter", " "].includes(e.key)) { e.preventDefault(); setOpen(true); } }}
        className={`${ui.input} flex items-center justify-between gap-3 text-left aria-expanded:border-brand aria-expanded:ring-4 aria-expanded:ring-brand/10`}>
        <span id={`${id}-v`} className="truncate">{current?.label}</span>
        <svg viewBox="0 0 16 16" aria-hidden className={`size-4 shrink-0 text-muted transition-transform ${open ? "rotate-180" : ""}`}><path d="m4 6 4 4 4-4" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round" /></svg>
      </button>
      {open && (
        <ul ref={list} role="listbox" tabIndex={-1} aria-labelledby={`${id}-l`} aria-activedescendant={`${id}-${active}`}
          onKeyDown={(e) => {
            if (e.key === "ArrowDown") { e.preventDefault(); setActive((a) => Math.min(options.length - 1, a + 1)); }
            if (e.key === "ArrowUp") { e.preventDefault(); setActive((a) => Math.max(0, a - 1)); }
            if (e.key === "Home") setActive(0); if (e.key === "End") setActive(options.length - 1);
            if (e.key === "Enter" || e.key === " ") { e.preventDefault(); pick(active); }
            if (e.key === "Escape" || e.key === "Tab") { setOpen(false); btn.current?.focus(); }
          }}
          className="chat-in absolute inset-x-0 top-full z-20 mt-2 max-h-64 overflow-auto rounded-xl border border-border bg-card p-1.5 shadow-[0_20px_50px_-20px_rgb(0_0_0/0.35)] outline-none">
          {options.map((o, i) => (
            <li key={o.value} id={`${id}-${i}`} role="option" aria-selected={o.value === value}
              onMouseEnter={() => setActive(i)} onMouseDown={(e) => { e.preventDefault(); pick(i); }}
              className={`flex cursor-pointer items-center justify-between gap-3 rounded-lg px-3 py-2.5 text-sm ${i === active ? "bg-background" : ""} aria-selected:font-medium`}>
              {o.label}
              {o.value === value && <Spark className="size-3 text-spark" />}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
```
Выбранный пункт отмечен искрой (не галочкой) — это и есть мотив. Активный (клавиатура/наведение) — фон `background`.
Список часовых поясов длинный — у списка своя тонкая полоса (п.4.1), активный пункт прокручивается в видимую зону (`scrollIntoView({block:"nearest"})` в эффекте по `active`).
Поиск по первой букве — желательно (typeahead по `e.key.length === 1`), не обязательно.

**Поле файла** (чат кабинета) — `input type="file"` остаётся скрытым (`sr-only`, не `hidden`, чтобы работал `label`), видно только наше:
- Кнопка-скрепка: SVG вместо эмодзи 📎 (`<path d="M15.5 6.5 8.4 13.6a2 2 0 0 0 2.8 2.8l7.4-7.4a4 4 0 0 0-5.6-5.6L5.6 10.8a6 6 0 0 0 8.5 8.5l6.4-6.4" />`, 20px, stroke 1.75),
  `data-tip="Прикрепить фото, видео или PDF"`, `aria-label` тот же. Загрузка — вместо «…» искра `spin size-4 text-spark`.
- Перетаскивание: при `dragover` над формой — рамка `border-dashed border-spark` и полупрозрачный слой поверх поля с искрой и текстом
  «Отпустите, чтобы прикрепить» (новая служебная подпись — на одобрение; можно без текста, только искра).
- Чипы файлов: `rounded-full border border-border bg-card pl-2 pr-1 h-8 text-sm`, слева иконка типа 14px (картинка / видео / документ — SVG),
  имя `truncate max-w-40`, справа кнопка-крестик 24px SVG (`M5 5l6 6M11 5l-6 6`), hover — `bg-background`.
- Ошибка файла (размер/тип) — `FieldError` под полем, не `alert`.

**Ошибки форм** — все формы с `noValidate`; проверка в обработчике, сообщения — наши:
```tsx
export function FieldError({ id, message }: { id?: string; message?: string | null }) {
  if (!message) return null;
  return (
    <p id={id} role="alert" className="chat-in mt-2 flex items-start gap-2 text-sm text-danger">
      <svg viewBox="0 0 16 16" aria-hidden className="mt-0.5 size-4 shrink-0"><circle cx="8" cy="8" r="6.5" fill="none" stroke="currentColor" strokeWidth="1.5" /><path d="M8 4.5v4M8 11h.01" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" /></svg>
      {message}
    </p>
  );
}
```
- Поле с ошибкой: `aria-invalid="true"` (рамка `danger` уже есть в `ui.input`) + `aria-describedby` на сообщение; фокус — на первое поле с ошибкой.
- Ошибка всей формы (ответ сервера) — плашка `rounded-xl border border-danger/30 bg-danger/5 px-4 py-3 text-sm text-danger` с той же иконкой.
- Успех (вместо `text-green-600` в профиле) — `flex gap-2 text-sm` + искра `text-spark` + текст `text-foreground`.
- Тексты ошибок клиентской проверки (новые служебные — на одобрение):
  почта пустая — «Укажите почту»; неверный формат — «Похоже, в почте опечатка»; согласие — «Нужно согласие, чтобы продолжить»;
  код — «Введите 6 цифр из письма»; обязательное поле админки — «Заполните поле».
- Вход: `requestCode` сначала проверяет почту (`/^\S+@\S+\.\S+$/`) и `consent`, при ошибке — `FieldError` у поля/чекбокса, запрос не уходит.

### 4.3 Подтверждения — `components/confirm.tsx` (клиентский)
Вместо `confirm()` в `refund-button.tsx` и любых будущих `confirm/alert`.
```tsx
export function useConfirm() {
  const ref = useRef<HTMLDialogElement>(null);
  const resolver = useRef<(v: boolean) => void>(null);
  const [text, setText] = useState({ title: "", body: "", ok: "Да", danger: false });
  const ask = (t: Partial<typeof text> & { title: string }) => { setText({ body: "", ok: "Да", danger: false, ...t }); ref.current?.showModal(); return new Promise<boolean>((r) => (resolver.current = r)); };
  const done = (v: boolean) => { ref.current?.close(); resolver.current?.(v); };
  const dialog = (
    <dialog ref={ref} onCancel={(e) => { e.preventDefault(); done(false); }} onClick={(e) => e.target === ref.current && done(false)}
      className="confirm m-auto w-[min(92vw,420px)] rounded-2xl border border-border bg-card p-0 text-foreground shadow-[0_40px_100px_-30px_rgb(0_0_0/0.5)]">
      <div className="p-6">
        <Spark className={`size-5 ${text.danger ? "text-danger" : "text-spark"}`} />
        <h2 className="mt-4 text-lg font-semibold">{text.title}</h2>
        {text.body && <p className="mt-2 text-sm leading-relaxed text-muted">{text.body}</p>}
        <div className="mt-6 flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
          <button type="button" autoFocus onClick={() => done(false)} className={ui.buttonGhost}>Отмена</button>
          <button type="button" onClick={() => done(true)} className={text.danger ? `${ui.buttonDanger} bg-danger! text-white! border-danger!` : ui.button}>{text.ok}</button>
        </div>
      </div>
    </dialog>
  );
  return { ask, dialog };
}
```
CSS: `.confirm::backdrop { background: rgb(10 10 9 / .55); backdrop-filter: blur(4px); }` + при no-preference `dialog[open] { animation: chat-in .2s both; }`.
Фокус по умолчанию — «Отмена» (безопасно), Esc и клик по фону — отмена. Возврат денег: `ask({ title: "Вернуть деньги за этот платёж полностью?", ok: "Вернуть", danger: true })`.
Удаление аккаунта в профиле (ввод «УДАЛИТЬ») — оставить как есть + этот же диалог перед отправкой.
Контраст `text-white` на `--danger` #c2341b — 5.5:1; в тёмной теме для danger-кнопки — `text-[#1a0a07]` на `#ff7a62` (8:1).

## 5. Окно чата в первом экране — бесконечный цикл (`components/hero-chat.tsx`, клиентский)

Сценарий «как мы ведём человека»: знакомство → бриф → эскиз → правки → запуск. Реплики появляются по очереди,
перед репликой агента — «печатает» (три искры), перед репликой человека — текст «набирается» в поле ввода внизу окна
(с курсором-чертой цвета искры) и «отправляется». В конце — пауза 3 с, окно плавно гаснет (0.4 с) и сценарий начинается сначала.

**Реплики** (примерные, иллюстрация — согласовать с заказчиком):
| # | Кто | Шаг | Текст / вложение |
|---|---|---|---|
| 1 | агент | Бриф | Привет! Расскажите, что за дело? |
| 2 | человек | Бриф | Кофейня у парка. Нужен сайт с меню. |
| 3 | агент | Бриф | Записал. Ещё адрес и запись на события? |
| 4 | человек | Бриф | Да, и фото зала. |
| 5 | агент | Эскиз | Вот эскиз главной ✶ — *карточка «Эскиз · главная», мини-макет* |
| 6 | человек | Правки | Теплее цвета и фото крупнее. |
| 7 | агент | Правки | Готово, посмотрите. — *карточка «Эскиз · v2», мини-макет с крупным блоком фото и тёплым цветом* |
| 8 | человек | Запуск | Отлично, запускаем! |
| 9 | агент | Запуск | Сайт работает ✶ — *карточка-ссылка «https://ваш-сайт.ru» с замком; рядом с текстом — искра* |

```tsx
"use client";
import { useEffect, useRef, useState } from "react";
import { Spark } from "@/components/brand";

type Msg = { who: "agent" | "user"; step: 0 | 1 | 2 | 3; text: string; card?: "sketch" | "sketch2" | "live" };
const STEPS = ["Бриф", "Эскиз", "Правки", "Запуск"];
// примерные реплики, иллюстрация
const SCRIPT: Msg[] = [
  { who: "agent", step: 0, text: "Привет! Расскажите, что за дело?" },
  { who: "user", step: 0, text: "Кофейня у парка. Нужен сайт с меню." },
  { who: "agent", step: 0, text: "Записал. Ещё адрес и запись на события?" },
  { who: "user", step: 0, text: "Да, и фото зала." },
  { who: "agent", step: 1, text: "Вот эскиз главной", card: "sketch" },
  { who: "user", step: 2, text: "Теплее цвета и фото крупнее." },
  { who: "agent", step: 2, text: "Готово, посмотрите.", card: "sketch2" },
  { who: "user", step: 3, text: "Отлично, запускаем!" },
  { who: "agent", step: 3, text: "Сайт работает", card: "live" },
];
const T = { agentTyping: 1100, perChar: 38, afterSend: 350, read: 900, end: 3000, fade: 400 };

export function HeroChat() {
  const [shown, setShown] = useState(0);              // сколько реплик видно
  const [typing, setTyping] = useState(false);        // агент печатает
  const [draft, setDraft] = useState("");             // человек набирает
  const [fading, setFading] = useState(false);
  const [still, setStill] = useState(false);          // reduced-motion или вкладка скрыта
  const feed = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const mq = matchMedia("(prefers-reduced-motion: reduce)");
    if (mq.matches) { setStill(true); setShown(SCRIPT.length); return; }   // статичный кадр: весь диалог
    let alive = true; const timers: number[] = [];
    const wait = (ms: number) => new Promise<void>((r) => timers.push(window.setTimeout(r, ms)));
    (async function loop() {
      while (alive) {
        for (let i = 0; i < SCRIPT.length && alive; i++) {
          const m = SCRIPT[i];
          if (m.who === "agent") { setTyping(true); await wait(T.agentTyping); setTyping(false); }
          else { for (let c = 1; c <= m.text.length && alive; c++) { setDraft(m.text.slice(0, c)); await wait(T.perChar); } await wait(T.afterSend); setDraft(""); }
          setShown(i + 1); await wait(T.read + (m.card ? 600 : 0));
        }
        await wait(T.end); setFading(true); await wait(T.fade); setShown(0); setFading(false); await wait(300);
      }
    })();
    return () => { alive = false; timers.forEach(clearTimeout); };
  }, []);

  useEffect(() => { feed.current?.scrollTo({ top: feed.current.scrollHeight, behavior: still ? "auto" : "smooth" }); }, [shown, typing, still]);

  const step = shown ? SCRIPT[shown - 1].step : 0;
  return (
    <div aria-hidden className="light-edge relative rounded-2xl bg-card/90 shadow-[0_30px_80px_-30px_rgb(0_0_0/0.6)] backdrop-blur">
      {/* шапка окна */}
      <div className="flex items-center gap-2 border-b border-border px-4 py-3">
        <span className="grid size-6 place-items-center rounded-full bg-foreground text-spark"><Spark className="size-3.5" /></span>
        <span className="font-mono text-[11px] uppercase tracking-[0.08em] text-muted">Lumen · агент</span>
        <span className="ml-auto inline-flex items-center gap-1.5 font-mono text-[11px] uppercase tracking-[0.08em] text-muted">
          <span className="relative size-1.5 rounded-full bg-spark"><span className="spark-ping absolute inset-0 rounded-full bg-spark" /></span>онлайн
        </span>
      </div>
      {/* шаги-прогресс */}
      <ol className="flex items-center gap-1 border-b border-border px-4 py-2.5 font-mono text-[10px] uppercase tracking-[0.08em]">
        {STEPS.map((s, i) => (
          <li key={s} className={`flex items-center gap-1 transition-colors duration-300 ${i === step ? "text-foreground" : i < step ? "text-muted" : "text-muted/50"}`}>
            {i === step ? <Spark className="size-2.5 text-spark" /> : <span className={`size-1 rounded-full ${i < step ? "bg-muted" : "bg-border"}`} />}
            {s}{i < STEPS.length - 1 && <span className="mx-1.5 h-px w-3 bg-border sm:w-5" />}
          </li>
        ))}
      </ol>
      {/* лента */}
      <div ref={feed} className={`scroll-none h-[300px] space-y-2.5 overflow-hidden p-4 text-sm leading-relaxed transition-opacity duration-400 sm:h-[340px] sm:p-5 ${fading ? "opacity-0" : "opacity-100"}`}>
        {SCRIPT.slice(0, shown).map((m, i) => <Bubble key={i} m={m} />)}
        {typing && (
          <div className="chat-in spark-typing inline-flex gap-1 rounded-full border border-border bg-background px-3 py-2.5 text-spark">
            <Spark className="size-2.5" /><Spark className="size-2.5" /><Spark className="size-2.5" />
          </div>
        )}
      </div>
      {/* поле ввода-иллюстрация */}
      <div className="m-3 mt-0 flex h-11 items-center gap-2 rounded-xl border border-border bg-background px-3 text-sm">
        <span className={`min-w-0 flex-1 truncate ${draft ? "text-foreground" : "text-muted/70"}`}>
          {draft || "Напишите сообщение…"}{draft && <span className="caret ml-px inline-block h-4 w-[2px] translate-y-[3px] rounded bg-spark" />}
        </span>
        <span className={`grid size-7 place-items-center rounded-lg transition-colors ${draft ? "bg-foreground text-background" : "bg-border text-muted"}`}>
          <svg viewBox="0 0 16 16" className="size-3.5"><path d="M8 13V3M3.5 7.5 8 3l4.5 4.5" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round" /></svg>
        </span>
      </div>
    </div>
  );
}

function Bubble({ m }: { m: Msg }) {
  if (m.who === "user")
    return <p className="chat-in ml-auto w-fit max-w-[82%] rounded-2xl rounded-br-md bg-foreground px-3.5 py-2 text-background">{m.text}</p>;
  return (
    <div className="chat-in w-fit max-w-[86%] rounded-2xl rounded-bl-md border border-border bg-background px-3.5 py-2">
      <span className="inline-flex items-center gap-1.5">{m.text}{m.card && m.card !== "sketch2" && <Spark className="size-3 text-spark" />}</span>
      {m.card === "sketch" && <Sketch />}
      {m.card === "sketch2" && <Sketch warm />}
      {m.card === "live" && (
        <span className="mt-2 flex items-center gap-2 rounded-lg border border-border bg-card px-2.5 py-1.5 font-mono text-[11px] text-muted">
          <svg viewBox="0 0 16 16" className="size-3.5"><rect x="3.5" y="7" width="9" height="6.5" rx="1.5" fill="none" stroke="currentColor" strokeWidth="1.5" /><path d="M5.5 7V5.5a2.5 2.5 0 0 1 5 0V7" fill="none" stroke="currentColor" strokeWidth="1.5" /></svg>
          https://ваш-сайт.ru
        </span>
      )}
    </div>
  );
}

function Sketch({ warm = false }: { warm?: boolean }) {
  return (
    <span className="mt-2 block w-48 rounded-lg border border-border bg-card p-2 sm:w-56">
      <span className="font-mono text-[10px] uppercase tracking-[0.08em] text-muted">{warm ? "Эскиз · v2" : "Эскиз · главная"}</span>
      <span className="mt-1.5 grid grid-cols-3 gap-1">
        <span className={`col-span-3 rounded ${warm ? "h-12 bg-spark-soft" : "h-8 bg-brand-soft"} transition-all`} />
        <span className="h-4 rounded bg-border" /><span className="h-4 rounded bg-border" /><span className={`h-4 rounded ${warm ? "bg-spark/60" : "bg-border"}`} />
      </span>
    </span>
  );
}
```
- Подключение в `page.tsx`: `import { HeroChat } from "@/components/hero-chat"`, локальный `HeroChat` из v2 удалить. Сервер рендерит кадр `shown=0`
  (пустая лента + шапка + поле) — без прыжка вёрстки, потому что высота ленты фиксирована.
- Длительность цикла ≈ 22 с. Лента фиксированной высоты, старые реплики уходят вверх (скролл внутри, полосы нет — `scroll-none`).
- **reduced-motion:** статичный кадр — весь диалог, лента прокручена к концу (видны эскиз v2 и «Сайт работает»), шаг «Запуск» активен, ничего не мигает.
- Вкладка скрыта — по желанию пауза (`document.visibilityState`), чтобы не крутить таймеры; не обязательно.
- `spark-ping` (точка «онлайн»): `@keyframes ping { 75%,100% { scale: 2.4; opacity: 0 } }`, `.spark-ping { animation: ping 2s cubic-bezier(0,0,.2,1) infinite }` — только в no-preference.
- Ширины: 375 — окно во всю колонку, лента 300px, мини-эскиз `w-48`; 768 — `max-w-md` как в v2; 1440 — справа, лента 340px.

**Световой край** (акцент 3) — `globals.css`:
```css
.light-edge { position: relative; isolation: isolate; }
.light-edge::before {
  content: ""; position: absolute; inset: 0; border-radius: inherit; padding: 1px; z-index: -1; pointer-events: none;
  background: linear-gradient(140deg, color-mix(in oklab, var(--spark) 70%, transparent), var(--border) 35%, var(--border) 65%, color-mix(in oklab, var(--brand) 70%, transparent));
  -webkit-mask: linear-gradient(#000 0 0) content-box, linear-gradient(#000 0 0); -webkit-mask-composite: xor; mask-composite: exclude;
}
.glow { background:
  radial-gradient(closest-side at 65% 35%, color-mix(in oklab, var(--spark) 22%, transparent), transparent 70%),
  radial-gradient(closest-side, color-mix(in oklab, var(--brand) 30%, transparent), transparent); }
```
`border border-border` у окна чата убрать (край даёт `::before`).

## 6. Мелкие элементы — богаче (`page.tsx`, `components/ui.ts`)

### 6.1 Кнопки — микро-состояния (`ui.ts`)
```ts
button:
  "group/btn relative inline-flex h-11 items-center justify-center gap-2 rounded-full bg-foreground px-5 text-sm font-medium text-background " +
  "transition-[background-color,transform,box-shadow] duration-150 hover:bg-foreground/88 hover:shadow-[0_8px_24px_-10px_color-mix(in_oklab,var(--foreground)_60%,transparent)] " +
  "active:translate-y-px active:scale-[.985] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand focus-visible:ring-offset-2 focus-visible:ring-offset-background " +
  "disabled:pointer-events-none disabled:opacity-40",
buttonGhost:
  "group/btn relative inline-flex h-11 items-center justify-center gap-2 rounded-full border border-border px-5 text-sm font-medium " +
  "transition-[border-color,background-color,transform] duration-150 hover:border-foreground/40 hover:bg-card active:translate-y-px active:scale-[.985] " +
  "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:pointer-events-none disabled:opacity-40",
arrow: "inline-block transition-transform duration-200 group-hover/btn:translate-x-0.5",
```
- Стрелка «→» в «Начать проект» — SVG 16px (`M3 8h10M9 4l4 4-4 4`) с классом `ui.arrow`; при hover сдвигается на 2px.
- Главная кнопка первого экрана: слева перед текстом маленькая искра `size-3 text-spark` (на светлой кнопке в ink — `--spark` на `#efeee9` 9.6:1),
  при hover искра поворачивается на 45° (`group-hover/btn:rotate-45 transition-transform duration-300`).
- Загрузка (`pending`) во всех формах: вместо текста-only — `<Spark className="spin size-3.5" />` + текст «Отправляем…» (тексты как есть).
- «Войти» в шапке — `ui.button` компактная (как v2).

### 6.2 Шапка
- Логотип: `<Link href="/" aria-label="Lumen — на главную"><LumenLogo className="h-6 w-auto" /></Link>` — квадратик `bg-brand` и текст `SITE_NAME` из v2 убрать.
- «Тарифы»: ссылка с подчёркиванием, которое вырастает слева направо (`bg-[linear-gradient(currentColor,currentColor)] bg-[length:0%_1px] bg-no-repeat bg-left-bottom hover:bg-[length:100%_1px] transition-[background-size] duration-300`).
- При прокрутке (sticky) — без изменений; `backdrop-blur` остаётся.

### 6.3 Метки (eyebrow) и номера разделов
- Eyebrow в первом экране: точку `bg-brand` → `<Spark className="size-3 text-spark" />`.
- Номера разделов «01», «02»: `<p className={`${ui.eyebrow} flex items-center gap-2`}><Spark className="size-2.5 text-spark" />01<span className="h-px w-10 bg-border" /></p>` —
  искра, номер, короткая волосяная линия.

### 6.4 «Как это работает» — шаги
- Над шагами (lg) — горизонтальная «нить»: `h-px bg-border` через всю ширину, на ней у каждого шага узел-искра 12px
  `text-spark` на подложке `bg-background px-1` (чтобы линия «прерывалась» искрой). На 375/768 нить вертикальная слева
  (`border-l border-border pl-6`, искра на `-left-[7px]` у номера). Разделители-линии шагов из v2 тогда убрать (`border-t`/`border-l`).
- Номер шага: `font-mono text-sm text-muted` + при hover на шаг номер становится `text-foreground`, а искра делает один поворот (`group-hover:rotate-90 duration-500`).
- Номера крупные-фоновые не вводим — хватает нити.

### 6.5 Тарифы — карточки
- Все карточки: `transition-[transform,box-shadow,border-color] duration-200 hover:-translate-y-1 hover:shadow-[0_24px_60px_-30px_rgb(0_0_0/0.35)]`
  (только при `@media (hover:hover)` — `hover:` в Tailwind v4 уже так работает).
- Цена: `tabular-nums`; знак валюты и «/ 30 дн.» — `text-muted` как в v2.
- Строка лимита: слева иконка-«батарея» 14px не нужна — вместо `border-t` ставим пунктир `border-t border-dashed border-border`.
- **Выделенная карточка (вторая):** рамка `border-foreground` → класс `light-edge` (градиентный край) + `bg-card`; маркер «●» → `<Spark className="size-4 text-spark spark-twinkle" />`
  в правом верхнем углу (`absolute right-6 top-6`), без текста-бейджа (тексты не меняем). Кнопка — `ui.button`, остальные — `ui.buttonGhost`.
- На 375/768 — одна колонка; выделенная карточка не поднимается (`lg:-translate-y-0`), чтобы не ломать стопку.

### 6.6 Преимущества (список под тарифами)
- «✓» → свой маркер: кружок 22px `bg-spark-soft` с искрой `size-3 text-spark` по центру. Строки — `border-t border-border py-4`, как v2.
- Hover на строке — маркер поворачивается на 45° (`group-hover:rotate-45`), строка без подсветки.

### 6.7 Разделитель между «Как это работает» и «Тарифы»
`border-t` у `#pricing` заменить на «искру на линии»:
`<div aria-hidden className="flex items-center gap-4 text-spark"><span className="h-px flex-1 bg-border" /><Spark className="size-3" /><span className="h-px flex-1 bg-border" /></div>`.

### 6.8 Подвал — гигантский словознак (акцент 2)
```tsx
<footer className="surface-ink relative overflow-hidden">
  <div className={`${container} flex flex-wrap justify-between gap-4 pt-12 text-sm text-muted`}>
    <span>© Lumen</span>
    <span>Оферта · Политика конфиденциальности</span>
  </div>
  <div aria-hidden className={`${container} pointer-events-none mt-8 -mb-[6%] select-none`}>
    <LumenLogo className="w-full h-auto text-foreground/[0.07]" />   {/* «водяной» словознак, искра остаётся тёплой и мерцает */}
  </div>
</footer>
```
- Словознак во всю ширину контейнера, нижняя часть обрезана краем страницы (`-mb-[6%]` + `overflow-hidden`); буквы — 7% (декор, не текст),
  искра — полный `--spark` с `spark-twinkle`. На 375 — та же логика, высота ~70px.
- `© {SITE_NAME}` подставит «Lumen».

### 6.9 Вход и кабинет
- `/login`: над формой логотип `h-6`; чекбокс согласия — `Checkbox` (п.4.2), поле почты и кода — `FieldError`, форма `noValidate`.
- Кабинет: в шапке логотип `h-5`; горизонтальная навигация на 375 — `scroll-none`; активный пункт навигации — точка-искра `size-2.5 text-spark` слева.
- Сообщения агента в кабинете: индикатор «печатает» — те же три искры (`spark-typing`), чтобы главная и кабинет совпадали.

## 7. Ширины — отличия от v2
| | 375 | 768 | 1440 |
|---|---|---|---|
| Шапка | лого 24px · тема · «Войти» | + «Тарифы» | как 768 |
| Чат героя | после кнопок, лента 300px, шаги-прогресс в 1 строку (`text-[10px]`, линии `w-3`) | `max-w-md`, лента 340px | справа, 340px |
| Шаги | вертикальная нить слева | 2 колонки, нить слева у каждой | горизонтальная нить с 4 искрами |
| Тарифы | стопка, без подъёма | стопка | 3 колонки, hover-подъём |
| Подвал | словознак ~70px высоты | ~150px | ~300px (по ширине контейнера) |

## 8. Проверка перед сдачей (фронтенд + QA)
- [ ] 375/768/1440 × светлая/тёмная/система: главная, вход, кабинет (чат, профиль), админка тарифов. Скриншоты.
- [ ] Поиск по коду: нет `type="checkbox"`/`"radio"` без `sr-only`, нет `<select`, нет `confirm(`/`alert(`, нет `title=`, у каждого `<form>` есть `noValidate`, нет `required` без своей проверки.
- [ ] Полосы прокрутки свои в Chrome/Safari/Firefox (список часовых поясов, чат кабинета, таблицы админки).
- [ ] Клавиатура: Select (стрелки, Enter, Esc, Tab), Checkbox/Switch (Space), диалог (Esc, фокус на «Отмена», фокус возвращается на кнопку).
- [ ] Цикл чата: 3 круга подряд без накопления реплик и без прыжков вёрстки; reduced-motion — статичный полный диалог.
- [ ] Логотип сверен наложением с `assets/lumen-logo.png`; фавикон в светлой и тёмной вкладке.
- [ ] Контраст: `--spark` как текст ≥ 4.5:1 во всех трёх поверхностях (значения в п.1); ошибки `danger` — как в v2.
- [ ] Первый экран 375: H1 и «Начать проект» видны без прокрутки.

## 9. Новое — на одобрение заказчику
1. Тёплый цвет искры (оранжевый на светлом, янтарный на тёмном) — на логотипе искра цветная, буквы графитовые. Можно сделать искру как на картинке — чёрной.
2. Искра как фирменный знак: маркеры, номера разделов, «печатает», курсор, выбранный пункт списка.
3. Лёгкое мерцание искры у логотипа (раз в 6 секунд) и в подвале.
4. Чат на первом экране крутится по кругу: бриф → эскиз → правки → запуск; реплики про кофейню — примерные.
5. Шаги-прогресс в окне чата («Бриф · Эскиз · Правки · Запуск»).
6. Гигантский полупрозрачный «lumen» в подвале.
7. Градиентный «световой» край у окна чата и у выделенного тарифа, тёплый блик в свечении первого экрана.
8. Нить с искрами в блоке «Как это работает».
9. Подъём карточек тарифов при наведении.
10. Новые служебные тексты ошибок форм и подсказка «Отпустите, чтобы прикрепить» (п.4.2).

## 10. Спорные места
- Искра на картинке — чёрная. Цветная — сильнее как мотив, но это изменение бренда → пункт 1 на одобрение. Если «нет» — `--spark` = `--foreground`
  и мотив держится на форме (тогда «печатает» и маркеры будут графитовыми; выразительность ниже).
- Словознак перерисован штрихами по картинке; исходника в векторе нет. Если у заказчика есть SVG/AI — взять его вместо путей из п.2.
- `field-sizing: content` пока не во всех браузерах — где не работает, textarea просто фиксированной высоты, без уголка ресайза.
- `<dialog>` — нативный элемент, но вид полностью наш (фон, рамка, кнопки, анимация); браузерных окон не остаётся.
