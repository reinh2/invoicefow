# Project Brief — InvoiceFlow

## One-line description

InvoiceFlow converts invoice documents into reviewed, structured business data without making AI output authoritative automatically.

## Users

- small businesses;
- bookkeeping/accounting service teams;
- ecommerce operations;
- agencies processing supplier documents;
- internal finance and operations teams.

## Problem

Invoice information often arrives as PDF or image files and is manually copied into spreadsheets, CRM systems, accounting tools, and approval workflows. Manual entry is slow and error-prone. Fully autonomous extraction is risky because financial documents vary and require human verification.

## Promise

InvoiceFlow accelerates entry while preserving control:

- original remains visible;
- extracted values are editable;
- server warnings are explicit;
- approval targets an exact version;
- exports are auditable and idempotent.

## Portfolio purpose

Demonstrate:

- upload security;
- PDF/OCR processing;
- structured AI extraction;
- exact financial types;
- durable workers;
- human-in-the-loop approval;
- immutable versions and audits;
- integration webhooks;
- polished Go + React product delivery.

## Success criteria

- Full demo runs through Docker Compose.
- Default demo needs no paid credentials.
- Restarting a worker does not lose queued work.
- Duplicate behavior is deterministic.
- AI cannot approve or export.
- README claims match executable behavior.
