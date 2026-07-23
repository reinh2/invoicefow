# InvoiceFlow Agent Pack

Готовый пакет инструкций для создания InvoiceFlow через:

- Codex;
- Claude Code;
- Kiro IDE / Kiro CLI;
- любой другой агент, который читает `AGENTS.md`.

## Установка

1. Создай пустую папку проекта и инициализируй Git:
   ```bash
   mkdir invoiceflow
   cd invoiceflow
   git init
   ```
2. Распакуй **содержимое** этой папки в корень проекта.
3. Открой проект в Codex, Claude Code или Kiro.
4. Отправь агенту содержимое `MASTER_PROMPT.md` либо просто напиши:
   ```text
   Read MASTER_PROMPT.md and execute it. Begin with Stage 0 only.
   ```
5. После Stage 0 продолжай в том же чате. Агент сам подготовит точный prompt для следующей стадии.

## Какой файл читает каждый инструмент

- **Codex:** `AGENTS.md`, `.codex/config.toml`, `.codex/agents/*.toml`.
- **Claude Code:** `CLAUDE.md`, который импортирует `AGENTS.md`; также `.claude/agents/*.md`.
- **Kiro:** `AGENTS.md`, `.kiro/steering/*.md`, `.kiro/agents/*.json`.
- **Другой агент:** попроси сначала прочитать `AGENTS.md`, `MASTER_PROMPT.md` и `docs/`.

## Codex

Начальный prompt:

```text
Read MASTER_PROMPT.md and execute it. Use the project-scoped custom agents when useful. Begin with Stage 0 only: inspect, validate the architecture, update living documentation, and present the implementation plan before writing the application.
```

Codex может запускать сабагентов сам, но в master prompt явно указано, когда делегировать работу.

## Claude Code

Запусти Claude в корне проекта:

```bash
claude
```

Начальный prompt:

```text
Read MASTER_PROMPT.md and execute it. Use the project subagents proactively where their descriptions match. Begin with Stage 0 only.
```

Полезные команды:

- `/build-stage`
- `/review-stage`
- `/release-check`

## Kiro IDE

Открой папку проекта. Steering-файлы будут подхвачены автоматически.

Можно использовать основной чат:

```text
Read MASTER_PROMPT.md and execute it. Run subagents for independent architecture, database, security, and document-processing analysis. Begin with Stage 0 only.
```

## Kiro CLI

Из-за различий между версиями сначала проверь доступные custom agents:

```text
/agent swap
```

Выбери `invoiceflow-orchestrator`, если он появился. Custom agent JSON-файлы находятся в `.kiro/agents/`.

## Безопасность

В архиве нет:

- API-ключей;
- настоящих счетов;
- платёжных реквизитов;
- приватных данных;
- автоматического деплоя;
- команд, публикующих код.

Не используй реальные финансовые документы в публичном портфолио.

## Главный принцип

Сначала архитектура и ограниченный план, затем реализация по стадиям. Сильная модель нужна для архитектуры, транзакций и security review; механические задачи можно делегировать более дешёвым агентам.
