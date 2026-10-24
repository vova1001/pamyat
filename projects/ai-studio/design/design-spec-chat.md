# Design spec — чат Lumen (кабинет + админка)

Стиль — v3 (`design-spec-v3.md`): токены v2 + `--spark`, свои элементы из `components/forms.tsx`, `ui.ts`, `Spark` из `components/brand.tsx`.
Новых библиотек нет. Ни одного браузерного вида: `<select>`, `alert/confirm`, `title`, нативные `<video controls>`, спиннеры — запрещены.
Искра ✶ в чате — только у «активного» (агент печатает/думает, выбранный пункт, «рекомендуем», текущий этап). Не больше 2 на экран.

Файлы:
- `lib/types.ts` — расширить `Message.meta` (п.1).
- `components/chat/` (новая папка): `bubble.tsx` (вместо `message-bubble.tsx`, тот — реэкспорт), `markdown.tsx`, `blocks.tsx`
  (choice, cards, form, payment, preview, files, progress), `status.tsx` (печатает/думает/человек), `composer.tsx` (поле ввода), `empty.tsx`.
- `app/cabinet/chat.tsx` — использует `composer`, `empty`, `status`, обработку ответов блоков.
- `app/admin/(panel)/users/[id]/admin-chat.tsx` — панель сессии + конструктор ответа (п.14–16).
- `app/admin/(panel)/sessions/page.tsx` — новый раздел «Сессии» (п.17).

---

## 0. Токены — добавить в `app/globals.css`

```css
:root                  { --ok: #1f7a4d; --ok-soft: #e8f4ec; }   /* 5.1:1 на #f7f6f3 */
:root[data-theme="dark"]{ --ok: #5fd39a; --ok-soft: #12261b; }  /* 9.8:1 на #0f0f0e */
/* тот же блок в @media (prefers-color-scheme: dark) :root:not([data-theme="light"]) */
@theme inline { --color-ok: var(--ok); --color-ok-soft: var(--ok-soft); }

@keyframes think-sweep { from { background-position: 200% 0; } to { background-position: -200% 0; } }
@keyframes collapse-in { from { opacity: 0; scale: .98; } to { opacity: 1; scale: 1; } }
@media (prefers-reduced-motion: no-preference) {
  .think-text { background: linear-gradient(90deg, var(--muted) 0 40%, var(--foreground) 50%, var(--muted) 60% 100%) 0 0/200% 100%;
                -webkit-background-clip: text; background-clip: text; color: transparent; animation: think-sweep 2.4s linear infinite; }
  .collapse-in { animation: collapse-in .25s ease-out both; }
}
```
Световой край (v3, акцент 3) переиспользуется классом `.light-edge` из v3 — у карточки «рекомендуем» и у блока оплаты.

Общие классы чата — добавить в `components/ui.ts`:
```ts
export const chat = {
  col: "mx-auto w-full max-w-2xl",                                  // 672px; на 375 — во всю ширину с px-4
  bubbleAgent: "rounded-2xl rounded-bl-md border border-border bg-card px-4 py-3",
  bubbleUser: "rounded-2xl rounded-br-md bg-foreground px-4 py-3 text-background",
  bubbleSupport: "rounded-2xl rounded-bl-md border border-support/40 bg-support-soft px-4 py-3",
  block: "w-full max-w-[34rem] rounded-2xl border border-border bg-card",  // интерактивные блоки — шире пузыря, не больше 544px
  meta: "font-mono text-[11px] uppercase tracking-[0.08em] text-muted",
  chip: "inline-flex min-h-10 items-center gap-2 rounded-full border border-border bg-card px-4 text-sm font-medium transition-[border-color,background-color,transform] duration-150 " +
        "hover:border-foreground/40 active:scale-[.97] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand focus-visible:ring-offset-2 focus-visible:ring-offset-background " +
        "aria-pressed:border-foreground aria-pressed:bg-foreground aria-pressed:text-background disabled:pointer-events-none disabled:opacity-40",
};
```

## 1. Данные — `Message.meta`

Обратно совместимо: `meta.buttons: string[]` остаётся (= `choice` одиночный). Новое — `meta.blocks`:
```ts
export type ChatBlock =
  | { kind: "choice"; id: string; mode: "single" | "multi"; options: { value: string; label: string }[]; other?: boolean; min?: number; max?: number }
  | { kind: "cards"; id: string; mode: "single" | "multi"; items: { value: string; title: string; caption?: string; image?: string; icon?: "site"|"shop"|"landing"|"blog"|"spark"; badge?: string; recommended?: boolean }[] }
  | { kind: "form"; id: string; title?: string; submit: string; fields: FormField[] }
  | { kind: "payment"; id: string; paymentId?: string; title: string; amount: number; currency: string; items?: { label: string; amount: number }[]; status: "due"|"pending"|"succeeded"|"failed"|"canceled" }
  | { kind: "preview"; id: string; projectId: string; version: number; url: string; image?: string; note?: string; status: "review"|"approved"|"changes" }
  | { kind: "progress"; stage: number; steps?: string[]; note?: string };
export type FormField = { name: string; label: string; type: "text"|"textarea"|"email"|"tel"|"code"|"select"; required?: boolean; placeholder?: string; options?: { value: string; label: string }[] };

type Meta = { buttons?: string[]; blocks?: ChatBlock[]; answers?: Record<string, BlockAnswer>; event?: SystemEvent; author?: { kind: "agent"|"admin"; name?: string }; attachmentIds?: string[] };
type BlockAnswer = { values: string[]; other?: string; fields?: Record<string,string>; at: string };
type SystemEvent = "stage_changed"|"subscription_started"|"subscription_renewed"|"subscription_expired"|"handoff_admin"|"handoff_agent"|"limit_low"|"version_published";
```
Ответ на блок = обычное сообщение пользователя с `meta.reply = { blockId, values, other?, fields? }`; текст сообщения — человекочитаемый итог
(«Лендинг, Магазин»; «Имя: Анна · Почта: a@b.ru»). Агент читает текст, фронт по `answers[blockId]` рисует блок «свёрнутым».
Блок активен, только если он в **последнем** сообщении агента и ещё не отвечен; остальные — `disabled` в свёрнутом виде.
API: `POST /api/chat` получает `{ message, reply? }`. Это задача бэкенду — в спорные места (п.19).

