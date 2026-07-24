# Roadmap — InvoiceFlow

> Статус на 24 июля 2026 года. Это живой план, а не описание уже поставленных возможностей. При завершении каждого пункта нужно обновить этот файл, `docs/CURRENT_TASK.md`, `docs/API_CONTRACT.md` и, если решение меняет границы системы, `docs/DECISIONS.md`.

## Цель продукта

InvoiceFlow превращает PDF/JPG/PNG-счёт в **структурированное предложение для проверки человеком**. Оригинал остаётся доступным для сверки, правки создают неизменяемые версии, а экспорт возможен только из явно утверждённой версии.

Продукт демонстрирует надёжную обработку документов, а не бухгалтерскую систему: он не оплачивает счета, не подключается к банкам и не делает финансовые решения автоматически.

## Где проект находится сейчас

Завершены этапы 0–6. Репозиторий не прототип: Go API и worker, PostgreSQL, React/Vite-интерфейс, Docker Compose, зелёный CI (Go + integration + frontend). Проверенная сквозная цепочка: **загрузка → durable processing → извлечение → `needs_review` → неизменяемая human-review версия → утверждение точной версии → CSV / signed webhook экспорт**, интерфейс раздаётся из API (`WEB_DIR`, ADR-013) с честным лендингом и media asset из реального приложения.

| Область | Статус |
| --- | --- |
| Приём файлов (PDF/JPEG/PNG, лимиты, сигнатуры, SHA-256, дубликаты) | ✅ Готово |
| Надёжная обработка (PostgreSQL jobs, lease, retry/dead-letter) | ✅ Готово |
| Извлечение (bounded Poppler, OCR для изображений, строгая схема, `money-v1`, warnings) | ✅ Готово с ограничениями |
| Ревью (оригинал рядом с данными, неизменяемые версии, отклонение, audit) | ✅ Готово |
| Утверждение и экспорт (exact version, CSV, signed webhook, export jobs) | ✅ Готово |
| Раздача фронтенда из API + честный landing со scroll-историей + media asset (этап 6) | ✅ Готово |
| Release-пакет (фикстуры, скриншоты, demo script, финальные ревью, release gate) | 🔴 Не готов |

---

## ⇒ Что осталось сделать

### Этап 6 — завершён

- [x] **Реалистичные фиктивные фикстуры.** `testdata/` содержит три правдоподобных, полностью вымышленных документа, сгенерированных `scripts/gen-fixtures.py`: `fixture-aurora-stationery.pdf` (чистый text-PDF), `fixture-meridian-supplies.png` (image через OCR), `fixture-cedarline-services.pdf` (даёт ровно один `subtotal_tax_total_mismatch`). Каждая зарегистрирована в `cmd/worker` по реальному SHA-256 и маркеру; `cmd/worker/main_test.go` проверяет хеши и набор warnings через реальную нормализацию, а Compose smoke гоняет и text-PDF, и OCR-путь.
- [x] **Media asset (ADR-005).** `web/public/media/` содержит `demo.webm` (muted-скролл лендинга, ~2.3 МБ), `demo-landing-poster.png` и `demo-review.png` (review-экран с засеянным Meridian-счётом). Регенерация — `web/scripts/capture-media.mjs` (`@playwright/test`, `npm --prefix web run capture:media`) поверх изолированного засеянного Compose-демо; лендинг встраивает видео с poster и равнозначным reduced-motion/без-video путём (`DemoMedia`).
- [x] **Ревью новой границы `internal/webui`** — `code-reviewer` и `security-reviewer` прошли по раздаче статики (ADR-013). High-severity нет: traversal/symlink-escape/листинг структурно невозможны, content-type и разделение с реальными API-роутами корректны. Medium (non-GET к зарезервированным префиксам отдавал голый 405 вместо JSON-envelope) и low-находки (заголовки на 405, учёт байт-бюджета, регистр префиксов, `object-src`) исправлены и покрыты тестами.

Сделано на этапе 6: раздача бандла из API (ADR-013), честный `/` без ложных метрик, scroll-story с равнозначным reduced-motion путём, разбивка CSS по поверхностям, реалистичные фикстуры, media asset и ревью границы. Детали — в `docs/CURRENT_TASK.md`.

### Этап 7 — воспроизводимый portfolio release

- [x] **Три фикстуры** разных путей: чистый text-PDF, JPEG/PNG через OCR, документ с warning. Всё fictional и безопасно для публикации. *(Сделано на этапе 6: `fixture-aurora-stationery.pdf`, `fixture-meridian-supplies.png`, `fixture-cedarline-services.pdf`.)*
- [ ] **Расширить Compose smoke** на failure paths: duplicate, warning/correction, retry/dead-letter. Сейчас smoke покрывает happy path + rejection.
- [ ] **Скриншоты из реального приложения** и **demo script на 60–90 секунд**.
- [ ] **Финальные ревью:** code, database, security, document-AI, frontend/accessibility, performance. Закрыть все high-severity или явно не выпускать релиз.
- [ ] **Release gate с чистого окружения** без ручных правок БД и платных credentials (команды ниже).
- [ ] **Привести `docs/PORTFOLIO_RELEASE_CHECKLIST.md` в соответствие с реальностью** — часть инженерных и security-пунктов уже выполнена, но не отмечена.
- [x] English `README.md` — назначение, quick start, архитектура, demo flow, security model, честные ограничения. *(Сделано досрочно.)*

### Сквозные решения (небольшие, но нужны для публичного портфолио)

- [ ] **Файл лицензии.** Его нет — формально прав на переиспользование кода не выдано. Обычно MIT или Apache-2.0.
- [ ] **Судьба `MASTER_PROMPT.md` и `MASTER_PROMPT_PREMIUM_DESIGN.md`** в публичном репозитории (локально они с правами `600`; в репозитории с первого коммита). Удаление потребует переписывания истории — отдельная операция по явной команде.
- [ ] **`actions/checkout@v4` → `v5`** — CI работает, но GitHub предупреждает об устаревании Node 20. Мелочь на потом.

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
🔴 Этап 7: reproducible portfolio release
   └─ После релиза: production hardening и расширения только по отдельным ADR
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
