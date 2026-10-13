# Перестройка 2026-10-13: куда переехала память

Всё, что было в `prod/` до перестройки, перенесено. Старые файлы — в истории git (коммит до перестройки).

| Было | Стало |
|---|---|
| `prod/CLAUDE.md` — кто ты, принципы, этапы, папки, стек, безопасность, когда спросить | `prod/CLAUDE.md` (менеджер, команда, маршруты, этапы); принципы — `common/standards.md`; стек — `crew/dev/tech-lead/`; безопасность — `common/security.md` |
| `knowledge/00-index.md` | заменён: таблица команды в `CLAUDE.md` + `MEMORY.md` каждой роли |
| `knowledge/01-brief.md` | `crew/management/manager/brief.md` |
| `knowledge/02-structure.md` | `crew/client/analyst/MEMORY.md` (+ размещение кнопок — `crew/design/ux-designer/`) |
| `knowledge/03-design.md` — референсы, стиль, эскиз кодом, уровень примера | `crew/design/art-director/MEMORY.md` |
| — Figma | `crew/design/ui-designer/figma.md` |
| — типографика, цвет, сетка, чего избегать | `crew/design/ui-designer/MEMORY.md` |
| — изображения и видео | `crew/design/photo-editor/MEMORY.md` |
| — как показывать результат | `crew/quality/qa-lead/MEMORY.md`, `crew/quality/qa/MEMORY.md` |
| `knowledge/04-build.md` — порядок, адаптив, скорость, формы, видео WebM+MP4 | `crew/dev/frontend/MEMORY.md` |
| — правила кода | `crew/dev/tech-lead/MEMORY.md` |
| — SEO | `crew/content/seo/MEMORY.md` (+ frontend) |
| — доступность, скорость (проверка) | `crew/quality/performance/MEMORY.md` |
| `knowledge/05-copywriting.md` | `crew/content/copywriter/MEMORY.md` (+ приёмка — `chief-editor`) |
| `knowledge/06-revisions.md` | `crew/management/manager/revisions.md`; этап 9 — `lab/parked/README.md`; «сигналы для памяти» — `lab/crew/README.md` |
| `knowledge/07-launch.md` | `lab/parked/launch.md` (целиком); чек-лист сдачи — `crew/quality/qa-lead/`; передача — `crew/management/manager/delivery.md` |
| `knowledge/08-client-communication.md` | `crew/management/manager/communication.md`; строки про сервисы — `lab/parked/README.md` |
| `knowledge/09-security.md` | `common/security.md`; коды и оплата — `lab/parked/services.md` |
| `knowledge/10-existing-project.md` | разбор — `crew/audit/auditor/`; перенос — `crew/dev/migration/`; доступы, хостинг, DNS — `lab/parked/README.md` |
| `knowledge/11-services.md` | `lab/parked/services.md` (целиком) |
| `knowledge/mistakes.md` | `common/mistakes.md` (+ чья зона у каждой ошибки) |
| `knowledge/niches/` | `prod/niches/` (без изменений) |
| `figma/README.md`, `figma/starter.md` | `crew/design/ui-designer/figma.md` |
| skill `self-review` | `crew/quality/qa/MEMORY.md` (чек-лист) + `crew/management/manager/acceptance.md` |
| skill `new-project` | обновлён под новую папку проекта |
| skill `show-result` | без изменений (выполняет `qa`) |
| `lab/owner.md` | `lab/talk/owner.md` (+ решения 2026-10-13) |
| `lab/inbox/owner.md` | `lab/archive/inbox-owner-2026-10.md` (все записи уже в проде) |
| `lab/inbox/README.md` | `lab/crew/README.md` |

## Что изменено по смыслу (из-за новых рамок)
- Первое сообщение заказчику: шаги «зарегистрирую в сервисах / запущу на адресе» заменены на «передам готовый сайт».
- Бриф: вопрос «куда отправлять заявки» → «как клиентам удобнее связаться»; «домен/хостинг» → «есть ли сайт или его части».
- Figma — по желанию; основной путь — эскиз кодом (вопрос владельцу — `lab/talk/questions.md`).
- Формы — только если есть, куда отправлять; по умолчанию кнопки звонка и мессенджеров.
- Тестовые клиенты 01 и 04 — проверки про адрес сайта и коды переписаны под новые рамки.
