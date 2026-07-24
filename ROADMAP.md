# Roadmap — InvoiceFlow

> Статус на 24 июля 2026 года. Это живой план развития, а не описание уже поставленных возможностей. При завершении каждого этапа нужно обновить этот файл, `docs/CURRENT_TASK.md`, `docs/API_CONTRACT.md` и, если решение меняет границы системы, `docs/DECISIONS.md`.

## Цель продукта

InvoiceFlow превращает PDF/JPG/PNG-счёт в **структурированное предложение для проверки человеком**. Оригинал остаётся доступным для сверки, правки создают неизменяемые версии, а экспорт возможен только из явно утверждённой версии.

Продукт демонстрирует надёжную обработку документов, а не бухгалтерскую систему: он не оплачивает счета, не подключается к банкам и не делает финансовые решения автоматически.

## Где проект находится сейчас

Завершены этапы 0–5; Stage 5 повторно проверен как `PASS` после remediation attempt projection, exact approved-version FK, durable retry/dead-letter и UI lifecycle. Репозиторий не является прототипом: в нём есть Go API и worker, PostgreSQL, React/Vite-интерфейс, Docker Compose и автоматические проверки.

| Область | Фактически реализовано | Статус |
| --- | --- | --- |
| Приём файлов | PDF/JPEG/PNG, лимиты, проверка расширения/MIME/сигнатуры, SHA-256, приватное серверное хранилище, детерминированные дубликаты | Готово |
| Надёжная обработка | PostgreSQL jobs, lease-токены, попытки, восстановление истёкших lease, retry/dead-letter для processing | Готово |
| Извлечение | bounded Poppler, OCR для JPEG/PNG, строгая схема, детерминированный offline fake extractor, нормализация `money-v1`, warnings | Готово с ограничениями |
| Ревью | Оригинал рядом с данными, правка полей и строк, неизменяемые версии, отклонение, audit history | Готово |
| Утверждение и экспорт | Утверждение конкретной версии, CSV, webhook, export jobs и export attempts | Готово |
| Презентация | Честный landing со scroll-историей, раздача собранного фронтенда из API (`WEB_DIR`, ADR-013), рабочая upload/review-страница | Частично готово: нет media asset |
| Release-пакет | Полный no-key demo, три фикстуры, реальные скриншоты, public README, финальные ревью | Не готов |

Текущая проверенная цепочка: **загрузка → durable processing → извлечение → `needs_review` → сохранение неизменяемой human-review версии → утверждение точной версии → CSV / signed webhook экспорт**.

## Неподвижные правила

- AI/OCR и пользовательский ввод — недоверенные данные; они не задают статус документа, storage key, актёра, секрет или destination webhook.
- Деньги представлены точно (integer minor units / `money-v1`), а не `float`.
- Оригинал, extraction/review versions и audit events не изменяются задним числом.
- Approval адресует ровно одну неизменяемую версию; экспорт адресует ровно утверждённую версию.
- Внешняя доставка at-least-once: идемпотентность обязательна, «exactly once webhook» не заявляется.
- Демо работает без платных ключей и использует только фиктивные документы/данные.
- Новый этап не расширяется неявно: сначала acceptance criteria, затем код, тесты, документация и review diff.

## Карта пути

```text
Готово: intake → processing → extraction → human review/reject
                                      │
                                      ▼
Этап 5: approve exact version → CSV / webhook jobs → export history
                                      │
                                      ▼
Этап 6: честный product story и media на landing
                                      │
                                      ▼
Этап 7: reproducible portfolio release
                                      │
                                      ▼
После релиза: production hardening и расширения только по отдельным ADR
```

## Этап 5 — утверждение и экспорт

**Цель:** замкнуть основной пользовательский сценарий без расширения в платежи, живого AI-провайдера, list/search, OCR scanned PDF или финальный landing.

### 5.1. Сначала зафиксировать контракт и модель данных

- Принять ADR для approval и webhook до реализации: server-owned actor/destination/secret, canonical payload bytes, HMAC algorithm, timestamp, replay window, TLS-only/без redirect, SSRF и DNS-rebinding защита.
- Добавить только forward migration. Явно связать документ с утверждённой immutable `invoice_version`; запретить смену этой ссылки после approval.
- Добавить сущности/таблицы export records и export attempts либо эквивалентную минимальную схему, которая фиксирует: тип экспорта, exact approved version, destination identity без секрета, idempotency key, состояние, число попыток, расписание, безопасную ошибку и timestamps.
- Сформулировать однозначную семантику статуса: документ переходит `needs_review → approved` атомарно с audit event; успешный экспорт переводит `approved → exported`. Если CSV и webhook оба поддерживаются для одного документа, заранее определить, как это согласуется с единственным состоянием `exported` и историей нескольких экспортов.
- Обновить `docs/API_CONTRACT.md`, `docs/ARCHITECTURE.md`, `docs/DECISIONS.md` и `docs/CURRENT_TASK.md` до/вместе с изменением контракта.

