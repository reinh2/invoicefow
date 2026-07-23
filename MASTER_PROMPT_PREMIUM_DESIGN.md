# Master Prompt — Build InvoiceFlow

You are the lead engineer and product designer responsible for creating **InvoiceFlow**, a portfolio-grade invoice document processing application.

You may create and modify files inside this repository and run local development, build, and test commands. You may use the repository-defined subagents. You must not deploy, publish, push, spend money, access secrets, use real financial documents, or create external paid resources without explicit permission.

Your goal is to build a product that demonstrates all of the following at once:

- reliable Go backend engineering;
- AI and document-processing workflows;
- PostgreSQL-backed durable jobs;
- polished React and TypeScript development;
- premium marketing-page design;
- modern motion design;
- scroll-driven storytelling;
- video-enhanced product presentation;
- accessible and performant frontend implementation.

The finished result should look like a real premium SaaS product, not a generic admin template or a static portfolio mockup.

---

## 1. Mandatory initialization

Before implementation:

1. Read `AGENTS.md` completely.
2. Read `CLAUDE.md` when supported.
3. Read every file under `docs/`.
4. Read all relevant files under:
   - `.kiro/steering/`
   - `.kiro/agents/`
   - `.kiro/prompts/`
   - `.codex/`
   - `.claude/`
5. Inspect the complete repository tree, existing toolchain, and Git status.
6. Determine whether the repository is empty, scaffolded, or partially implemented.
7. Compare the actual code with the proposed architecture, product scope, and design requirements.
8. Explicitly report contradictions, unavailable tools, missing assets, and material assumptions.
9. Run independent read-only analysis where supported:
   - architecture;
   - database and durable jobs;
   - security;
   - PDF, OCR, and AI trust boundaries;
   - frontend architecture;
   - visual design, motion, and video strategy.
10. Update `docs/CURRENT_TASK.md` with:
    - inspected repository state;
    - confirmed architecture;
    - confirmed visual direction;
    - Stage 1 acceptance criteria;
    - expected files;
    - validation commands.

Do **not** begin by generating the whole application.

Complete **Stage 0 only** in the first response.

---

## 2. Product objective

Build a system where a user can:

1. upload a fictional PDF, JPG, or PNG invoice;
2. observe durable processing;
3. receive structured invoice data;
4. review the source document and extracted data side by side;
5. correct supplier, metadata, totals, tax, and line items;
6. see server-generated validation warnings;
7. approve or reject an exact immutable review version;
8. export approved data as CSV;
9. optionally send approved data through a signed generic webhook;
10. inspect an append-only audit history.

The default demo must work without a paid API key through a deterministic fake provider.

InvoiceFlow is a document-processing and human-review product. It is not a full accounting system and never pays invoices.

---

## 3. Portfolio objective

The project must prove two categories of ability.

### Product engineering

- secure document upload;
- PDF and OCR processing;
- structured AI extraction;
- exact money handling;
- durable PostgreSQL-backed workers;
- human-in-the-loop approval;
- immutable versions and audit events;
- idempotent exports and webhooks;
- production-minded testing.

### Premium frontend and design

- beautiful marketing landing page;
- clear visual identity;
- cinematic but restrained motion;
- scroll-driven product storytelling;
- video-enhanced presentation;
- premium application dashboard;
- responsive interaction;
- consistent typography, spacing, color, and motion;
- accessibility and performance.

The marketing site and the application must feel like one coherent product.

---

## 4. Required frontend surfaces

The project must contain two distinct frontend surfaces.

### A. Marketing landing page

Preferred route:

```text
/
```

Purpose:

- explain the problem immediately;
- show the product visually;
- prove strong landing-page and motion-design skills;
- direct the visitor into the working demo.

Required sections:

1. Premium hero section.
2. Clear one-sentence value proposition.
3. Real product preview or polished product mockup based on the real UI.
4. Animated explanation of the processing workflow.
5. Scroll-driven video, image sequence, or live interface transformation.
6. Feature sections for:
   - upload;
   - extraction;
   - validation;
   - human review;
   - approval;
   - export;
   - audit history.
7. Reliability and architecture section.
8. Use-case section.
9. Final call to action leading to the working application.

### B. Working application

Preferred route:

```text
/app
```

Purpose:

- provide the complete upload-to-export demo;
- show real backend integration and real states;
- prove the project is functional rather than a static concept.

Do not replace the application with a marketing-only prototype.

---

## 5. Visual direction

Create a premium modern SaaS identity.

During Stage 0, propose two or three concrete art directions, compare them, and recommend one before implementation.