## 2. Лента и выравнивание

```tsx
<div role="log" aria-live="polite" aria-relevant="additions" className="flex-1 overflow-y-auto px-4 pb-6 pt-8 sm:px-6">
  <ol className={`${chat.col} flex flex-col gap-1.5`}>{/* группы */}</ol>
</div>
```
- **Группировка:** подряд идущие сообщения одного автора < 3 мин — одна группа: `gap-1`, между группами `mt-5`.
  Скругление «хвоста» (`rounded-bl-md` / `rounded-br-md`) — только у последнего пузыря группы; у остальных `rounded-2xl`.
- **Подпись группы** (над первым пузырём, не у пользователя в кабинете):
  `<p className={`${chat.meta} mb-1.5 flex items-center gap-1.5`}>` + аватар 20px + «Lumen» / «Анна · поддержка» / в админке «Заказчик».
  Аватар агента: `<span className="grid size-5 place-items-center rounded-full bg-foreground text-spark"><Spark className="size-2.5"/></span>`;
  администратора: `size-5 rounded-full bg-support-soft text-support text-[10px] font-semibold` с инициалом.
- **Время:** под последним пузырём группы, `text-[11px] text-muted tabular-nums mt-1`, `ЧЧ:ММ`; у своих справа + статус доставки (п.12).
- **Разделитель дня:** `<li className="my-4 flex items-center gap-3 text-xs text-muted before:h-px before:flex-1 before:bg-border after:h-px after:flex-1 after:bg-border">Сегодня</li>`.
- Ширина пузыря: `max-w-[85%] sm:max-w-[78%]`. Новые сообщения — `chat-in`.
- Кнопка «вниз»: если пользователь прокрутил вверх > 200px — справа снизу над полем:
  `<button className="absolute -top-14 right-4 grid size-10 place-items-center rounded-full border border-border bg-card shadow-lg">↓</button>`
  + точка `size-2 rounded-full bg-spark` в углу, если пришло новое. Автопрокрутка — только если пользователь у низа.

## 3. Текст и markdown — `components/chat/markdown.tsx`

Свой мини-парсер (без библиотек, без `dangerouslySetInnerHTML`): строки → блоки, инлайн — регулярками в React-узлы.
Поддержать: абзацы, `**жирный**`, `*курсив*`, `` `код` ``, ссылки `[текст](url)` и голые `https://`, списки `- ` и `1. `, `> цитата`,
заголовок `### ` (только h3-уровень), блок кода ```.
```tsx
<div className="space-y-2.5 break-words text-[15px] leading-relaxed [&_strong]:font-semibold">
  <p>…</p>
  <ul className="space-y-1 pl-1">{/* li: */}<li className="flex gap-2.5"><span aria-hidden className="mt-[.6em] size-1.5 shrink-0 rounded-full bg-current opacity-50" />…</li></ul>
  <ol className="space-y-1 pl-1">{/* li: */}<li className="flex gap-2.5"><span className="w-4 shrink-0 font-mono text-xs leading-[1.7rem] text-muted tabular-nums">1.</span>…</li></ol>
  <blockquote className="border-l-2 border-spark/60 pl-3 text-muted">…</blockquote>
  <h3 className="pt-1 text-[15px] font-semibold">…</h3>
  <code className="rounded-md bg-background px-1.5 py-0.5 font-mono text-[13px]">…</code>
  <pre className="overflow-x-auto rounded-xl bg-background p-3 font-mono text-[13px] leading-5">…</pre>
  <a href={url} target="_blank" rel="noopener noreferrer" className="font-medium text-brand underline decoration-brand/30 underline-offset-4 hover:decoration-brand">…</a>
</div>
```
- В пузыре пользователя (`bg-foreground`) ссылки `text-background underline decoration-background/40`, `code` — `bg-background/15`.
- Ссылки: разрешены только `https:`/`http:`/`mailto:`/`tel:`; иначе выводится текстом. Длинный URL — `break-all`.
- Стриминг: во время `delta` в конце текста — каретка `<span className="caret ml-0.5 inline-block h-[1.1em] w-0.5 translate-y-[.2em] rounded-full bg-spark" />`.

## 4. Быстрые ответы — `choice`

Под последним пузырём агента, выровнены влево (как агент), `mt-2 flex flex-wrap gap-2`. На 375 — те же чипы, перенос строк; если ≥ 5 и
подписи короткие — горизонтальная лента `flex-nowrap overflow-x-auto scroll-none -mx-4 px-4` с градиентным краем справа.

**Одиночный** (по нажатию сразу отправляется):
```tsx
<div role="group" aria-label="Варианты ответа" className="mt-2 flex flex-wrap gap-2">
  {options.map(o => <button key={o.value} type="button" className={chat.chip} onClick={() => answer([o.value])}>{o.label}</button>)}
  {other && <button type="button" className={`${chat.chip} border-dashed text-muted`} onClick={openOther}>Свой вариант…</button>}
