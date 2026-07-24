# Roadmap — InvoiceFlow

> Статус на 24 июля 2026 года. Это живой план, а не описание уже поставленных возможностей. При завершении каждого пункта нужно обновить этот файл, `docs/CURRENT_TASK.md`, `docs/API_CONTRACT.md` и, если решение меняет границы системы, `docs/DECISIONS.md`.

## Цель продукта

InvoiceFlow превращает PDF/JPG/PNG-счёт в **структурированное предложение для проверки человеком**. Оригинал остаётся доступным для сверки, правки создают неизменяемые версии, а экспорт возможен только из явно утверждённой версии.

Продукт демонстрирует надёжную обработку документов, а не бухгалтерскую систему: он не оплачивает счета, не подключается к банкам и не делает финансовые решения автоматически.

## Где проект находится сейчас

Завершены этапы 0–7. Репозиторий не прототип: Go API и worker, PostgreSQL, React/Vite-интерфейс, Docker Compose, зелёный CI (Go + integration + frontend). Проверенная сквозная цепочка: **загрузка → durable processing → извлечение → `needs_review` → неизменяемая human-review версия → утверждение точной версии → CSV / signed webhook экспорт**, интерфейс раздаётся из API (`WEB_DIR`, ADR-013) с честным лендингом и media asset из реального приложения.

Внешнее ревью 24 июля 2026 подтвердило это на живом стенде: `go test ./...`, 29 фронтовых тестов и `tsc -b` зелёные; `docker compose up --build --wait` поднимается; загрузка `fixture-cedarline-services.pdf` доходит до `needs_review` ровно с одним обещанным `subtotal_tax_total_mismatch`. Инженерные границы (durable jobs, точные деньги, неизменяемость, безопасность раздачи и вебхуков) претензий не вызвали. Всё, что осталось, — этап 8: демо-опыт и презентация репозитория.

| Область | Статус |
| --- | --- |
| Приём файлов (PDF/JPEG/PNG, лимиты, сигнатуры, SHA-256, дубликаты) | ✅ Готово |
| Надёжная обработка (PostgreSQL jobs, lease, retry/dead-letter) | ✅ Готово |
| Извлечение (bounded Poppler, OCR для изображений, строгая схема, `money-v1`, warnings) | ✅ Готово с ограничениями |
| Ревью (оригинал рядом с данными, неизменяемые версии, отклонение, audit) | ✅ Готово |
| Утверждение и экспорт (exact version, CSV, signed webhook, export jobs) | ✅ Готово |
| Раздача фронтенда из API + честный landing со scroll-историей + media asset (этап 6) | ✅ Готово |
| Release-пакет (фикстуры, скриншоты, demo script, финальные ревью, release gate) | ✅ Готово |
| Лицензия (MIT) | ✅ Готово |
| Демо на произвольном документе (не из `testdata/`) | ✅ Готово (этап 8, ADR-015) |
| Чистота корня публичного репозитория | ✅ Готово (этап 8, `.internal/`) |
| Warnings о недостающих полях, привязка warnings к полям UI, список документов | ✅ Готово (этап 8, P1) |
| Формат фронтенд-кода (Prettier + ESLint в CI) | ✅ Готово (этап 8, P2) |
| Готовность к публичному стенду (баннер, rate limit, ADR-016, руководство) | ✅ Готово (этап 8, P2) |
| Само развёртывание публичного стенда | ⏸ За владельцем — нужны аккаунт и учётные данные |

---

## ⇒ Что осталось сделать

### Этап 6 — завершён

- [x] **Реалистичные фиктивные фикстуры.** `testdata/` содержит три правдоподобных, полностью вымышленных документа, сгенерированных `scripts/gen-fixtures.py`: `fixture-aurora-stationery.pdf` (чистый text-PDF), `fixture-meridian-supplies.png` (image через OCR), `fixture-cedarline-services.pdf` (даёт ровно один `subtotal_tax_total_mismatch`). Каждая зарегистрирована в `cmd/worker` по реальному SHA-256 и маркеру; `cmd/worker/main_test.go` проверяет хеши и набор warnings через реальную нормализацию, а Compose smoke гоняет и text-PDF, и OCR-путь.
- [x] **Media asset (ADR-005).** `web/public/media/` содержит `demo.webm` (muted-скролл лендинга, ~2.3 МБ), `demo-landing-poster.png` и `demo-review.png` (review-экран с засеянным Meridian-счётом). Регенерация — `web/scripts/capture-media.mjs` (`@playwright/test`, `npm --prefix web run capture:media`) поверх изолированного засеянного Compose-демо; лендинг встраивает видео с poster и равнозначным reduced-motion/без-video путём (`DemoMedia`).
- [x] **Ревью новой границы `internal/webui`** — `code-reviewer` и `security-reviewer` прошли по раздаче статики (ADR-013). High-severity нет: traversal/symlink-escape/листинг структурно невозможны, content-type и разделение с реальными API-роутами корректны. Medium (non-GET к зарезервированным префиксам отдавал голый 405 вместо JSON-envelope) и low-находки (заголовки на 405, учёт байт-бюджета, регистр префиксов, `object-src`) исправлены и покрыты тестами.