### 5.2. Утверждение exact version

- Реализовать явный endpoint с `version_number` и `confirm: true`; не допускать неявного «approve latest».
- В одной транзакции заблокировать document row, проверить `needs_review`, существование и допустимость указанной версии, сохранить approval reference, сменить статус и добавить `document_approved` audit event с номером версии.
- Вернуть стабильные ошибки для несуществующего документа, несуществующей/устаревшей версии, повторного approval и иной недопустимой стадии.
- В UI показать конкретную утверждаемую версию, предупреждение о необратимости в текущем workflow и модальное подтверждение. После approval форма read-only; audit явно показывает актёра, время и версию.

### 5.3. CSV как детерминированный локальный export

- Экспортировать только canonical normalized data утверждённой версии; никогда не пересчитывать или не брать «последнюю» версию во время экспорта.
- Зафиксировать версию CSV-формата, порядок/названия колонок, UTF-8 encoding, escaping, представление exact money и строковую обработку переносов. Не использовать browser float.
- Сделать повторный запрос идемпотентным: один и тот же exact version/type не должен создавать противоречивые результаты или дублировать audit events.
- Выбрать и документировать безопасный UX: контролируемое скачивание сервером сформированного CSV либо durable CSV export job с доступом к неизменяемому результату. Выбор определяется тем, нужно ли демонстрировать единый job-path для всех exports; он не должен обходить invariant approval.

### 5.4. Generic signed webhook через durable job

- Конфигурировать destination и secret только на сервере; local demo по умолчанию не отправляет запросы наружу. Для smoke использовать локальный контролируемый receiver или зафиксированный demo adapter, не пользовательский URL.
- Создавать `export_document` job только после approval и вместе с audit event в транзакции; взять существующие lease/retry/dead-letter primitives как основу, но не смешивать lifecycle processing и export.
- Подписывать канонические bytes payload HMAC, добавлять timestamp и immutable idempotency key. При необходимости receiver проверяет подпись constant-time сравнением.
- До соединения валидировать URL и разрешённый адрес; запретить private/reserved ranges, redirects, произвольные схемы/ports и DNS rebinding. Установить bounded timeout, response-size limit и безопасную классификацию retryable/permanent ошибок.
- Сохранять только безопасный результат доставки. Не логировать секреты, document text, raw payload и подробности сетевой ошибки.

### 5.5. Представление export в UI

- Добавить отдельные confirm flows для approval и export; показывать approved version, формат/назначение, состояние job, retry/dead-letter и завершение без ложных «success».
- Сохранить accessibility: нативные кнопки, корректный focus в modal, видимые статусы, текстовые альтернативы motion и `prefers-reduced-motion`.
- Дополнить audit timeline событиями approval, enqueue, successful export, retry и dead-letter. История объясняет факт, а не раскрывает секреты или внешние URL.

### Definition of done этапа 5

- Нельзя утвердить несуществующую, неактуальную или неявно выбранную версию; approved version неизменяема.
- Нельзя экспортировать `needs_review`, `rejected`, `failed` или произвольную review version.
- CSV стабилен и содержит только canonical approved data.
- Повтор export-запроса идемпотентен; worker restart, lease loss и transient webhook error не теряют историю и не обходят лимит попыток.
- Webhook подписан, destination server-owned и защищён от базовых SSRF/replay/redirect ошибок; секреты не попадают в API, БД audit, ошибки или логи.
- Есть unit, PostgreSQL integration, API/UI и Compose smoke coverage полного пути: upload → extract → correction → approve exact version → CSV и/или controlled webhook → audit.
- Все существующие проверки зелёные, документация описывает только реально доступное поведение.

### За пределами этапа 5

Нельзя включать сюда платёжные функции, реального платного extractor provider, пользовательскую конфигурацию webhook URL, production authentication, list/search, manual retry endpoint, PDF raster OCR, финальный landing или неограниченные интеграции.

## Этап 6 — premium landing и достоверная продуктовая история

**Цель:** превратить уже работающий demo в понятную портфолио-презентацию, не маскируя незавершённые функции.

