import { PageFrame } from '../../components/PageFrame';
import { ProvenanceStory } from '../../components/ProvenanceStory';
import { DemoMedia } from '../../components/DemoMedia';
import { StatusTag } from '../../components/StatusTag';
import type { ReactElement } from 'react';

const pipeline = [
  { title: 'Intake', body: 'One PDF, JPEG, or PNG up to 20 MiB. Extension, declared media type, and file signature must agree; images are fully decoded and PDFs must carry an end marker. SHA-256 is computed before any record exists, so a repeated file is a deterministic conflict rather than a second document.' },
  { title: 'Durable job', body: 'The stored object, the document, the first audit event, and the processing job are created in one transaction. Jobs are claimed with a lease token, track their attempts, recover after an abandoned lease, retry with a bound, and dead-letter instead of looping.' },
  { title: 'Bounded extraction', body: 'Poppler inspects and extracts PDF text under fixed page, byte, and time limits; Tesseract handles JPEG and PNG when there is no text layer. Tools are invoked as literal argument arrays with server-owned temporary copies — never a shell string built from a filename.' },
  { title: 'Server normalization', body: 'Provider output is decoded against a strict schema that rejects unknown fields. Money becomes exact integer minor units under the versioned money-v1 policy, dates are accepted only as ISO calendar dates, and arithmetic is recomputed where the data allows it.' },
  { title: 'Human review', body: 'The original renders beside editable candidate values, server warnings, source evidence, and the audit trail. Every save creates a new immutable version; approval targets one exact version number and requires an explicit confirmation.' },
  { title: 'Export', body: 'CSV v1 is generated from the approved snapshot with RFC 4180 quoting and CRLF records, identical on every repeat. The webhook export is a durable job signed with HMAC-SHA256, carrying a stable idempotency key so the receiver can deduplicate.' },
];

const checks = [
  { code: 'subtotal_tax_total_mismatch', body: 'Subtotal plus tax does not equal the stated total.' },
  { code: 'line_total_mismatch', body: 'A line total disagrees with quantity × unit price plus line tax.' },
  { code: 'line_items_subtotal_mismatch', body: 'The sum of the line items disagrees with the subtotal.' },
  { code: 'missing_or_invalid_currency', body: 'No usable ISO currency was present, so no amount can be trusted as money.' },
  { code: 'invalid_date', body: 'A date was absent or was not an unambiguous ISO calendar date.' },
  { code: 'invalid_money', body: 'An amount was not a plain decimal the server is willing to convert exactly.' },
];

const limits = [
  'It never pays an invoice, moves money, or connects to a bank.',
  'It is not bookkeeping, double-entry accounting, or tax filing, and it makes no compliance claim.',
  'The default demo runs a deterministic offline extractor. No paid model is required, and none is called.',
  'The local demo uses one fixed server-side actor. There is no authentication, and no multi-user authorization is claimed.',
  'Raster OCR for scanned PDFs is not implemented; image OCR covers JPEG and PNG only.',
  'Webhook delivery is at-least-once. The receiver must deduplicate by the idempotency key.',
];