</div>
```
**Множественный** — чипы-переключатели + кнопка «Готово» с числом:
```tsx
<button type="button" aria-pressed={picked.has(o.value)} className={chat.chip} onClick={() => toggle(o.value)}>
  <span aria-hidden className="grid size-4 place-items-center rounded-[5px] border-[1.5px] border-current/40 [[aria-pressed=true]>&]:border-background [[aria-pressed=true]>&]:bg-background">
    {picked.has(o.value) && <svg viewBox="0 0 16 16" className="size-3 text-foreground"><path d="M3.5 8.5 6.5 11.5 12.5 4.5" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round"/></svg>}
  </span>
  {o.label}
</button>
…
<button type="button" disabled={picked.size < (min ?? 1)} className={`${ui.button} h-10! px-4!`} onClick={submit}>
  Готово{picked.size ? <span className="tabular-nums opacity-70">· {picked.size}</span> : null} <span className={ui.arrow}>→</span>
</button>
```
`max` достигнут — невыбранные `disabled`, под группой `text-xs text-muted`: «Можно выбрать до 3».

**«Свой вариант»** — чип превращается в поле в той же строке (не модалка):
```tsx
<form onSubmit={…} className="collapse-in flex w-full items-center gap-1.5 rounded-full border border-brand bg-card p-1 pl-4 ring-4 ring-brand/10 sm:w-auto sm:min-w-80">
  <input autoFocus aria-label="Свой вариант" placeholder="Напишите свой вариант" className="min-w-0 flex-1 bg-transparent text-sm outline-none" />
  <button className={`${ui.button} h-8! px-3! text-xs!`} disabled={!text.trim()}>Отправить</button>
</form>
```
Esc — свернуть обратно в чип. При «множественном» свой вариант добавляется чипом в выбранные.

**После ответа:** блок исчезает, ответ — обычный пузырь пользователя. В истории (блок не последний) варианты не показываются.

## 5. Карточки выбора — `cards`

```tsx
<div role={mode === "single" ? "radiogroup" : "group"} aria-label={…} className="mt-2 grid w-full max-w-[34rem] grid-cols-2 gap-2 sm:grid-cols-3">
  <button type="button" role={mode === "single" ? "radio" : "checkbox"} aria-checked={sel}
    className="group relative flex flex-col overflow-hidden rounded-2xl border border-border bg-card text-left transition-[border-color,box-shadow,transform] duration-150
               hover:-translate-y-0.5 hover:border-foreground/30 hover:shadow-[0_12px_30px_-18px_color-mix(in_oklab,var(--foreground)_50%,transparent)]
               focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand focus-visible:ring-offset-2 focus-visible:ring-offset-background
               aria-checked:border-foreground aria-checked:shadow-[0_0_0_1px_var(--foreground)] active:scale-[.98]">
    {image
      ? <img src={image} alt="" className="aspect-[4/3] w-full bg-background object-cover" />
      : <span className="grid aspect-[4/3] place-items-center bg-background text-foreground/70"><CardIcon name={icon} className="size-8" /></span>}
    <span className="flex flex-1 flex-col gap-0.5 p-3">
      <span className="text-sm font-semibold leading-snug">{title}</span>
      {caption && <span className="text-xs leading-snug text-muted">{caption}</span>}
    </span>
    {recommended && <span className="absolute left-2 top-2 inline-flex items-center gap-1 rounded-full bg-foreground px-2 py-1 text-[11px] font-medium text-background"><Spark className="size-2.5 text-spark" />Рекомендуем</span>}
    {badge && !recommended && <span className="absolute left-2 top-2 rounded-full bg-card/90 px-2 py-1 text-[11px] font-medium backdrop-blur">{badge}</span>}
    <span aria-hidden className="absolute right-2 top-2 grid size-6 place-items-center rounded-full border-[1.5px] border-border bg-card/90 backdrop-blur group-aria-checked:border-foreground group-aria-checked:bg-foreground group-aria-checked:text-background">
      {sel && <svg viewBox="0 0 16 16" className="size-3"><path d="M3.5 8.5 6.5 11.5 12.5 4.5" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round"/></svg>}
    </span>
  </button>
</div>
<div className="mt-2 flex items-center gap-3">{/* одиночный выбор: тоже «Выбрать», чтобы не отправлять по случайному тапу */}
  <button disabled={!picked.size} className={`${ui.button} h-10! px-4!`}>Выбрать <span className={ui.arrow}>→</span></button>
  <span className="text-xs text-muted">{mode === "multi" ? "Можно несколько" : ""}</span>
</div>
```
- Рекомендуемая карточка — дополнительно `.light-edge` (световой край v3). Только одна на блок.
- Без картинок и иконок (только текст) — `grid-cols-1 sm:grid-cols-2`, карточка `p-4`, иконка не рисуется.
- 375: `grid-cols-2`; 1 или 2 карточки — `grid-cols-2`; 4 — `sm:grid-cols-2`. 1440: `sm:grid-cols-3` при ≥ 3.
- Иконки `CardIcon` — свои SVG-штрихи 24×24, `stroke-width 1.6`, как скрепка в поле ввода.
- **Свёрнутый итог:** `<p className="mt-2 inline-flex items-center gap-2 text-xs text-muted"><Check/> Выбрано: <b className="font-medium text-foreground">Лендинг</b></p>` + пузырь пользователя.

## 6. Форма в чате — `form`

Блок слева, как сообщение агента, `chat.block p-4 sm:p-5`. Поля — `TextField` из `components/forms.tsx` (`ui.input`, `ui.label`),
выбор — `Select` (свой). Ошибки — `FieldError`, проверка `checkRequired` + своя проверка почты/телефона; `noValidate` всегда.
```tsx
<form noValidate onSubmit={submit} className={`${chat.block} mt-2 space-y-4 p-4 sm:p-5`} aria-labelledby={`${id}-t`}>
  {title && <p id={`${id}-t`} className="text-sm font-semibold">{title}</p>}
  {/* text / email / tel / textarea */}
  <TextField name="email" type="email" inputMode="email" autoComplete="email" label="Почта" required error={errors.email} />
  <TextField name="phone" type="tel" inputMode="tel" autoComplete="tel" label="Телефон" placeholder="+7 900 000-00-00" error={errors.phone} />
  {/* code — п.6.1 */}
  {/* select */}
  <Select name="niche" label="Ниша" options={options} />
  <div className="flex flex-wrap items-center gap-3 pt-1">
    <button disabled={sending} className={ui.button}>{sending ? <Spark className="spin size-3.5" /> : null}{submit}</button>
    <span className="text-xs text-muted">Данные видит только команда Lumen</span>
  </div>
</form>
```
- Телефон: маска при вводе (`+7 (900) 000-00-00`), своя, без библиотек; хранение — цифры.
- 375: поля одно под другим, кнопка `w-full`; ≥ 640 — пары коротких полей (почта+телефон) `sm:grid sm:grid-cols-2 sm:gap-3`.

### 6.1 Код из СМС — `CodeField` (новый в `forms.tsx`)
Шесть ячеек, под ними один настоящий `input` (`autoComplete="one-time-code" inputMode="numeric" maxLength={6}`), прозрачный поверх ячеек — вставка
и автоподстановка с телефона работают сами.
```tsx
<div className="relative w-fit">
  <input … className="absolute inset-0 z-10 w-full cursor-text bg-transparent text-transparent caret-transparent outline-none" aria-label="Код из СМС" />
  <div aria-hidden className="flex gap-2">
    {Array.from({ length: 6 }, (_, i) => (
      <span key={i} data-active={focused && i === Math.min(value.length, 5)}
        className="grid h-12 w-10 place-items-center rounded-xl border border-border bg-card font-mono text-lg tabular-nums transition-colors
                   data-[active=true]:border-brand data-[active=true]:ring-4 data-[active=true]:ring-brand/10 aria-[invalid=true]:border-danger">
        {value[i] ?? (focused && i === value.length ? <span className="caret h-5 w-0.5 rounded-full bg-spark" /> : null)}
      </span>))}
  </div>
</div>
<p className="mt-2 text-xs text-muted">Отправить ещё раз через <span className="tabular-nums">0:42</span></p>  {/* потом — ui.textButton «Отправить код ещё раз» */}
```
6 цифр введены — форма отправляется сама. Ошибка — ячейки `border-danger`, лёгкое покачивание (`translate-x` 3 раза, 240 мс, только без reduced-motion).

### 6.2 После отправки — свёрнутый итог
Форма заменяется (`collapse-in`) на:
```tsx
<div className={`${chat.block} mt-2 flex items-start gap-3 p-4`}>
  <span className="grid size-7 shrink-0 place-items-center rounded-full bg-ok-soft text-ok"><Check className="size-3.5" /></span>
  <div className="min-w-0 flex-1">
    <p className="text-sm font-medium">{title ?? "Данные отправлены"}</p>
    <dl className="mt-1.5 grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5 text-sm"><dt className="text-muted">Почта</dt><dd className="truncate">a@b.ru</dd>…</dl>
  </div>
  {editable && <button className={ui.textButton}>Изменить</button>}   {/* только пока это последний блок */}
</div>
```
Код из СМС в итоге не показывается («Телефон подтверждён»). Пароли и коды в текст сообщения не попадают.

## 7. Оплата — `payment`

```tsx
<div className={`${chat.block} light-edge mt-2 overflow-hidden`}>
  <div className="flex items-start justify-between gap-4 p-4 sm:p-5">
    <div><p className={chat.meta}>Счёт</p><p className="mt-1 text-sm font-medium">{title}</p></div>
    <PayStatus status={status} />
  </div>
  {items && <dl className="space-y-1.5 border-t border-dashed border-border px-4 py-3 text-sm sm:px-5">
    <div className="flex justify-between gap-4"><dt className="text-muted">{label}</dt><dd className="tabular-nums">{formatMoney(amount, currency)}</dd></div>
  </dl>}
  <div className="flex flex-wrap items-center justify-between gap-3 border-t border-border bg-background/50 p-4 sm:px-5">
    <p className="text-2xl font-semibold tracking-[-0.02em] tabular-nums">{formatMoney(amount, currency)}</p>
    {status === "due" && <a href={`/api/billing/pay/${paymentId}`} className={`${ui.button} w-full sm:w-auto`}>Оплатить <span className={ui.arrow}>→</span></a>}
    {status === "failed" && <a href={…} className={`${ui.button} w-full sm:w-auto`}>Попробовать снова</a>}
    {status === "succeeded" && receiptUrl && <a href={receiptUrl} className={ui.link + " text-sm"}>Чек</a>}
  </div>
</div>
```
`PayStatus` — бейдж `inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium`:
`due` «К оплате» `bg-spark-soft text-spark` + `Spark size-2.5`; `pending` «Проверяем оплату» `bg-background text-muted` + `Spark spin`;
`succeeded` «Оплачено» `bg-ok-soft text-ok` + галочка; `failed` «Не прошла» `bg-danger/10 text-danger`; `canceled` «Отменён» `text-muted`.
После `succeeded` световой край снимается, блок остаётся в ленте (история). Статус обновляется опросом (как сообщения).

## 8. Превью / версия — `preview`

```tsx
<article className={`${chat.block} mt-2 overflow-hidden`}>
  <a href={url} target="_blank" rel="noopener noreferrer" className="group relative block border-b border-border bg-background">
    <div className="flex h-7 items-center gap-1.5 border-b border-border px-3">{/* рамка браузера — своя */}
      <span className="size-2 rounded-full bg-border" /><span className="size-2 rounded-full bg-border" /><span className="size-2 rounded-full bg-border" />
      <span className="ml-2 truncate font-mono text-[11px] text-muted">{host(url)}</span>
    </div>
    {image ? <img src={image} alt={`Версия ${version}`} className="aspect-[16/10] w-full object-cover object-top transition-transform duration-500 group-hover:scale-[1.02]" />
           : <span className="grid aspect-[16/10] place-items-center text-muted"><Spark className="size-6 text-spark" /></span>}
    <span className="absolute bottom-3 right-3 inline-flex items-center gap-1.5 rounded-full bg-foreground px-3 py-1.5 text-xs font-medium text-background opacity-0 transition-opacity group-hover:opacity-100 group-focus-visible:opacity-100">Открыть ↗</span>
  </a>
  <div className="p-4 sm:p-5">
    <div className="flex items-center justify-between gap-3">
      <p className="text-sm font-semibold">Версия {version}</p>
      <VersionStatus status={status} />  {/* review «На согласовании» spark-soft/spark; approved «Согласовано» ok; changes «Правки» support-soft/support */}
    </div>
    {note && <p className="mt-1 text-sm text-muted">{note}</p>}
    {status === "review" && <div className="mt-4 flex flex-col gap-2 sm:flex-row">
      <button className={ui.button} onClick={approve}>Согласовать</button>
      <button className={ui.buttonGhost} onClick={openChanges}>Нужны правки</button>
    </div>}
  </div>
</article>
```
- «Согласовать» — подтверждение через `useConfirm` (`components/confirm.tsx`): «Согласовать версию 3? После этого начнём следующий этап».
- «Нужны правки» — под кнопками раскрывается `TextField multiline rows={3}` «Что поправить?» + «Отправить правки»; можно приложить файл (скрепка).
  Отправка = сообщение пользователя «Правки к версии 3: …», статус → `changes`.
- На 375 картинка во всю ширину блока, кнопки `w-full` столбиком.

## 9. Файлы

**Картинки — галерея** (в пузыре и без него — если сообщение только из картинок, пузырь не рисуется):
- 1 шт: `max-h-72 w-auto max-w-full rounded-xl`; 2 шт: `grid grid-cols-2 gap-1`; 3 шт: первая `row-span-2`; 4+: сетка 2×2, на последней
  `+N` поверх — `absolute inset-0 grid place-items-center bg-foreground/55 text-lg font-semibold text-background`.
  Ячейки `aspect-square object-cover rounded-lg`, общий контейнер `overflow-hidden rounded-xl max-w-[22rem]`.
- По нажатию — свой просмотрщик (`components/chat/lightbox.tsx`): `<dialog>` с `showModal()` (фокус и Esc из коробки), фон
  `backdrop:bg-foreground/80 backdrop:backdrop-blur-sm`, картинка `max-h-[85vh] rounded-xl`, кнопки ← → и ✕ — `size-11 rounded-full bg-card/90`,
  подпись снизу `font-mono text-xs` «2 / 5 · photo.jpg», свайп на телефоне (pointer events, порог 50px), стрелки клавиатуры.
- **Видео** — без нативных `controls`: превью-кадр + своя кнопка ▶ `size-12 rounded-full bg-card/90` по центру; по нажатию открывается в
  том же просмотрщике, плеер — `<video>` с нашей панелью: play/pause, полоса (`h-1 rounded-full bg-background/30`, заполнение `bg-spark`), время `tabular-nums`, звук.

**Документы:**
```tsx
<a href={`/api/files/${f.id}`} target="_blank" rel="noreferrer"
   className="group flex w-full max-w-80 items-center gap-3 rounded-xl border border-border bg-background p-2.5 pr-3 transition-colors hover:border-foreground/30">
  <span className="grid size-10 shrink-0 place-items-center rounded-lg bg-card font-mono text-[10px] font-semibold uppercase text-muted">{ext}</span>
  <span className="min-w-0 flex-1"><span className="block truncate text-sm font-medium">{f.fileName}</span><span className="text-xs text-muted tabular-nums">{size} · PDF</span></span>
  <DownloadIcon className="size-4 text-muted group-hover:text-foreground" />
</a>
```
В пузыре пользователя — `border-background/20 bg-background/10`, подписи `text-background/70`. (`size` — добавить в `FeedFile` при наличии.)

## 10. Этап / прогресс — `progress`

```tsx
<div className={`${chat.block} mt-2 p-4 sm:p-5`}>
  <p className={chat.meta}>Этап {stage + 1} из {steps.length}</p>
  <ol className="mt-3 flex items-center gap-1.5">
    {steps.map((s, i) => <li key={s} className="flex-1"><span className={`block h-1.5 rounded-full ${i < stage ? "bg-foreground" : i === stage ? "bg-spark" : "bg-border"}`} /></li>)}
  </ol>
  <div className="mt-2 hidden justify-between text-xs text-muted sm:flex">{steps.map((s, i) => <span key={s} className={i === stage ? "font-medium text-foreground" : ""}>{s}</span>)}</div>
  <p className="mt-2 text-sm sm:hidden"><span className="font-medium">{steps[stage]}</span>{note && <span className="text-muted"> · {note}</span>}</p>
</div>
```
`steps` по умолчанию — `STAGES` из `ui.ts`. Активный этап — искра-цвет (единственная искра блока).

## 11. Системные сообщения

По `meta.event`, по центру ленты, без пузыря:
```tsx
<li className="my-3 flex justify-center">
  <span className="inline-flex max-w-full items-center gap-2 rounded-full border border-border bg-background px-3 py-1.5 text-xs text-muted">
    <EventIcon event={e} className="size-3.5" />
    <span className="truncate">{content}</span>
    {action && <a href={action.href} className="font-medium text-foreground underline underline-offset-2">{action.label}</a>}
  </span>
</li>
```
| event | иконка / цвет | текст (пример) | действие |
|---|---|---|---|
| `stage_changed` | `Spark text-spark` | «Новый этап: Вёрстка» | — |
| `subscription_started` / `renewed` | галочка `text-ok` | «Подписка «Старт» активна до 24 ноября» | — |
| `subscription_expired` | `!` `text-danger`, чип `border-danger/30` | «Подписка закончилась» | «Продлить» → `/cabinet/billing` |
| `limit_low` | `text-support`, чип `bg-support-soft border-support/30` | «Осталось 10% лимита» | «Пополнить» |
| `handoff_admin` | аватар админа | «К чату подключилась Анна из поддержки» | — |
| `handoff_agent` | `Spark` | «Снова отвечает Lumen» | — |
| `version_published` | — | не системное: рисуется блоком `preview` | — |
Без `event` — нейтральный чип без иконки (как сейчас, но в чипе).

## 12. Статусы — `components/chat/status.tsx`

Место — сразу после последнего сообщения, слева, с аватаром агента (как новая группа).
- **Печатает** (пришёл первый `delta` ещё нет, `status` пустой):
  `<div className="spark-typing inline-flex gap-1 rounded-2xl rounded-bl-md border border-border bg-card px-3.5 py-3 text-spark"><Spark className="size-2.5"/>×3</div>`
  + `<span className="sr-only">Lumen печатает</span>`.
- **Думает** (`status` из потока — «Смотрю референс…», «Готовлю варианты…»): тот же пузырь, внутри
  `<Spark className="spin size-3 text-spark" /> <span className="think-text text-sm">{status}</span>`; после 8 с без событий добавить
  `text-xs text-muted` «Это может занять до минуты». Смена текста статуса — fade 150 мс.
- **Отвечает человек** — плашка над лентой (вместо текущей жёлтой полосы):
```tsx
<div className="flex items-center gap-3 border-b border-support/30 bg-support-soft px-4 py-2.5 text-sm">
  <span className="grid size-7 place-items-center rounded-full bg-card text-xs font-semibold text-support">А</span>
  <p className="min-w-0 flex-1"><span className="font-medium">Анна, поддержка</span> <span className="text-muted">· отвечает вместо Lumen</span></p>
  <span className="hidden text-xs text-muted sm:block">обычно в течение 15 минут</span>
</div>
```
  Когда агент — плашки нет; в шапке чата (кабинет, над лентой, `h-12 border-b`) — «Lumen» + точка `size-2 rounded-full bg-ok` «на связи»
  либо `bg-support` «отвечает человек». Админ печатает — тот же индикатор «печатает», но точки `text-support` и подпись «Анна печатает».
- **Статус отправки своего сообщения** (под пузырём, справа): отправляется — `Spark spin size-3 text-muted`; доставлено — галочка `text-muted`;
  **ошибка** — пузырь `opacity-70`, под ним:
```tsx
<p className="mt-1 flex items-center justify-end gap-2 text-xs text-danger">
  Не отправлено
  <button onClick={() => retry(m)} className="inline-flex h-7 items-center gap-1 rounded-full border border-danger/30 px-2.5 font-medium hover:bg-danger/10">↻ Повторить</button>
  <button onClick={() => discard(m)} aria-label="Удалить" className="grid size-7 place-items-center rounded-full text-muted hover:bg-card">✕</button>
</p>
```
  Сообщение и вложения остаются в ленте локально (`local-…` со статусом `failed`), повтор — тем же `send` без новой копии.
  Ошибка агента после сохранения (сообщение ушло, ответа нет) — системный чип `text-danger`: «Lumen не ответил — администратор уже в курсе» + «Повторить запрос».

## 13. Пустой чат — `components/chat/empty.tsx`

```tsx
<div className="mx-auto flex max-w-xl flex-1 flex-col items-center justify-center px-4 py-12 text-center">
  <span className="grid size-14 place-items-center rounded-2xl bg-foreground text-spark"><Spark className="spark-twinkle size-6" /></span>
  <h2 className="mt-5 text-xl font-semibold tracking-[-0.02em] sm:text-2xl">Расскажите, какой сайт нужен</h2>
  <p className="mt-2 text-sm text-muted">Lumen задаст пару вопросов и соберёт первую версию. Можно приложить фото, логотип или ссылку на сайт-пример.</p>
  <ul className="mt-8 grid w-full gap-2 sm:grid-cols-2">
    {starters.map(s => <li key={s.t}><button onClick={() => send(s.t)}
      className="group flex w-full items-start gap-3 rounded-2xl border border-border bg-card p-4 text-left transition-[border-color,transform] hover:-translate-y-0.5 hover:border-foreground/30 focus-visible:ring-2 focus-visible:ring-brand">
      <StarterIcon name={s.i} className="mt-0.5 size-4 text-muted group-hover:text-spark" />
      <span><span className="block text-sm font-medium">{s.t}</span><span className="text-xs text-muted">{s.d}</span></span>
    </button></li>)}
  </ul>
</div>
```
Стартеры (тексты — `[УТОЧНИТЬ]` у копирайтера, черновик): «Лендинг для услуги» / «одна страница с заявкой»; «Интернет-магазин» / «каталог и корзина»;
«Сайт по примеру» / «пришлю ссылку, сделаем похоже»; «Переделать мой сайт» / «есть сайт, хочу современнее». На 375 — 1 колонка, 4 штуки; на 1440 — 2×2.

## 14. Поле ввода — `components/chat/composer.tsx`

Каркас — текущий (скрепка, `textarea`, «Отправить»), меняется:
- `textarea`: авто-высота (`field-sizing: content` + JS-фолбэк по `scrollHeight`), `min-h-10 max-h-40`, Enter — отправить, Shift+Enter — новая строка,
  при IME-вводе (`e.nativeEvent.isComposing`) Enter не отправляет. На телефоне (`(pointer: coarse)`) Enter = новая строка, отправка кнопкой.
- Кнопка отправки на 375 — круглая иконка `size-10 rounded-xl` со стрелкой ↑ (`aria-label="Отправить"`), с `sm:` — текст «Отправить».
- Подсказка под полем, только ≥ 640 и когда поле в фокусе: `text-[11px] text-muted` «Enter — отправить · Shift+Enter — новая строка».
- **Перетаскивание:** зона — **весь чат** (лента + поле), не только форма. Оверлей — текущий (рамка `border-dashed border-spark`), текст
  «Отпустите, чтобы прикрепить» + `text-xs text-muted` «Фото, видео, PDF · до 10 файлов». Вставка картинки из буфера (`onPaste`) — тоже вложение.
- Вложения перед отправкой: картинки — миниатюры `size-14 rounded-lg object-cover` с ✕ в углу (`absolute -right-1.5 -top-1.5 size-5 rounded-full bg-foreground text-background`);
  остальное — текущие чипы. Загрузка — на миниатюре полоса прогресса `absolute inset-x-1 bottom-1 h-1 rounded-full bg-background/60` с заливкой `bg-spark`.
- Пока активен блок `form` или `cards` — placeholder «Или напишите своими словами…» (поле не блокируется).
- Агент отвечает — поле доступно, кнопка «Отправить» → «Стоп» (`ui.buttonGhost`, квадратик 10px) — **если бэкенд умеет прерывать**, иначе disabled (спорное, п.19).
- Подписка неактивна / лимит 0 — поле заменяется плашкой `chat.block p-4`: «Лимит закончился» + `ui.button` «Пополнить».

## 15. Админка — чат пользователя (`admin-chat.tsx`)

Раскладка 1440: `grid grid-cols-[1fr_320px] gap-6` — слева чат (`ui.card p-0 h-[calc(100vh-10rem)]`), справа **панель сессии** (`sticky top-6`).
768–1279: панель над чатом, свёрнута в одну строку (статус + кнопка), раскрывается. 375: админка — не основной сценарий, но всё в одну колонку,
панель — строка-аккордеон.
Лента — тот же `MessageBubble viewer="admin"`: справа — агент и поддержка, слева — заказчик; блоки рисуются **как видит заказчик, но неактивными**,
с ответом заказчика поверх (`answers`). Подпись у пузыря агента/админа — «Lumen» / «Анна».

### 15.1 Панель сессии
```tsx
<aside className={`${ui.card} space-y-5`}>
  <div className="flex items-center justify-between">
    <p className={ui.h2}>Сессия</p>
    <SessionBadge status={status} />   {/* active: «Работает» ok · paused: «Ведёт человек» support · error: «Ошибка» danger · closed: «Закрыта» muted; бейдж как PayStatus */}
  </div>
  <div>
    <p className="text-xs text-muted">Кто ведёт</p>
    <div className="mt-2 grid grid-cols-2 gap-1 rounded-full border border-border p-1" role="radiogroup" aria-label="Кто ведёт">
      <button role="radio" aria-checked={!paused} className="h-9 rounded-full text-sm font-medium aria-checked:bg-foreground aria-checked:text-background inline-flex items-center justify-center gap-1.5"><Spark className="size-3 text-spark"/>Агент</button>
      <button role="radio" aria-checked={paused} className="h-9 rounded-full text-sm font-medium aria-checked:bg-foreground aria-checked:text-background">Я</button>
    </div>
    <p className="mt-2 text-xs text-muted">{paused ? "Агент молчит, отвечаете вы" : "Ответ администратора перехватит чат"}</p>
  </div>
  <dl className="grid grid-cols-2 gap-3 text-sm">
    <div><dt className="text-xs text-muted">Токены за сессию</dt><dd className="font-medium tabular-nums">{formatTokens(used)}</dd></div>
    <div><dt className="text-xs text-muted">Остаток лимита</dt><dd className="font-medium tabular-nums">{formatTokens(left)}</dd></div>
    <div className="col-span-2"><div className="h-1.5 rounded-full bg-border"><div className="h-full rounded-full bg-foreground" style={{ width: `${pct}%` }} /></div></div>
    <div><dt className="text-xs text-muted">Этап</dt><dd>{STAGES[stage]}</dd></div>
    <div><dt className="text-xs text-muted">Последний ответ</dt><dd className="tabular-nums">{formatDateTime(lastAt)}</dd></div>
  </dl>
  {status === "error" && <FormAlert message={lastError} />}   {/* + ui.buttonGhost «Перезапустить агента» */}
  <div className="flex flex-wrap gap-2 border-t border-border pt-4">
    <a href={`/admin/sessions/${sessionId}`} className={ui.textButton}>Журнал сессии</a>
    <button className={ui.textButton}>Закрыть сессию</button>  {/* через useConfirm */}
  </div>
</aside>
```
- **«Перехватить»** = переключатель на «Я» (`setAgentPausedAction(userId, true)`), без подтверждения; заказчику уходит событие `handoff_admin`.
  **«Вернуть агенту»** = «Агент», с `useConfirm`: «Вернуть чат агенту? Он увидит всю переписку, включая ваши ответы».
- Если администратор начал печатать при «Агент» — над полем плашка `bg-support-soft text-sm` «Отправка перехватит чат у агента» (не блокирует).

### 15.2 Конструктор ответа
Поле ввода админа = composer (п.14) + панель инструментов над `textarea`:
```tsx
<div className="flex items-center gap-1 border-b border-border px-2 py-1.5">
  <ToolButton icon="template" label="Шаблоны" />        {/* открывает список шаблонов (текущий picker) */}
  <ToolButton icon="chips" label="Кнопки ответа" />
  <ToolButton icon="cards" label="Карточки" />
  <ToolButton icon="form" label="Форма" />
  <ToolButton icon="pay" label="Счёт" />
  <ToolButton icon="preview" label="Версия" />
  <span className="ml-auto text-[11px] text-muted">**жирный** · [ссылка](url) · - список</span>   {/* только ≥1280 */}
</div>
```
`ToolButton`: `grid size-8 place-items-center rounded-lg text-muted hover:bg-background hover:text-foreground aria-pressed:bg-background aria-pressed:text-foreground` + `data-tip`, `aria-label`.

Добавленный блок появляется **между панелью и текстом** как редактируемый черновик (`rounded-xl border border-dashed border-border bg-background p-3`), заголовок
`chat.meta` «Кнопки ответа» + ✕ «убрать блок». Внутри:
- **Кнопки:** список строк-чипов, каждая — инлайн-поле `h-9 rounded-full border px-3 text-sm` + ✕; «+ вариант» (`chat.chip border-dashed`);
  переключатели (`Switch`): «Можно несколько», «Свой вариант». Перетаскивание порядка — ручка ⋮⋮ слева (pointer events).
- **Карточки:** строки «заголовок / подпись / картинка (FilePicker или из вложений) / иконка (Select) / Рекомендуем (Switch, только одна)»; до 6 карточек.
- **Форма:** строки «подпись поля + тип (Select: текст, почта, телефон, код из СМС, выбор) + обязательное (Checkbox)»; текст кнопки отправки.
- **Счёт:** Select тариф/пакет или «своя сумма» (TextField `inputMode="decimal"`), назначение; создаёт платёж при отправке.
- **Версия:** Select проекта, № версии подставляется, URL превью из проекта, скриншот (FilePicker), заметка.
Справа от черновика на ≥1280 — **живое превью** «Так увидит заказчик» (`w-72`, масштаб 1:1, тот же компонент блока, `inert`). На меньших — кнопка «Предпросмотр».
Отправка — `ui.button` «Отправить» (как у заказчика); без текста, но с блоком — можно.

### 15.3 Шаблоны
Текущий picker (кнопка + меню) остаётся, но меню — `w-80 rounded-2xl border bg-card p-1.5 shadow-xl`:
поиск сверху (`ui.input h-9 text-sm`), список `max-h-72 overflow-y-auto`, пункт — `rounded-xl px-3 py-2 hover:bg-background aria-selected:bg-background`
с названием `text-sm font-medium` и первой строкой тела `text-xs text-muted truncate`; ↑↓ Enter Esc; `/` в пустом поле открывает меню.
Шаблон может содержать блок (в `ReplyTemplate` — `blocks?: ChatBlock[]`) — тогда вставляется и текст, и черновик блока; у пункта метка `chat.meta` «+ кнопки».
Внизу меню — `ui.textButton` «Управлять шаблонами →» (`/admin/templates`). Подстановки `{имя}`, `{тариф}` — подсвечиваются в поле `text-brand`.

## 16. Раздел «Сессии» — `app/admin/(panel)/sessions/page.tsx` (новый)

Пункт меню «Сессии» после «Чаты». Страница: `ui.h1` «Сессии» + фильтры-чипы (`chat.chip`, `aria-pressed`): Все · Работают · Ведёт человек · Ошибка · Ждут ответа.
Таблица `ui.table` на ≥ 768, на 375 — карточки `ui.card p-4` списком.
Колонки: Заказчик (имя + почта `text-xs text-muted`) · Статус (`SessionBadge`) · Кто ведёт (Lumen / имя) · Последнее сообщение (обрезано, `max-w-72 truncate`) ·
Токены (`tabular-nums`, за сессию) · Обновлено (`formatDateTime`) · действие `ui.textButton` «Открыть →» (`/admin/users/[id]`).
Строка «ждёт ответа человека» (последнее — от заказчика, сессия на паузе) — слева полоса `shadow-[inset_3px_0_0_var(--support)]`.
Пусто: по центру `Spark` + «Сессий с такими условиями нет».
Обновление — опрос раз в 10 с (как чат), без перезагрузки страницы.

## 17. Свет / тьма

Все цвета — токены, отдельных правил почти нет. Проверить:
- Пузырь пользователя `bg-foreground text-background` — в тёмной светлый пузырь с тёмным текстом (как сейчас; так и задумано).
- Картинки в тёмной — `ring-1 ring-inset ring-white/5`, чтобы не сливались края.
- `bg-ok-soft/text-ok`, `bg-spark-soft/text-spark`, `bg-support-soft/text-support` — ≥ 4.5:1 в обеих темах (посчитано для spark/support в v3, ok — п.0).
- Тени в тёмной не видны — карточкам выбора в тёмной при hover добавляется `dark:border-foreground/40` (через `[data-theme=dark]_&` или общий класс).

## 18. 375 / 1440

| | 375 | 1440 |
|---|---|---|
| Лента | `px-4`, пузырь `max-w-[85%]` | колонка `max-w-2xl` по центру, пузырь `max-w-[78%]` |
| Блоки | во всю ширину колонки, кнопки `w-full` | `max-w-[34rem]`, кнопки по содержимому |
| Чипы ответов | перенос / лента со скроллом при ≥ 5 | перенос |
| Карточки | 2 колонки | до 3 колонок |
| Отправить | иконка ↑ | «Отправить» + подсказка Enter |
| Поле ввода | прилипает к низу, `pb-[env(safe-area-inset-bottom)]`; при клавиатуре — `100dvh` у контейнера чата | как сейчас |
| Админка | одна колонка, панель-аккордеон | чат + панель 320px |

## 19. Спорные места / нужно от бэкенда

1. `meta.blocks`, `meta.answers`, `meta.author`, `reply` в `POST /api/chat` — нет в API (`models.go`: только `buttons`). Без этого фронт делает только
   быстрые ответы (уже есть) и визуал статусов/файлов/markdown. Агенту нужен инструмент «показать блок» — вопрос бэкенду/менеджеру.
2. «Стоп» генерации — нужен эндпоинт прерывания; иначе кнопка disabled.
3. Размер файла (`FeedFile.size`) для документов — добавить в API, иначе не показываем.
4. Токены за сессию и `lastError` для панели — проверить, отдаёт ли `/api/admin/users/[id]`.
5. Счёт из чата (админ создаёт платёж) — нужен эндпоинт, и решение заказчика, можно ли админу выставлять «свою сумму».

## 20. Проверка перед сдачей (фронтенд + QA)
- [ ] Каждый тип из п.3–13 — в обеих темах, на 375 и 1440, скриншоты в `design/approved/chat/`.
- [ ] Клавиатура: Tab по чипам/карточкам, Space/Enter выбирает, Esc закрывает «свой вариант»/просмотрщик/шаблоны; фокус-кольцо `ring-brand` везде.
- [ ] Экранный диктор: лента `role="log"`, «печатает» объявляется, карточки — radio/checkbox.
- [ ] `prefers-reduced-motion`: нет мерцания, вращения, покачивания, свипа.
- [ ] Нет ни одного нативного вида: select, title, alert/confirm, `<video controls>`, спиннер, полоса прогресса `<progress>`.
- [ ] Свёрнутый итог формы не содержит кодов/паролей.

## 21. Новое — на одобрение заказчику
1. Карточки выбора с картинками и меткой «Рекомендуем» со световым краем.
2. Форма прямо в чате с кодом из СМС по ячейкам.
3. Оплата и согласование версии прямо в переписке (кнопки «Оплатить», «Согласовать» / «Нужны правки»).
4. Свой просмотрщик фото/видео со свайпом.
5. Пустой чат со стартерами (4 подсказки).
6. «Думает» с бегущим бликом по тексту статуса.
7. Админка: живое превью ответа «Так увидит заказчик» и шаблоны с блоками.
8. Раздел «Сессии» с фильтрами и подсветкой «ждёт ответа человека».
