# Design spec — «Студия ИИ», редизайн (v1, на показ)

**Направление:** «тихий редактор». Тёплая светлая бумага, почти чёрный текст, один акцент (синий чернильный).
Много воздуха, тонкие линии вместо теней, крупная плотная типографика. Главная кнопка — чёрная (в тёмной теме — светлая), акцент
только на деталях: фокус, рекомендуемый тариф, номера шагов, свои сообщения в чате.
**Две детали стиля (и всё):** 1) моно-подписи `01 / Знакомство`, `Тарифы — 02` (номера разделов и шагов); 2) волосяные линии 1px,
делящие блоки, вместо карточек-«плиток» на главной. Анимаций нет, кроме `transition-colors` 150ms.
Тексты, тарифы, порядок блоков, логика — без изменений. Меняются только `className`, `globals.css`, `ui.ts`, `layout.tsx` (шрифты).

## 1. Токены — `app/globals.css` (заменить целиком)
```css
@import "tailwindcss";

:root {
  --background: #f7f6f3;   /* тёплая бумага */
  --foreground: #121212;   /* чернила */
  --muted: #6b6a66;        /* вторичный текст, AA на фоне */
  --card: #ffffff;
  --border: #e4e2dc;       /* волосяная линия */
  --brand: #2f45d8;        /* акцент: чернильный синий */
  --brand-soft: #eceffd;
  --support: #a35a00;
  --support-soft: #fbf3e6;
  --danger: #c2341b;
}
@media (prefers-color-scheme: dark) {
  :root {
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
  }
}
@theme inline {
  --color-background: var(--background);
  --color-foreground: var(--foreground);
  --color-muted: var(--muted);
  --color-card: var(--card);
  --color-border: var(--border);
  --color-brand: var(--brand);
  --color-brand-soft: var(--brand-soft);
  --color-support: var(--support);
  --color-support-soft: var(--support-soft);
  --color-danger: var(--danger);
  --font-sans: var(--font-onest);
  --font-mono: var(--font-mono-jb);
}
body { background: var(--background); color: var(--foreground); }
::selection { background: var(--brand); color: #fff; }
:focus-visible { outline: 2px solid var(--brand); outline-offset: 2px; }
```
Акцент `--brand` в светлой теме — с белым текстом контраст > 7:1; в тёмной на `brand` текст ставить `text-background` (см. чат).

## 2. Шрифты — `app/layout.tsx` (next/font/google, кириллица)
- Текст и заголовки: **Onest** (`subsets: ["latin","cyrillic"]`, `variable: "--font-onest"`, веса 400/500/600/700).
- Моно-подписи: **JetBrains Mono** (`subsets: ["latin","cyrillic"]`, `variable: "--font-mono-jb"`, вес 400/500).
- `<html className={`${onest.variable} ${mono.variable} h-full antialiased`}>`; Inter убрать.
- Шкала: H1 главной `text-[40px] leading-[1.05] sm:text-6xl lg:text-[76px] font-semibold tracking-[-0.03em]`;
  H2 разделов `text-3xl sm:text-4xl font-semibold tracking-[-0.02em]`; текст `text-base`/`text-lg`, `leading-relaxed`;
  моно-подпись `font-mono text-xs uppercase tracking-[0.08em] text-muted`.

## 3. Сетка и отступы
- Контейнер: `mx-auto w-full max-w-6xl px-5 sm:px-8` (375 → 20px поля, 768 → 32px, 1440 → контент 1152px по центру).
- Вертикальный ритм разделов: `py-16 sm:py-24`; между разделами — `border-t border-border` (линия на всю ширину контейнера).
- Радиусы: поля и кнопки `rounded-full` (кнопки) / `rounded-xl` (поля, textarea); карточки `rounded-2xl`. Теней нет.