Сделано на этапе 6: раздача бандла из API (ADR-013), честный `/` без ложных метрик, scroll-story с равнозначным reduced-motion путём, разбивка CSS по поверхностям, реалистичные фикстуры, media asset и ревью границы. Детали — в `docs/CURRENT_TASK.md`.

### Этап 7 — воспроизводимый portfolio release

- [x] **Три фикстуры** разных путей: чистый text-PDF, JPEG/PNG через OCR, документ с warning. Всё fictional и безопасно для публикации. *(Сделано на этапе 6: `fixture-aurora-stationery.pdf`, `fixture-meridian-supplies.png`, `fixture-cedarline-services.pdf`.)*
- [x] **Расширить Compose smoke**: duplicate, warning/correction, OCR и retry/dead-letter.
- [x] **Скриншоты из реального приложения** и **demo script на 60–90 секунд**.
- [x] **Финальные ревью:** code, database, security, document-AI, frontend/accessibility, performance; domain/database highs fixed. No auth/authz is the explicit loopback-demo boundary.
- [x] **Release gate** без ручных правок БД и платных credentials: Go, disposable PostgreSQL 17, Node frontend, clean Compose and isolated smoke passed 24 July 2026.
- [x] **Checklist приведён в соответствие с фактическим gate.**
- [x] English `README.md` — назначение, quick start, архитектура, demo flow, security model, честные ограничения. *(Сделано досрочно.)*

### Этап 8 — демо-опыт и презентация (текущий)

Этапы 0–7 закрыли инженерную часть. Этап 8 закрывает разрыв между качеством кода и первым впечатлением: сейчас посетитель портфолио видит в корне промпты для агентов, а загрузив собственный счёт — пустую форму. Ни один пункт ниже не меняет доменные инварианты.

#### P0 — блокируют ценность демо

- [x] **Офлайн-fallback экстрактор для незнакомого документа.** Реализовано в ADR-015. `FallbackStructuredExtractor` спрашивает реестр фикстур первым и обращается к `HeuristicStructuredExtractor` только когда тот не вернул ни одного кандидата (триггер — `Proposal.HasCandidates()`, форма результата, а не код диагностики, поэтому цепочка провайдер-нейтральна). Ридер детерминирован, офлайн, без состояния и устроен так, что его отказ — это молчание, а не выдумка: неоднозначные даты через слэш пропускаются, процент никогда не читается как сумма, построчный налог остаётся неизвестным, а имя поставщика предлагается только при подтверждении другим полем. Доказательства правдивы по построению — цитируется та самая строка источника, поэтому серверный `ValidateEvidence` проходит на реальных данных. Живая проверка: незнакомый счёт Northwind даёт supplier/email/номер/обе даты/валюту/subtotal/tax/total и обе позиции таблицы, `71.00 + 13.49 = 84.49` — без ложных предупреждений. Побочно исправлено: `pdftotext` получил `-layout` (без него Poppler отрывал каждую метку от её суммы) и процент больше не читается как сумма. Все фикстуры сохранили побайтово идентичные снимки, Compose smoke зелёный.
- [x] **Убрать агентную обвязку из корня публичного репозитория.** `MASTER_PROMPT.md`, `MASTER_PROMPT_PREMIUM_DESIGN.md`, `MODEL_ROUTING.md`, `MANIFEST.json`, `START_HERE_RU.md`, `gitignore.fragment`, `.kiro/` и `.codex/` перенесены в `.internal/` (в `.gitignore`, файлы остались на диске); `scripts/check-agent-pack.py` переехал туда же с обновлёнными путями, а `agent-pack` убран из `make check` как локальная проверка обвязки, а не product gate. В корне остались `README.md`, `ROADMAP.md`, `LICENSE`, `Makefile`, `Dockerfile`, `docker-compose.yml`, `go.mod`/`go.sum` и каталоги продукта. `AGENTS.md`, `CLAUDE.md` и `.claude/agents/` оставлены сознательно: это стандартные для современного репозитория файлы конфигурации агентов, а не промпты-инструкции, и перенос `CLAUDE.md` сломал бы загрузку инструкций проекта. История **не переписывалась** — старые коммиты по-прежнему содержат эти файлы; полное удаление из истории остаётся отдельной операцией по явной команде.

