import {
  useEffect,
  useRef,
  useState,
  type CSSProperties,
  type ReactElement,
  type ReactNode,
} from 'react';
import { StatusTag, type StatusTone } from './StatusTag';
import { useReducedMotion } from '../motion/useReducedMotion';

/* Every value below is the fictional ORCHARD-001 demo document that ships in
   `testdata/`. The offline extractor in `cmd/worker` is configured with exactly
   these candidates, so this walkthrough shows real demo output rather than
   invented marketing data. */
const demo = {
  supplier: 'Orchard Office Supplies',
  email: 'billing@orchard.example.test',
  number: 'ORCHARD-001',
  issued: '2026-07-01',
  due: '2026-07-31',
  currency: 'USD',
  subtotal: '20.00',
  tax: '4.00',
  total: '24.00',
  lineDescription: 'Paper archive boxes',
  correctedDescription: 'Paper archive boxes - reviewed',
  quantity: '2',
  unitPrice: '10.00',
} as const;

type FieldTone = 'extracted' | 'edited' | 'approved';

function Field({
  label,
  value,
  tone,
  note,
}: {
  label: string;
  value: string;
  tone: FieldTone;
  note?: string;
}): ReactElement {
  return (
    <div className={`story-field story-field-${tone}`}>
      <span className="story-field-label">{label}</span>
      <span className="story-field-value">{value}</span>
      {note !== undefined ? <span className="story-field-note">{note}</span> : null}
    </div>
  );
}

type Step = { state: string; tone: StatusTone; title: string; body: string; panel: ReactNode };

const steps: Step[] = [
  {
    state: 'Stored',
    tone: 'neutral',
    title: 'The original is kept, hashed, and never rewritten.',
    body: 'Upload is accepted only when the extension, the declared media type, and the file signature agree. The bytes are hashed with SHA-256 before a document record exists, stored under a server-generated key, and remain readable beside every later proposal.',
    panel: (
      <div className="story-paper" aria-hidden="true">
        <span className="story-paper-label">Original document</span>
        <span className="story-paper-title">{demo.number}</span>
        <span className="story-paper-lines" />
      </div>
    ),
  },
  {
    state: 'Extracted',
    tone: 'info',
    title: 'Extraction produces a proposal, not an answer.',
    body: 'A durable PostgreSQL job runs bounded text extraction, falls back to OCR for images, and asks the extractor for strict structured candidates. The server re-validates and normalizes every field into version 1 — an immutable snapshot the model cannot edit afterwards.',
    panel: (
      <div className="story-fields">
        <Field label="Supplier" value={demo.supplier} tone="extracted" />
        <Field label="Invoice number" value={demo.number} tone="extracted" />
        <Field
          label="Line 1"
          value={demo.lineDescription}
          tone="extracted"
          note={`${demo.quantity} × ${demo.unitPrice} ${demo.currency}`}
        />
        <Field
          label="Total"
          value={`${demo.total} ${demo.currency}`}
          tone="extracted"
          note="Exact minor units, never a browser float"
        />
      </div>
    ),
  },
  {
    state: 'Corrected',
    tone: 'success',
    title: 'A human edit adds a version instead of overwriting one.',
    body: 'Saving a correction writes a new immutable version with source human_review and one audit event, in a single transaction. Version 1 keeps its original candidates, its warnings, and its source evidence — the history is append-only, so nothing is quietly replaced.',
    panel: (
      <div className="story-fields">
        <Field
          label="Line 1 · version 1"
          value={demo.lineDescription}
          tone="extracted"
          note="Extracted proposal, retained"
        />
        <Field
          label="Line 1 · version 2"
          value={demo.correctedDescription}
          tone="edited"
          note="Human correction"
        />
        <Field
          label="Total"
          value={`${demo.total} ${demo.currency}`}
          tone="extracted"
          note="Unchanged by this edit"
        />
      </div>
    ),
  },
  {
    state: 'Approved',
    tone: 'success',
    title: 'Approval names one exact version, and export reads only that one.',
    body: 'Approval requires the explicit version number plus a confirmation, and it locks the document against further edits. CSV export of the same approved version is byte-identical every time; the webhook export is a durable signed job with a stable idempotency key.',
    panel: (
      <div className="story-fields">
        <Field
          label="Approved version"
          value="Version 2"
          tone="approved"
          note="Referenced immutably on the document"
        />
        <Field
          label="CSV export"
          value={`invoice-…-v2.csv · ${demo.total} ${demo.currency}`}
          tone="approved"
          note="Repeatable, byte-identical"
        />
        <Field
          label="Webhook export"
          value="HMAC-SHA256 signed, server-owned destination"
          tone="approved"
          note="Bounded retries, then dead-letter"
        />
      </div>
    ),
  },
];