- Привести `/` в соответствие с текущим продуктом: убрать устаревшие заявления о «foundation only» и будущих review/upload, если эти возможности уже работают.
- Построить единую историю: оригинал → извлечение → server warnings → human correction → approval exact version → export → audit. Все экраны и данные должны происходить из реального fictional demo.
- Добавить hero, real product preview, workflow/architecture, use cases, reliability section и CTA в `/app`; не добавлять фиктивные метрики, клиентов или claims об accuracy/compliance.
- Реализовать одну качественную scroll-driven сцену средствами live DOM или оптимизированного factual media. Для reduced motion, mobile и отсутствия video дать равнозначный статический/упрощённый путь.
- Создать минимум один реальный media asset из приложения: короткий WebM/MP4 с poster/fallback или оптимизированную image sequence. Зафиксировать источник, команду регенерации, форматы и размерный бюджет.
- Проверить keyboard navigation, focus, контраст, responsive split-review, отсутствие layout shift и production bundle/media size. Motion не должен скрывать ошибку или задерживать действие.

### Definition of done этапа 6

- [x] Landing ведёт в настоящий рабочий `/app` и описывает ровно то, что демонстрирует приложение. Раздача собранного бандла из API добавлена отдельным решением ADR-013, потому что до неё `docker compose up` не отдавал интерфейс вообще.
- [x] Ключевые feature sections и product preview опираются на реальные fictional flows: значения walkthrough — это фикстура `OFFICE-001`, на которую действительно настроен offline extractor.
- [x] Scroll-story, mobile/tablet layout и reduced-motion fallback проверены автоматизированно (`web/src/app/routes/landing.test.tsx`) и вручную в реальном браузере против изолированного Compose-демо.
- [ ] **Новый media asset оптимизирован, имеет fallback и не создаёт неприемлемую загрузку страницы.** Не сделано. Блокеры записаны в `docs/CURRENT_TASK.md`: нужен headless-инструмент захвата и реалистичные фикстуры, иначе на скриншоте панель оригинала пустая.

## Этап 7 — portfolio release

**Цель:** сделать проект воспроизводимым и честно демонстрируемым с чистого clone до полного сценария.

- Подготовить не менее трёх фикстур: чистый text-PDF, JPEG/PNG с OCR-путём и документ с предупреждением. Все данные должны быть fictional и безопасны для публикации.
- Расширить Compose smoke до полного happy path и ключевых failure paths: duplicate, warning/correction, approval, idempotent CSV/webhook, retry/dead-letter там, где применимо.
- Подготовить English `README.md`: назначение, быстрый старт, команды, архитектура, demo flow, screenshots, security model, честные ограничения и отсутствие payment/accounting claims.
- Обновить architecture/API/decision docs так, чтобы они соответствовали коду. Добавить 60–90-second demo script и реальные скриншоты/медиа.
- Выполнить финальные code, database, security, document-AI, frontend/accessibility и performance reviews; закрыть все high-severity замечания или явно не выпускать релиз.
- Выполнить release gate с чистого окружения без ручных правок БД и без платных credentials.

### Release gate

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

Команда `make check` должна оставаться удобным агрегированным gate; для CI/чистой машины ей требуется доступный PostgreSQL согласно текущему контракту команды.

## После portfolio release — отдельный backlog

Эти работы не являются обещанием ближайшего MVP. Каждая требует отдельной оценки, ADR и threat model.

| Приоритет | Направление | Почему отдельно |
| --- | --- | --- |
| P0 для production, не для demo | Authentication/authorization, actor identity, document-level access control, roles | Текущий local demo сознательно использует fixed actor и не делает production security claim. |
| P0 для production | Least-privilege DB roles, secrets management, observability/metrics, backup/retention/runbooks, stronger process sandbox/malware controls | Эти меры меняют эксплуатационную модель, а не только UI. |
| P1 | PDF raster OCR с page/pixel accounting, безопасный live provider adapter | Нельзя «включить» без нового resource/security design. |
| P1 | Document list/search, pagination, manual processing retry и операторский UX | Нужны для работы с потоком документов, но не должны смешиваться с этапом approval/export. |
| P1 | S3/MinIO storage adapter, managed webhook destinations | Потребуют управления конфигурацией, секретами и deployment model. |
| P2 | Multi-tenant boundaries, extra accounting integrations, analytics | Не входят в текущую продуктовую цель и могут разрушить ясность portfolio demo. |

## Как вести roadmap

1. Перед началом этапа перенести его точную цель, scope, invariants и test plan в `docs/CURRENT_TASK.md`.
2. Сначала принять решения, которые меняют данные, доверенные границы или публичный API; после этого делать миграции и код.
3. Реализовывать вертикальным срезом: migration → repository/domain → worker/API → UI → tests → smoke → docs.
4. Не помечать пункт «готово» по наличию кода: обязательны проверка failure paths, зелёные релевантные тесты и review diff.
5. После релиза пересматривать этот план по фактическим ограничениям, не по желаемым marketing claims.