#### P1 — соответствие собственной спецификации

- [x] **Warnings о недостающих обязательных полях.** `missingFieldWarnings` в `internal/invoices/normalize.go` выдаёт `missing_required_field` для `supplier_name`, `invoice_number`, `issue_date`, `currency` и `total`. Проверяется **сырой** кандидат, а не нормализованный результат, поэтому присутствующее-но-отклонённое значение сохраняет свой конкретный warning (`invalid_date`, `invalid_money`, `unsupported_currency`) и не считается дважды. `subtotal` и `tax` сознательно не обязательны: множество законных счетов указывают только итог, а лишние предупреждения приучают ревьюера их игнорировать. Пустое предложение больше не проходит молча. Код добавлен в перечень на лендинге и в `docs/API_CONTRACT.md`.
- [x] **Привязать warnings к полям формы.** `ReviewWorkspace` строит индекс `field → warnings[]` и рендерит предупреждения на самом поле — `aria-invalid`, `aria-describedby` и амбер-рамка из существующих токенов, включая построчные адреса вида `line_items.0.total`. Список предупреждений внизу остался как сводка. Важная деталь, которую поймал тест: список предупреждений вынесен **за** `<label>`, иначе его текст попадал в доступное имя поля вместо описания.
- [x] **Список документов.** `GET /api/v1/documents` отдаёт bounded-страницу с keyset-пагинацией по `(created_at, id)`, а не offset: вставка нового документа во время листания не может продублировать или потерять строку. Размер страницы зажимается сервером (20 по умолчанию, максимум 100), курсор непрозрачный и валидируется — чужой курсор даёт `400 invalid_pagination`, а не тихий возврат на первую страницу. Проекция presentation-safe: без SHA-256, object id, storage key и внутреннего UUID версии; интеграционный тест это проверяет. На `/app` — таблица с поставщиком, номером, точной суммой из минорных единиц (целочисленная арифметика, без float), статусом и временем, плюс «Load more».

#### P2 — качество и презентация

- [x] **Привести в порядок фронтенд-код.** Подключены Prettier и ESLint (typescript-eslint с типовыми правилами + react-hooks), вся кодовая база переформатирована, `npm run format:check` и `npm run lint` добавлены в `make frontend-test` и в CI-job `frontend` перед сборкой. `ReviewWorkspace.tsx` (288 строк, строки до 2142 символов) разнесён на `review/ReviewForm.tsx`, `review/ReviewContext.tsx`, `review/ConfirmDialog.tsx`, `review/SourcePanel.tsx`, `review/ReviewMessage.tsx`; сам компонент остался оркестратором состояния. Правила выбраны по принципу «ловят дефекты, а не стиль»: `react-hooks/set-state-in-effect` нашёл три реальных каскадных рендера, и все три исправлены по существу — в `ProvenanceStory` признак «показать всё» стал производным значением вместо записи в состояние из эффекта, а в `ReviewWorkspace` и `DocumentList` сброс ошибки переехал в обработчик успеха (заодно ушло мигание сообщения при перезагрузке).
- [x] **Готовность к публичному стенду.** Реализовано всё, что предшествует нажатию «deploy». `PUBLIC_DEMO=true` включает баннер «общий демо-стенд, входа нет, данные периодически стираются, загружайте только вымышленные документы»; флаг отдаётся через `GET /api/v1/config`, где полезная нагрузка — один булев флаг, потому что маршрут неаутентифицирован. `UPLOAD_RATE_PER_MINUTE` ограничивает загрузки по адресу клиента **до чтения тела**, поэтому отказанный вызывающий не заставляет сервер прочитать 20 МиБ. `X-Forwarded-For` сознательно игнорируется: его может выставить кто угодно, и доверие к нему выдавало бы новый лимит на каждый запрос — за прокси лимит обязан стоять на самом прокси, и это записано в ADR-016 и `docs/DEPLOYMENT.md`. Оба параметра по умолчанию выключены, локальное демо не изменилось. Граница зафиксирована в **ADR-016**, инструкция — в **`docs/DEPLOYMENT.md`** (эфемерная БД, офлайн-экстрактор, без вебхука, регулярный сброс, чек-лист после развёртывания). Проверено вживую в Compose: `{"public_demo":true}`, баннер в бандле, лимит 2/мин даёт `201, 201, 429, 429` с `Retry-After: 60`.
  - [ ] **Само развёртывание не выполнено — оно за владельцем репозитория.** Требуется аккаунт на площадке и учётные данные, и оно публикует стенд в интернет; такое действие не выполняется без явной команды. Публиковать после прохождения чек-листа в `docs/DEPLOYMENT.md`; репозиторий по-прежнему не заявляет ни одного публичного URL.