export function LandingPage(): ReactElement {
  return <PageFrame>
    <main id="main-content" tabIndex={-1}>
      <section className="hero" aria-labelledby="hero-title">
        <div className="hero-copy-column">
          <p className="eyebrow">Invoice data, kept reviewable</p>
          <h1 id="hero-title">Invoices become structured proposals a person still approves.</h1>
          <p className="hero-copy">InvoiceFlow turns a PDF, JPEG, or PNG invoice into normalized, versioned data — and stops there. Extraction is a proposal. A person compares it against the original, corrects it, and approves one exact version before anything is exported.</p>
          <div className="hero-actions">
            <a className="button button-primary" href="/app">Open the workspace</a>
            <a className="button button-quiet" href="#story">See the full path</a>
          </div>
        </div>
        <div className="hero-preview" aria-hidden="true">
          <div className="hero-preview-source">
            <span>Original</span>
            <span className="story-paper-lines" />
          </div>
          <div className="hero-preview-data">
            <span className="hero-preview-heading">Proposal · version 1</span>
            <span className="hero-preview-row"><em>Supplier</em>Orchard Office Supplies</span>
            <span className="hero-preview-row"><em>Invoice</em>ORCHARD-001</span>
            <span className="hero-preview-row"><em>Total</em>24.00 USD</span>
            <span className="hero-preview-foot">Awaiting human review</span>
          </div>
        </div>
      </section>

      <ProvenanceStory />

      <DemoMedia />

      <section id="workflow" className="pipeline" aria-labelledby="pipeline-title">
        <div className="section-intro">
          <p className="eyebrow">What runs on the server</p>
          <h2 id="pipeline-title">Six stages, each with its own failure behavior.</h2>
          <p className="hero-copy">Uploaded files, extracted text, OCR output, and model output are all treated as untrusted input. None of them can set a document state, a storage location, an approval, or an export destination.</p>
        </div>
        <ol className="pipeline-list">
          {pipeline.map((stage, index) => <li key={stage.title}>
            <p className="pipeline-index" aria-hidden="true">{String(index + 1).padStart(2, '0')}</p>
            <div>
              <h3>{stage.title}</h3>
              <p>{stage.body}</p>
            </div>
          </li>)}
        </ol>
      </section>

      <section id="checks" className="checks" aria-labelledby="checks-title">
        <div className="section-intro">
          <p className="eyebrow">Server warnings</p>
          <h2 id="checks-title">The server says what it could not verify.</h2>
          <p className="hero-copy">Warnings are generated on the server from the normalized snapshot and stored with the version that produced them. They report arithmetic and format checks — they do not assert that tax treatment or accounting is correct, and a warning never blocks a person from deciding.</p>
        </div>
        <ul className="check-list">
          {checks.map((check) => <li key={check.code}>
            <code>{check.code}</code>
            <p>{check.body}</p>
          </li>)}
        </ul>
      </section>

      <section id="reliability" className="reliability" aria-labelledby="reliability-title">
        <div className="section-intro">
          <p className="eyebrow">Reliability</p>
          <h2 id="reliability-title">Restarting a process does not lose work.</h2>
        </div>
        <ul className="reliability-list">
          <li><StatusTag tone="info">Durable</StatusTag><p>Processing and export are PostgreSQL jobs, not in-memory queues. A worker that dies mid-attempt loses its lease, and the job is recovered rather than dropped.</p></li>
          <li><StatusTag tone="info">Exact</StatusTag><p>Money is stored as integer minor units with an explicit currency, under a named rounding policy recorded on every snapshot. No binary floating point touches an amount.</p></li>
          <li><StatusTag tone="success">Immutable</StatusTag><p>Versions and audit events reject updates and deletes at the database level. A correction, an approval, and an export each append to the history.</p></li>
          <li><StatusTag tone="warning">Bounded</StatusTag><p>Retries have a ceiling and end in an explicit dead-letter state with a sanitized error summary. Nothing retries forever and nothing fails silently.</p></li>
        </ul>
      </section>

      <section id="limits" className="limits" aria-labelledby="limits-title">
        <div className="section-intro">
          <p className="eyebrow">Honest scope</p>
          <h2 id="limits-title">What this deliberately does not do.</h2>
          <p className="hero-copy">These are current boundaries of the running system, not a roadmap promise.</p>
        </div>
        <ul className="limit-list">
          {limits.map((limit) => <li key={limit}>{limit}</li>)}
        </ul>
      </section>

      <section className="cta" aria-labelledby="cta-title">
        <h2 id="cta-title">Upload a fictional invoice and walk the whole path.</h2>
        <p className="hero-copy">The workspace is the real application: intake, durable processing, the split review screen, approval of an exact version, and both export routes.</p>
        <div className="hero-actions"><a className="button button-primary" href="/app">Open the workspace</a></div>
      </section>
    </main>
  </PageFrame>;
}