type StoryScroll = {
  active: number;
  revealed: boolean[];
  register: (index: number) => (element: HTMLLIElement | null) => void;
};

/* Two observers drive the scroll motion. A center band picks the single active
   step (the one whose transition is currently being read); a lower threshold
   marks each step "revealed" once — a sticky flag, so a card never re-hides on
   scroll-up. When IntersectionObserver is unavailable, every step is revealed
   immediately, so the no-observer path is equivalent rather than blank. */
function useStoryScroll(enabled: boolean): StoryScroll {
  const [active, setActive] = useState(0);
  const [observed, setObserved] = useState<boolean[]>(() => steps.map(() => false));
  const elements = useRef<(HTMLLIElement | null)[]>([]);

  // Whether the fallback applies is derived from props and platform support, so
  // it never has to be written back into state from an effect.
  const revealEverything = !enabled || typeof IntersectionObserver === 'undefined';

  useEffect(() => {
    if (revealEverything) return undefined;
    const emphasis = new IntersectionObserver(
      (entries) => {
        setActive((current) => {
          let next = current;
          for (const entry of entries) {
            const index = Number(entry.target.getAttribute('data-step-index'));
            if (entry.isIntersecting && Number.isInteger(index)) next = index;
          }
          return next;
        });
      },
      { rootMargin: '-45% 0px -45% 0px' },
    );

    const reveal = new IntersectionObserver(
      (entries) => {
        setObserved((current) => {
          let changed = false;
          const next = current.slice();
          for (const entry of entries) {
            const index = Number(entry.target.getAttribute('data-step-index'));
            if (entry.isIntersecting && Number.isInteger(index) && !next[index]) {
              next[index] = true;
              changed = true;
            }
          }
          return changed ? next : current;
        });
      },
      { rootMargin: '0px 0px -12% 0px', threshold: 0.2 },
    );

    for (const element of elements.current) {
      if (element !== null) {
        emphasis.observe(element);
        reveal.observe(element);
      }
    }
    return () => {
      emphasis.disconnect();
      reveal.disconnect();
    };
  }, [revealEverything]);

  const register =
    (index: number) =>
    (element: HTMLLIElement | null): void => {
      elements.current[index] = element;
    };
  const revealed = revealEverything ? steps.map(() => true) : observed;
  return { active, revealed, register };
}

/* The scene never hides content behind scrolling: every step is rendered and
   readable at all times, and scroll position only moves visual emphasis. That
   keeps the reduced-motion and no-IntersectionObserver paths equivalent rather
   than degraded. */
export function ProvenanceStory(): ReactElement {
  const reducedMotion = useReducedMotion();
  const { active, revealed, register } = useStoryScroll(!reducedMotion);

  return (
    <section id="story" className="story" aria-labelledby="story-title">
      <div className="story-intro">
        <p className="eyebrow">One document, end to end</p>
        <h2 id="story-title">Each state is visible, and each transition is recorded.</h2>
        <p className="hero-copy">
          This walkthrough uses the fictional ORCHARD-001 document included in the repository.
          Running the demo locally produces these values from the offline extractor — no key, no
          network call, and no customer data.
        </p>
      </div>
      <ol className="story-steps" data-motion={reducedMotion ? 'static' : 'scroll'}>
        {steps.map((step, index) => (
          <li
            key={step.state}
            ref={register(index)}
            data-step-index={index}
            data-active={!reducedMotion && index === active ? 'true' : 'false'}
            data-passed={!reducedMotion && index < active ? 'true' : 'false'}
            data-inview={reducedMotion || revealed[index] ? 'true' : 'false'}
            style={{ '--story-step': index } as CSSProperties}
          >
            <p className="story-index" aria-hidden="true">
              {index + 1}
            </p>
            <div className="story-body">
              <StatusTag tone={step.tone}>{step.state}</StatusTag>
              <h3>{step.title}</h3>
              <p>{step.body}</p>
              {step.panel}
            </div>
          </li>
        ))}
      </ol>
    </section>
  );
}