- [x] **`actions/checkout@v4` → `v5`** — обновлено в обеих CI-job.

Не входит в этап 8: аутентификация, raster OCR для сканированных PDF, S3-адаптер, метрики. Они остаются в backlog ниже и требуют отдельных ADR.

### Release gate этапа 7

```text
make fmt
make lint
make test
DATABASE_URL=… make test-integration
make frontend-test
make build
docker compose up --build --wait
sh scripts/compose-smoke.sh
```

`make check` остаётся агрегированным gate; для CI/чистой машины ему нужен доступный PostgreSQL.

---

## Карта пути

```text
✅ intake → processing → extraction → human review/reject
✅ approve exact version → CSV / webhook jobs → export history
✅ Этап 6: честный product story + раздача UI из API + media asset
✅ Этап 7: reproducible portfolio release
✅ Этап 8: демо на любом документе, чистый репозиторий, warnings у полей,
   список документов, Prettier/ESLint, готовность к публичному стенду
   └─ Осталось: развернуть публичный стенд (владелец), затем production
      hardening и расширения только по отдельным ADR
```

## Неподвижные правила

- AI/OCR и пользовательский ввод — недоверенные данные; они не задают статус документа, storage key, актёра, секрет или destination webhook.
- Деньги представлены точно (integer minor units / `money-v1`), а не `float`.
- Оригинал, extraction/review versions и audit events не изменяются задним числом.
- Approval адресует ровно одну неизменяемую версию; экспорт адресует ровно утверждённую версию.
- Внешняя доставка at-least-once: идемпотентность обязательна, «exactly once webhook» не заявляется.
- Демо работает без платных ключей и использует только фиктивные документы/данные.
- Новый этап не расширяется неявно: сначала acceptance criteria, затем код, тесты, документация и review diff.

## После portfolio release — отдельный backlog

Эти работы не являются обещанием ближайшего MVP. Каждая требует отдельной оценки, ADR и threat model.

| Приоритет | Направление | Почему отдельно |
| --- | --- | --- |
| P0 для production, не для demo | Authentication/authorization, actor identity, document-level access control, roles | Текущий local demo сознательно использует fixed actor и не делает production security claim. |
| P0 для production | Least-privilege DB roles, secrets management, observability/metrics, backup/retention/runbooks, stronger process sandbox/malware controls | Эти меры меняют эксплуатационную модель, а не только UI. |
| P1 | PDF raster OCR с page/pixel accounting, безопасный live provider adapter | Нельзя «включить» без нового resource/security design. |
| P1 | Document list/search, pagination, manual processing retry и операторский UX | Нужны для потока документов, но не должны смешиваться с этапом approval/export. |
| P1 | S3/MinIO storage adapter, managed webhook destinations | Потребуют управления конфигурацией, секретами и deployment model. |
| P2 | Multi-tenant boundaries, extra accounting integrations, analytics | Не входят в текущую продуктовую цель и могут разрушить ясность portfolio demo. |

## Как вести roadmap

1. Перед началом пункта перенести его точную цель, scope, invariants и test plan в `docs/CURRENT_TASK.md`.
2. Сначала принять решения, меняющие данные, доверенные границы или публичный API; после — миграции и код.
3. Реализовывать вертикальным срезом: migration → repository/domain → worker/API → UI → tests → smoke → docs.
4. Не помечать пункт «готово» по наличию кода: обязательны проверка failure paths, зелёные релевантные тесты и review diff.
5. После релиза пересматривать план по фактическим ограничениям, не по желаемым marketing claims.

## Архив выполненных этапов

Полные acceptance-критерии завершённых этапов не дублируются здесь, чтобы план оставался читаемым:

- **Этапы 0–4** (fundament, intake, extraction, human review) — в `docs/DECISIONS.md` (ADR-000…ADR-011) и `docs/ARCHITECTURE.md`.
- **Этап 5** (approval и export) — ADR-012, `stage-5-review.md`, `docs/CURRENT_TASK.md`.
- **Этап 6** (раздача UI, product story) — ADR-013, `docs/ARCHITECTURE.md`, `docs/CURRENT_TASK.md`.
- **Этап 7** (reproducible portfolio release) — `docs/CURRENT_TASK.md`, `docs/DEMO_SCRIPT.md`, `docs/PORTFOLIO_RELEASE_CHECKLIST.md`.