## 4. Классы — `components/ui.ts` (заменить объект `ui`, остальное не трогать)
```ts
export const ui = {
  card: "rounded-2xl border border-border bg-card p-5 sm:p-6",
  input:
    "w-full rounded-xl border border-border bg-card px-4 py-3 text-base outline-none transition-colors placeholder:text-muted/70 hover:border-foreground/30 focus:border-brand focus:ring-4 focus:ring-brand/10 aria-[invalid=true]:border-danger",
  button:
    "inline-flex h-11 items-center justify-center gap-2 rounded-full bg-foreground px-5 text-sm font-medium text-background transition-colors hover:bg-foreground/85 active:bg-foreground/75 disabled:opacity-40",
  buttonGhost:
    "inline-flex h-11 items-center justify-center gap-2 rounded-full border border-border px-5 text-sm font-medium transition-colors hover:border-foreground/40 hover:bg-card disabled:opacity-40",
  buttonDanger:
    "inline-flex h-11 items-center justify-center gap-2 rounded-full border border-danger/40 px-5 text-sm font-medium text-danger transition-colors hover:bg-danger/10 disabled:opacity-40",
  label: "mb-1.5 block text-sm font-medium",
  h1: "text-2xl font-semibold tracking-[-0.02em] sm:text-3xl",
  h2: "font-mono text-xs uppercase tracking-[0.08em] text-muted",
  table: "w-full text-left text-sm [&_td]:px-3 [&_td]:py-3 [&_th]:px-3 [&_th]:py-2 [&_th]:font-mono [&_th]:text-xs [&_th]:font-normal [&_th]:uppercase [&_th]:tracking-[0.08em] [&_th]:text-muted [&_tr]:border-b [&_tr]:border-border",
  eyebrow: "font-mono text-xs uppercase tracking-[0.08em] text-muted",
  link: "underline decoration-border underline-offset-4 transition-colors hover:decoration-foreground",
};
```
Ошибки форм: везде `text-red-600` → `text-danger`. Админка подхватывает стиль через `ui` без отдельной работы.

## 5. Главная (`app/page.tsx`) — по блокам, тексты те же
1. **Шапка:** контейнер, `h-16 flex items-center justify-between`. Логотип — текст `SITE_NAME`, `font-semibold tracking-[-0.02em]`,
   перед ним квадрат 8×8 `bg-brand rounded-[2px]` (единственный «знак»). «Тарифы» — `text-sm text-muted hover:text-foreground`
   (скрыт < 640, как сейчас). «Войти» — `ui.button` с `h-9 px-4`. Шапка `sticky top-0 z-10 bg-background/85 backdrop-blur border-b border-border`.
2. **Первый экран:** `pt-16 pb-20 sm:pt-28 sm:pb-28`. Плашку «Сайт под ключ с ИИ-агентом» заменить на `ui.eyebrow` с точкой
   `bg-brand` 6px слева (без заливки). H1 — шкала из п.2, `max-w-[14ch]` (на 1440 — 2–3 строки). Подзаголовок `mt-6 max-w-xl text-lg text-muted leading-relaxed`.
   Кнопки `mt-10`: «Начать проект» — `ui.button h-12 px-6 text-base` + стрелка `→`; «Как это работает» — `ui.buttonGhost h-12 px-6 text-base`.
   375: кнопки на всю ширину столбиком (`flex-col sm:flex-row`, `w-full sm:w-auto`); заголовок и кнопка — в первом экране.
3. **Как это работает:** `border-t`. Сверху `ui.eyebrow` «01», затем H2. Шаги — без карточек: `grid sm:grid-cols-2 lg:grid-cols-4`,
   каждый `border-t border-border pt-5` (на lg — ещё `lg:border-l lg:pl-6 lg:first:border-l-0 lg:first:pl-0`, без верхней).
   «Шаг {i+1}» → моно `0{i+1}` в `text-brand`; заголовок шага `mt-6 text-lg font-semibold`; текст `mt-2 text-muted`.
4. **Тарифы:** `border-t`, `ui.eyebrow` «02», H2, подзаголовок `text-muted`. Карточки `ui.card flex flex-col p-6 sm:p-8`;
   рекомендуемая (i===1) — `border-foreground` + над названием моно-метка `text-brand` «●» (без нового текста).
   Цена `mt-8 text-4xl font-semibold tracking-[-0.03em]`, «/ N дн.» `text-base font-normal text-muted`. Лимит — `text-sm text-muted`,
   над ним `border-t border-border pt-4 mt-6`. Кнопка «Выбрать» `mt-auto` (отступ сверху 8): i===1 → `ui.button w-full`, иначе `ui.buttonGhost w-full`.
   375: одна колонка; 768: одна колонка (карточки шире, удобно); ≥1024 (`lg:grid-cols-3`) — три.
   Список преимуществ: `mt-12 grid sm:grid-cols-2 gap-x-8`, каждый пункт `border-t border-border py-4 flex gap-3`; «✓» → `text-brand`.
5. **Подвал:** `border-t`, контейнер, `py-8 text-sm text-muted`, слева `© SITE_NAME`, справа ссылки `ui.link`.

