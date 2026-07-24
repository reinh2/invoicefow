import type { ReactElement } from 'react';
import { StatusTag } from '../StatusTag';

/* The whole-screen states the review can be in when there is no proposal to
   show: loading, load failure, processing failure, or a document that produced
   no extraction snapshot. Each one is stated plainly rather than rendered as an
   empty form the reviewer might mistake for extracted data. */
export function ReviewMessage({
  tone,
  title,
  message,
  onRetry,
}: {
  tone: 'info' | 'warning' | 'danger';
  title: string;
  message: string;
  onRetry?: () => void;
}): ReactElement {
  return (
    <main id="main-content" className="app-main" tabIndex={-1}>
      <section className="review-message">
        <StatusTag tone={tone}>{title}</StatusTag>
        <h1>{title}</h1>
        <p>{message}</p>
        {onRetry ? (
          <button className="button button-quiet" type="button" onClick={onRetry}>
            Reload review
          </button>
        ) : null}
      </section>
    </main>
  );
}