The final direction should generally include:

- dark or refined neutral base;
- restrained accent colors;
- strong contrast;
- high-quality typography;
- generous spacing;
- calm technical atmosphere;
- subtle depth and layered surfaces;
- clear hierarchy;
- polished transitions;
- custom visual details instead of an obvious template appearance.

Avoid:

- default dashboard-template styling;
- excessive gradients;
- random neon effects;
- exaggerated cyberpunk visuals;
- too many simultaneous animations;
- fake testimonials;
- fake client logos;
- fabricated analytics or usage metrics.

The result should look expensive, trustworthy, modern, and technically precise.

---

## 6. Design system

Create a small explicit design system.

At minimum define:

- typography scale;
- spacing scale;
- color tokens;
- surface tokens;
- border radii;
- shadows;
- motion durations;
- easing curves;
- breakpoints;
- focus states;
- success, warning, error, processing, and approved states.

Prefer CSS variables or an equivalent token layer.

Reusable components should include only what the product needs:

- buttons;
- inputs;
- status badges;
- cards;
- dialogs;
- navigation;
- section containers;
- animated headings;
- media frames;
- document panels;
- warning panels;
- audit timeline;
- upload drop zone.

Do not build a huge standalone design-system package.

---

## 7. Motion and animation requirements

Motion is a major portfolio requirement, but it must support the product story.

Implement a coherent motion system that includes:

- hero entrance sequence;
- staggered text and interface reveals;
- subtle parallax where appropriate;
- scroll-triggered section transitions;
- animated invoice-processing pipeline;
- card and panel transitions;
- smooth route or page transitions when useful;
- micro-interactions for buttons, tabs, uploads, warnings, approval, and export;
- loading and processing animations based on real application state;
- animated audit-timeline appearance;
- polished transitions between extracted, edited, and approved states.

Prefer a small set of reusable motion primitives over unrelated one-off effects.

Possible tools:

- CSS transitions and keyframes;
- Framer Motion;
- GSAP with ScrollTrigger only if the required scroll effect genuinely needs it;
- Intersection Observer;
- `requestAnimationFrame` for tightly controlled custom animation.

Do not add multiple animation libraries without a concrete justification.

---

## 8. Required scroll-driven video experience

The landing page must contain at least one high-quality scroll-driven visual section.

Preferred behavior:

1. A section becomes pinned or visually fixed during part of the scroll.
2. Scroll progress controls one or more of:
   - video playback position;
   - image-sequence frame;
   - product interface transformation;
   - document layers opening and closing;
   - transitions between invoice, extracted data, warnings, approval, and export.
3. Supporting text changes as the visual progresses.
4. The section exits cleanly and returns to normal document flow.

The preferred InvoiceFlow narrative is:

```text
Raw invoice
  ↓
Text and OCR extraction
  ↓
Structured fields appear
  ↓
Validation warnings are highlighted
  ↓
A human corrects and approves the data
  ↓
Approved data is exported
```

Acceptable technical approaches:

- scroll-scrubbed muted WebM/MP4 product video;
- rendered image sequence controlled by scroll progress;
- layered live DOM interface panels controlled by scroll;
- hybrid video plus live text and UI overlays.

Choose the approach during Stage 0 based on:

- visual quality;
- performance;
- implementation complexity;
- mobile behavior;
- ability to replace assets later;
- maintainability.

The section must not trap scrolling or make the page unusable.

---

## 9. Video and media asset strategy

During Stage 0, define how final media will be produced.

Allowed approaches:

- record the real product after the app exists;
- render a deterministic image sequence from the real UI;
- use a temporary abstract placeholder during development;
- include a small optimized local demo video;
- build the visual entirely from live DOM and CSS layers.

Rules:

- do not use copyrighted commercial footage;
- do not commit huge unoptimized media files;
- provide a poster image;
- prefer WebM and MP4 fallbacks where practical;
- use muted `playsInline` video;
- do not autoplay audio;
- lazy-load non-critical media;
- reserve dimensions to avoid layout shift;
- provide a mobile fallback;
- provide a no-video fallback;
- document how to regenerate or replace media.

If final video does not exist yet, build the component and fallback honestly. Do not describe placeholder footage as finished work.

---

## 10. Motion accessibility and performance

The page must remain usable and fast.

Required:

- respect `prefers-reduced-motion`;
- replace scroll scrubbing with a static or simplified experience for reduced-motion users;
- preserve keyboard navigation;
- preserve semantic HTML;
- maintain visible focus states;
- avoid scroll trapping;
- avoid excessive main-thread work;
- avoid layout shifts;
- lazy-load heavy assets;
- optimize images and video;
- use GPU-friendly properties where practical;
- test laptop, tablet, and mobile layouts;
- provide touch-friendly mobile behavior;
- make the product understandable if motion or media fails.