## 6. Вход (`/login`)
Фон страницы — `bg-background`; центр `max-w-[400px]`. Логотип как в шапке, `mb-10`, по центру. Карточка `ui.card p-6 sm:p-8`.
H1 «Вход и регистрация» `text-2xl font-semibold tracking-[-0.02em]`, подпись `mt-1 mb-8 text-muted`. Поле и кнопка — `ui.input`, `ui.button w-full h-12`.
Поле кода: `ui.input text-center font-mono text-2xl tracking-[0.4em]`. Чекбокс согласия `accent-[var(--brand)] size-4`, текст `text-sm text-muted`, ссылки `ui.link`.
Уведомление (notice) — `rounded-xl bg-brand-soft px-4 py-3 text-sm`; ошибка — `text-sm text-danger`. На 375 карточка без рамки допускается: `border-0 bg-transparent p-0 sm:border sm:bg-card sm:p-8`.

## 7. Кабинет — шапка и чат
- **Шапка:** `h-14 border-b border-border bg-background px-4 sm:px-6 flex items-center gap-6`. Логотип как на главной.
  Навигация — подчёркивание вместо плашек: пункт `relative h-14 inline-flex items-center text-sm text-muted hover:text-foreground`,
  активный — `text-foreground` + `after:absolute after:inset-x-0 after:bottom-0 after:h-px after:bg-foreground` (активность по `usePathname`
  в маленьком клиентском компоненте `NavLink`; если не хочется — оставить без активного состояния, это допустимо). «Админка» — `text-brand`.
  Email `hidden md:inline text-sm text-muted`; «Выйти» — `text-sm text-muted hover:text-foreground`. 375: навигация скроллится по горизонтали, `gap-5`.
- **Лента:** `max-w-2xl`, `space-y-5`, `py-8`. Пузыри (`message-bubble.tsx`): радиус `rounded-2xl`, `px-4 py-3 leading-relaxed`;
  свои — `bg-foreground text-background rounded-br-md`; агент — без рамки, `bg-card border border-border rounded-bl-md`;
  поддержка — как сейчас (support-токены), подпись «Поддержка» моно `text-[11px] uppercase tracking-[0.08em]`.
  Системные — `font-mono text-xs text-muted`. Индикатор «печатает» — точка `bg-brand` как сейчас.
  Быстрые ответы (чипы) — `rounded-full border border-border px-3.5 py-1.5 text-sm hover:border-foreground/40 hover:bg-card`.
- **Поле ввода:** панель `border-t border-border bg-background/90 backdrop-blur p-3 sm:p-4`. Внутри одна «капсула»:
  `rounded-2xl border border-border bg-card p-1.5 flex items-end gap-1.5 focus-within:border-brand focus-within:ring-4 focus-within:ring-brand/10`;
  скрепка — `size-10 rounded-xl text-muted hover:bg-background hover:text-foreground` (без рамки);
  textarea — `flex-1 bg-transparent px-2 py-2.5 outline-none resize-none min-h-10 max-h-40`; «Отправить» — `ui.button h-10 rounded-xl px-4`.
  Прикреплённые файлы — чипы `rounded-full bg-background border border-border px-3 py-1 text-sm`.
- Оплата/файлы/профиль: контейнер `max-w-3xl px-5 py-8 sm:px-8`, заголовки через `ui.h1`, разделы через `ui.h2` (моно-подпись), карточки `ui.card`.

## 8. Ширины — что отличается
| | 375 | 768 | 1440 |
|---|---|---|---|
| Поля контейнера | 20px | 32px | центр, 1152px |
| H1 главной | 40px | 60px | 76px |
| Кнопки героя | столбиком, во всю ширину | в ряд | в ряд |
| Шаги | 1 колонка | 2 | 4, разделены вертикальными линиями |
| Тарифы | 1 | 1 | 3 |
| Кабинет | email скрыт, навигация скроллится | email скрыт | всё в строку |

## 9. Состояния (обязательно)
hover — у всех кнопок/ссылок (см. классы); focus-visible — кольцо `brand` (глобально); active — потемнение кнопки; disabled — `opacity-40`;
ошибка поля — `aria-invalid` → рамка `danger`, текст под полем `text-sm text-danger`; загрузка кнопки — текст тот же, `disabled`; пусто (нет тарифов из API) — блок не ломается, остаётся заголовок.

## 10. Спорные места
- Главная кнопка чёрная, а не синяя: стильнее и спокойнее; если заказчик хочет цветную — замена одного класса (`bg-brand text-white`) в `ui.button`.
- Тёмная тема — автоматически по системе, переключателя нет (как сейчас).
