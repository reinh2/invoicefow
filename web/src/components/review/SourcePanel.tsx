import type { ReactElement } from 'react';

/* The original document, always visible beside the proposal. It is read-only by
   construction: the route streams stored bytes and exposes no filename, storage
   key, or filesystem path. */
export function SourcePanel({
  documentID,
  mediaType,
}: {
  documentID: string;
  mediaType: string;
}): ReactElement {
  const source = `/api/v1/documents/${documentID}/source`;
  return (
    <section className="source-panel" aria-label="Original source document">
      <div className="source-heading">
        <p className="eyebrow">Original source</p>
        <span>Read-only</span>
      </div>
      {mediaType === 'application/pdf' ? (
        <object
          className="source-object"
          data={source}
          type="application/pdf"
          aria-label="Original invoice PDF"
        >
          <a href={source}>Open the original PDF</a>
        </object>
      ) : (
        <img className="source-image" src={source} alt="Original invoice" />
      )}
    </section>
  );
}
