import { useCallback, useEffect, useState, type ReactElement } from 'react';
import {
  formatMinorUnits,
  listDocuments,
  UploadRequestError,
  type DocumentSummary,
} from '../api/documents';
import { StatusTag, type StatusTone } from './StatusTag';

/* Before this list existed a document was reachable only through the URL
   returned at upload time, so closing the tab lost it. The list is deliberately
   read-only: it opens a document, and every state change still happens on the
   review screen behind its own confirmation. */

const statusTones: Record<DocumentSummary['status'], StatusTone> = {
  queued: 'info',
  processing: 'info',
  needs_review: 'warning',
  approved: 'success',
  exported: 'success',
  rejected: 'danger',
  failed: 'danger',
};

const statusLabels: Record<DocumentSummary['status'], string> = {
  queued: 'Queued',
  processing: 'Processing',
  needs_review: 'Needs review',
  approved: 'Approved',
  exported: 'Exported',
  rejected: 'Rejected',
  failed: 'Failed',
};

export function DocumentList({
  onOpenDocument,
  reloadToken,
}: {
  onOpenDocument?: (documentID: string) => void;
  reloadToken?: number;
}): ReactElement {
  const [documents, setDocuments] = useState<DocumentSummary[]>();
  const [nextCursor, setNextCursor] = useState<string>();
  const [error, setError] = useState<string>();
  const [loadingMore, setLoadingMore] = useState(false);
  const [reload, setReload] = useState(0);

  useEffect(() => {
    const controller = new AbortController();
    void listDocuments(undefined, controller.signal)
      .then((page) => {
        setDocuments(page.documents);
        setNextCursor(page.next_cursor);
        setError(undefined);
      })
      .catch((requestError: unknown) => {
        if (controller.signal.aborted) return;
        setError(
          requestError instanceof UploadRequestError
            ? requestError.message
            : 'InvoiceFlow could not load the document list.',
        );
      });
    return () => controller.abort();
  }, [reload, reloadToken]);

  const loadMore = useCallback((): void => {
    if (!nextCursor) return;
    setLoadingMore(true);
    setError(undefined);
    void listDocuments(nextCursor)
      .then((page) => {
        // Append rather than replace: the keyset cursor guarantees these rows come
        // strictly after the ones already shown.
        setDocuments((current) => [...(current ?? []), ...page.documents]);
        setNextCursor(page.next_cursor);
      })
      .catch((requestError: unknown) => {
        setError(
          requestError instanceof UploadRequestError
            ? requestError.message
            : 'InvoiceFlow could not load more documents.',
        );
      })
      .finally(() => setLoadingMore(false));
  }, [nextCursor]);

  if (error && documents === undefined) {
    return (
      <section className="document-list" aria-labelledby="documents-title">
        <h2 id="documents-title">Documents</h2>
        <p className="review-error" role="alert">
          {error}
        </p>
        <button
          className="button button-quiet"
          type="button"
          onClick={() => setReload((value) => value + 1)}
        >
          Reload documents
        </button>
      </section>
    );
  }

  if (documents === undefined) {
    return (
      <section className="document-list" aria-labelledby="documents-title">
        <h2 id="documents-title">Documents</h2>
        <p role="status">Loading documents…</p>
      </section>
    );
  }

  return (
    <section className="document-list" aria-labelledby="documents-title">
      <h2 id="documents-title">Documents</h2>
      {error ? (
        <p className="review-error" role="alert">
          {error}
        </p>
      ) : null}
      {documents.length === 0 ? (
        <p className="field-note">No documents yet. Upload one above to start a review.</p>
      ) : (
        <table className="document-table">
          <caption className="visually-hidden">Uploaded documents, newest first</caption>
          <thead>
            <tr>
              <th scope="col">Supplier</th>
              <th scope="col">Invoice</th>
              <th scope="col">Total</th>
              <th scope="col">Status</th>
              <th scope="col">Uploaded</th>
            </tr>
          </thead>
          <tbody>
            {documents.map((document) => {
              const total = formatMinorUnits(document.total_minor, document.currency);
              return (
                <tr key={document.id}>
                  <th scope="row">
                    <a
                      href={`/app/documents/${document.id}`}
                      onClick={(event) => {
                        if (
                          !onOpenDocument ||
                          event.metaKey ||
                          event.ctrlKey ||
                          event.shiftKey ||
                          event.button !== 0
                        )
                          return;
                        event.preventDefault();
                        onOpenDocument(document.id);
                      }}
                    >
                      {document.supplier_name || 'Untitled document'}
                    </a>
                  </th>
                  <td>{document.invoice_number || '—'}</td>
                  <td className="document-total">{total || '—'}</td>
                  <td>
                    <StatusTag tone={statusTones[document.status]}>
                      {statusLabels[document.status]}
                    </StatusTag>
                  </td>
                  <td>
                    <time dateTime={document.created_at}>
                      {new Date(document.created_at).toLocaleString()}
                    </time>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      )}
      {nextCursor ? (
        <button
          className="button button-quiet"
          type="button"
          disabled={loadingMore}
          onClick={loadMore}
        >
          {loadingMore ? 'Loading…' : 'Load more documents'}
        </button>
      ) : null}
    </section>
  );
}