During Stage 0, establish measurable performance budgets for:

- JavaScript bundle size;
- video/image-sequence size;
- layout stability;
- initial loading strategy;
- production-build Lighthouse checks;
- reduced-motion behavior.

Do not sacrifice the working application for visual effects.

---

## 11. Architecture target

Unless repository inspection proves another compatible structure is better:

- Go backend;
- PostgreSQL via `pgx`;
- forward SQL migrations;
- modular monolith;
- separate API and worker binaries;
- PostgreSQL-backed durable processing and export jobs;
- React + TypeScript + Vite frontend;
- local filesystem storage adapter behind an interface;
- PDF text extraction adapter;
- OCR adapter;
- provider-neutral structured extractor;
- deterministic fake provider;
- optional real provider;
- Docker Compose;
- CI with Go, PostgreSQL integration, frontend, and Compose smoke checks.

Do not create microservices.

The frontend architecture should clearly separate:

- landing-page sections;
- application shell;
- design tokens;
- motion utilities;
- media components;
- reusable product components;
- API state and domain state.

---

## 12. Required domain concepts

Design explicit representations for:

- document;
- stored object;
- processing job;
- processing attempt;
- extraction version;
- review version;
- supplier;
- invoice header;
- line item;
- validation warning;
- human edit;
- approval or rejection;
- export job;
- export attempt;
- audit event.

Money must use an exact representation, never binary floating point.

---

## 13. Required processing behavior

1. Validate file extension, MIME, size, and signature.
2. Calculate SHA-256.
3. Resolve duplicates deterministically.
4. Durably save the original before asynchronous work.
5. Atomically save the upload audit event and enqueue processing.
6. Claim jobs transactionally with lease recovery.
7. Extract text from text-based PDFs.
8. Use OCR only when necessary.
9. Send bounded hostile text to the provider as data, not instructions.
10. Require strict structured provider output.
11. Validate and normalize output server-side.
12. Recalculate arithmetic and generate warnings.
13. Persist immutable extraction and review versions.
14. Move the document to `needs_review`.
15. Let a human create an edited exact review version.
16. Approve or reject explicitly.
17. Export only an approved version.
18. Make external exports idempotent.
19. Retry only appropriate transient failures.
20. Dead-letter exhausted jobs without losing the document or history.

---

## 14. Security requirements

Implement the repository rules, especially:

- real file-signature validation;
- server-owned storage keys;
- path-traversal and symlink defenses;
- fixed extraction binaries and argument arrays;
- no shell interpolation;
- process timeouts and bounded output;
- sanitized logs;
- strict provider schemas;
- prompt-injection resistance;
- server-owned approval and export authority;
- signed webhooks with constant-time verification;
- no secrets or real documents in the repository.

---

## 15. Central application UI

The working application must visibly show:

- source PDF or image on the left;
- editable supplier, metadata, totals, tax, and line items on the right;
- clear visual states for:
  - extracted;
  - warned;
  - edited;
  - approved;
- audit history;
- explicit approve, reject, and export confirmation;
- loading, duplicate, failed, retry, and empty states;
- responsive and accessible interaction.

The application must also use polished state-driven motion:

- upload drop-zone feedback;
- real processing transitions;
- warning reveal;
- form save feedback;
- approval confirmation;
- export completion;
- audit timeline reveal.

Animations must never hide errors or delay essential actions unnecessarily.

---

## 16. Seeded demo

Provide at least three fictional fixtures:

1. clean text-based PDF;
2. scan or image requiring OCR or deterministic fake OCR behavior;
3. invoice with a total, tax, or missing-field warning.

Primary portfolio demo flow:

1. open the premium landing page;
2. scroll through the video-driven product explanation;
3. enter the application;
4. upload an invoice;
5. observe processing;
6. review extracted fields;
7. correct one warning;
8. approve the exact version;
9. export CSV;
10. inspect audit history.

---

## 17. Agent orchestration

Use specialized agents deliberately.

### Stage 0 read-only work

Parallel where supported:

- `architect`;
- `code_mapper`;
- `database_reviewer`;
- `security_reviewer`;
- `document_ai`;
- `frontend_react` for frontend architecture and visual feasibility.

### Implementation

- `backend_go`;
- `frontend_react`;
- `document_ai`;
- `test_engineer`.

### Review

After every major stage:

- `code_reviewer`;
- `database_reviewer` for migrations, jobs, and state transitions;
- `security_reviewer` for upload, process execution, authorization, webhooks, and AI boundaries;
- frontend review for accessibility, performance, responsive behavior, and motion quality.

Do not allow concurrent overlapping writes.

The main agent owns integration and final decisions.

---

## 18. Implementation stages

### Stage 0 — inspect, design, and plan

Deliver:

- repository inventory;
- validated backend architecture;
- validated frontend architecture;
- two or three visual-direction options;
- recommended final visual direction;
- design-token proposal;
- motion strategy;
- scroll-driven video implementation strategy;
- media-production strategy;
- package and module layout;
- entity and transaction outline;
- API outline;
- risk register;
- accessibility safeguards;
- performance budgets;
- exact staged plan;
- validation strategy;
- updated living documents.

Do not build the application yet.

### Stage 1 — foundation and design system

Deliver:

- backend and frontend scaffolds;
- API and worker entry points;
- configuration;
- PostgreSQL and migrations;
- health and readiness;
- Docker Compose;
- baseline CI;
- storage and extraction-provider interfaces;
- fake provider skeleton;
- frontend routing for `/` and `/app`;
- design tokens;
- typography and layout primitives;
- motion primitives;
- reduced-motion foundation;
- landing-page skeleton.

### Stage 2 — upload and durable processing

Deliver:

- file validation;
- hashing;
- storage;
- duplicate behavior;
- document persistence;
- atomic job enqueue;
- worker claim, lease, and attempt lifecycle;
- PostgreSQL tests;
- polished animated upload experience;
- real processing-state UI.

### Stage 3 — extraction and validation

Deliver:

- PDF extraction;
- OCR adapter;
- strict invoice schema;
- fake provider;
- optional real provider;
- normalization;
- arithmetic and completeness warnings;
- immutable extraction versions;
- retry and failure behavior;
- animated processing-pipeline component for the landing page.

### Stage 4 — review and approval

Deliver:

- document list;
- review API;
- split-screen review UI;
- editing and line items;
- warnings;
- exact review versions;
- approval and rejection;
- audit events;
- tests;
- polished state-driven transitions.

### Stage 5 — export and integrations

Deliver:

- exact approved-version export;
- CSV;
- signed generic webhook;
- idempotency;
- retries and dead letter;
- sample n8n payload or workflow documentation;
- export and completion animations.

### Stage 6 — premium landing and scroll-driven story

Deliver:

- final premium landing page;
- polished hero sequence;
- real product preview;
- final scroll-driven video, image-sequence, or live UI story;
- motion-rich feature sections;
- use cases;
- reliability and architecture section;
- final CTA into the real application;
- mobile and tablet behavior;
- reduced-motion fallback;
- optimized media;
- frontend performance checks.

Use real product screens or footage wherever practical.

### Stage 7 — portfolio release

Deliver:

- fictional fixtures;
- complete no-key demo;
- Compose smoke test;
- real screenshots;
- optimized video and visual assets;
- public README;
- architecture documentation matching the code;
- honest limitations;
- 60–90 second demo recording script;
- final security, database, code, frontend, accessibility, and performance reviews.

---

## 19. Project commands

Create a simple project-native command surface, preferably:

- `make fmt`
- `make lint`
- `make test`
- `make test-integration`
- `make frontend-test`
- `make build`
- `make up`
- `make smoke`
- `make check`

Add frontend validation where appropriate:

- TypeScript checking;
- production build;
- component tests;
- accessibility checks;
- reduced-motion checks;
- media-size validation;
- performance inspection.

Adapt commands to the actual repository, but preserve one full release gate.

---

## 20. End-of-stage response

After every stage provide:

- concise summary;
- changed files;
- commands and results;
- architecture decisions;
- design and motion decisions where relevant;
- unresolved risks;
- living-document updates;
- exact next prompt.

Do not continue into a major stage when a material architecture or visual-direction decision requires user approval.

Otherwise, proceed autonomously through straightforward implementation and testing.

---

# First action

Begin now with **Stage 0 only**.

Read all repository instruction files, inspect the actual repository, and run focused read-only subagent analysis.

Return:

1. repository state;
2. proposed backend architecture;
3. proposed frontend architecture;
4. two or three premium visual directions;
5. your recommended visual direction;
6. motion-system strategy;
7. scroll-driven video or image-sequence strategy;
8. media-production strategy;
9. accessibility and performance safeguards;
10. staged implementation plan;
11. risks and open decisions;
12. updated `docs/CURRENT_TASK.md`.

Do not implement the complete application in the first response.
