import { PageFrame } from '../../components/PageFrame';
import { StatusTag } from '../../components/StatusTag';
import type { ReactElement } from 'react';

export function LandingPage(): ReactElement {
  return <PageFrame>
    <main id="main-content" tabIndex={-1}>
      <section className="hero" aria-labelledby="hero-title">
        <p className="eyebrow">Invoice data, kept reviewable</p>
        <h1 id="hero-title">A deliberate place to turn invoices into structured proposals.</h1>
        <p className="hero-copy">InvoiceFlow is being built for human review. Extraction results are proposals—not financial authority.</p>
        <div className="hero-actions"><a className="button button-primary" href="/app">Open the foundation</a><a className="button button-quiet" href="#principles">How it is designed</a></div>
      </section>
      <section id="principles" className="principles" aria-labelledby="principles-title">
        <div><p className="eyebrow">Groundwork</p><h2 id="principles-title">The workflow is intentionally not connected yet.</h2></div>
        <ul className="principle-list">
          <li><StatusTag tone="info">Source retained</StatusTag><p>The original document will remain alongside a reviewable proposal.</p></li>
          <li><StatusTag tone="warning">Human control</StatusTag><p>Warnings and corrections will be explicit before any future approval step.</p></li>
          <li><StatusTag tone="neutral">Foundation stage</StatusTag><p>This shell contains no uploads, extraction results, metrics, or exports.</p></li>
        </ul>
      </section>
    </main>
  </PageFrame>;
}
