import { useEffect, useMemo, useState, type ChangeEvent, type ReactElement } from 'react';
import { getReviewDocument, rejectReviewDocument, saveHumanReview, type EditableLineItem, type EditableProposal, type ReviewDocument, type ReviewVersion, UploadRequestError } from '../api/documents';
import { StatusTag } from './StatusTag';

const blankLine = (): EditableLineItem => ({ description: '', quantity: '', unit_price: '', tax_amount: '', total: '' });
const blankProposal = (): EditableProposal => ({ supplier_name: '', supplier_email: '', invoice_number: '', issue_date: '', due_date: '', currency: '', subtotal: '', tax_amount: '', total: '', line_items: [] });
const cloneProposal = (proposal: EditableProposal): EditableProposal => ({ ...proposal, line_items: proposal.line_items.map((line) => ({ ...line })) });

export function ReviewWorkspace({ documentID }: { documentID: string }): ReactElement {
  const [document, setDocument] = useState<ReviewDocument>();
  const [error, setError] = useState<string>();
  const [reload, setReload] = useState(0);
  const [proposal, setProposal] = useState<EditableProposal>(blankProposal);
  const [savedProposal, setSavedProposal] = useState<EditableProposal>(blankProposal);
  const [saving, setSaving] = useState(false);
  const [showReject, setShowReject] = useState(false);
  const [rejecting, setRejecting] = useState(false);

  useEffect(() => {
    const controller = new AbortController();
    setDocument(undefined); setError(undefined);
    void getReviewDocument(documentID, controller.signal).then((next) => {
      setDocument(next);
      const editable = next.versions[0]?.editable ?? blankProposal();
      setProposal(cloneProposal(editable)); setSavedProposal(cloneProposal(editable));
    }).catch((requestError: unknown) => {
      if (controller.signal.aborted) return;
      setError(requestError instanceof UploadRequestError ? requestError.message : 'InvoiceFlow could not load this review. Try again.');
    });
    return () => controller.abort();
  }, [documentID, reload]);

  const latest = document?.versions[0];
  const dirty = useMemo(() => JSON.stringify(proposal) !== JSON.stringify(savedProposal), [proposal, savedProposal]);
  useEffect(() => {
    const warn = (event: BeforeUnloadEvent): void => { if (dirty) { event.preventDefault(); event.returnValue = ''; } };
    window.addEventListener('beforeunload', warn); return () => window.removeEventListener('beforeunload', warn);
  }, [dirty]);

  const save = async (): Promise<void> => {
    if (!latest) return;
    setSaving(true); setError(undefined);
    try { await saveHumanReview(documentID, latest.version_number, proposal); setReload((value) => value + 1); }
    catch (requestError: unknown) { setError(requestError instanceof UploadRequestError ? requestError.message : 'InvoiceFlow could not save this correction. Try again.'); }
    finally { setSaving(false); }
  };
  const reject = async (): Promise<void> => {
    setRejecting(true); setError(undefined);
    try { await rejectReviewDocument(documentID); setShowReject(false); setReload((value) => value + 1); }
    catch (requestError: unknown) { setError(requestError instanceof UploadRequestError ? requestError.message : 'InvoiceFlow could not reject this document. Try again.'); }
    finally { setRejecting(false); }
  };

  if (error && !document) return <ReviewMessage tone="danger" title="Review unavailable" message={error} onRetry={() => setReload((value) => value + 1)} />;
  if (!document) return <ReviewMessage tone="info" title="Loading review" message="Loading the immutable extraction proposal and source document." />;
  if (document.status === 'failed') return <ReviewMessage tone="danger" title="Processing failed" message="This document has no review proposal. Reload to check whether processing was retried; retrying processing is not available in Stage 4." onRetry={() => setReload((value) => value + 1)} />;
  if (!latest) return <ReviewMessage tone="warning" title="No review proposal" message="This document is still processing or did not produce an extraction snapshot." onRetry={() => setReload((value) => value + 1)} />;
  const editable = document.status === 'needs_review';
  return <main id="main-content" className="app-main" tabIndex={-1}>
    <header className="page-heading"><div><p className="eyebrow">Human review</p><h1>Compare the source and proposal.</h1><p>Extraction is a proposal. Corrections create a new immutable human-review version.</p></div><StatusTag tone={document.status === 'rejected' ? 'danger' : 'warning'}>{document.status === 'rejected' ? 'Rejected' : 'Needs review'}</StatusTag></header>
    {error ? <p className="review-error" role="alert">{error}</p> : null}
    {dirty ? <p className="review-unsaved" role="status">Unsaved changes. Save a correction before leaving this review.</p> : null}
    <section className="review-workspace" aria-label="Invoice review workspace">
      <SourcePanel documentID={documentID} mediaType={document.media_type} />
      <section className="review-panel" aria-label="Extracted invoice proposal">
        <div className="review-version-bar"><div><p className="eyebrow">Version {latest.version_number}</p><h2>{latest.source === 'human_review' ? 'Human-reviewed proposal' : 'Extracted proposal'}</h2></div><span className={`version-source version-source-${latest.source}`}>{latest.source === 'human_review' ? 'Human edited' : 'AI extracted'}</span></div>
        <ReviewForm value={proposal} disabled={!editable || saving} onChange={setProposal} />
        <ReviewContext version={latest} audit={document.audit} />
        {editable ? <div className="review-actions"><button className="button button-primary" type="button" disabled={saving || !dirty} onClick={() => void save()}>{saving ? 'Saving correction…' : 'Save correction'}</button><button className="button button-danger" type="button" disabled={saving} onClick={() => setShowReject(true)}>Reject document</button></div> : <p className="review-readonly" role="status">This document is rejected. Its extraction and review versions remain read-only.</p>}
      </section>
    </section>
    {showReject ? <section className="confirm-dialog" role="dialog" aria-modal="true" aria-labelledby="reject-title"><h2 id="reject-title">Reject this document?</h2><p>This is a terminal Stage 4 transition. It does not delete the source or review history.</p><div className="review-actions"><button className="button button-danger" type="button" disabled={rejecting} onClick={() => void reject()}>{rejecting ? 'Rejecting…' : 'Confirm rejection'}</button><button className="button button-quiet" type="button" disabled={rejecting} autoFocus onClick={() => setShowReject(false)}>Cancel</button></div></section> : null}
  </main>;
}

function SourcePanel({ documentID, mediaType }: { documentID: string; mediaType: string }): ReactElement {
  const source = `/api/v1/documents/${documentID}/source`;
  return <section className="source-panel" aria-label="Original source document"><div className="source-heading"><p className="eyebrow">Original source</p><span>Read-only</span></div>{mediaType === 'application/pdf' ? <object className="source-object" data={source} type="application/pdf" aria-label="Original invoice PDF"><a href={source}>Open the original PDF</a></object> : <img className="source-image" src={source} alt="Original invoice" />}</section>;
}

function ReviewForm({ value, disabled, onChange }: { value: EditableProposal; disabled: boolean; onChange: (proposal: EditableProposal) => void }): ReactElement {
  const field = (name: Exclude<keyof EditableProposal, 'line_items'>, label: string, type = 'text'): ReactElement => <label className="review-field"><span>{label}</span><input type={type} value={value[name]} disabled={disabled} onChange={(event) => onChange({ ...value, [name]: event.currentTarget.value })} /></label>;
  const lineChange = (index: number, key: keyof EditableLineItem, event: ChangeEvent<HTMLInputElement>): void => { const lines = value.line_items.map((line, lineIndex) => lineIndex === index ? { ...line, [key]: event.currentTarget.value } : line); onChange({ ...value, line_items: lines }); };
  return <form className="review-form" onSubmit={(event) => event.preventDefault()} aria-label="Editable invoice proposal"><fieldset disabled={disabled}><legend>Invoice metadata</legend><div className="review-grid">{field('supplier_name', 'Supplier name')}{field('supplier_email', 'Supplier email', 'email')}{field('invoice_number', 'Invoice number')}{field('issue_date', 'Issue date', 'date')}{field('due_date', 'Due date', 'date')}{field('currency', 'Currency')}</div><div className="review-grid review-money">{field('subtotal', 'Subtotal')}{field('tax_amount', 'Tax amount')}{field('total', 'Total')}</div><div className="line-items-heading"><h3>Line items</h3><button className="button button-quiet" type="button" onClick={() => onChange({ ...value, line_items: [...value.line_items, blankLine()] })}>Add line item</button></div>{value.line_items.length === 0 ? <p className="field-note">No line items were extracted.</p> : <ol className="line-items">{value.line_items.map((line, index) => <li key={index}><div className="review-grid line-item-grid"><label className="review-field"><span>Description</span><input value={line.description} onChange={(event) => lineChange(index, 'description', event)} /></label><label className="review-field"><span>Quantity</span><input value={line.quantity} inputMode="decimal" onChange={(event) => lineChange(index, 'quantity', event)} /></label><label className="review-field"><span>Unit price</span><input value={line.unit_price} inputMode="decimal" onChange={(event) => lineChange(index, 'unit_price', event)} /></label><label className="review-field"><span>Tax amount</span><input value={line.tax_amount} inputMode="decimal" onChange={(event) => lineChange(index, 'tax_amount', event)} /></label><label className="review-field"><span>Line total</span><input value={line.total} inputMode="decimal" onChange={(event) => lineChange(index, 'total', event)} /></label></div><button className="remove-line" type="button" onClick={() => onChange({ ...value, line_items: value.line_items.filter((_, lineIndex) => lineIndex !== index) })}>Remove line item {index + 1}</button></li>)}</ol>}</fieldset></form>;
}

function ReviewContext({ version, audit }: { version: ReviewVersion; audit: ReviewDocument['audit'] }): ReactElement {
  return <div className="review-context"><section aria-labelledby="warnings-title"><h3 id="warnings-title">Server validation warnings</h3>{version.warnings.length ? <ul className="warning-list">{version.warnings.map((warning, index) => <li key={`${warning.code}-${index}`}><strong>{warning.field}</strong><span>{warning.message}</span></li>)}</ul> : <p>No server validation warnings on this version.</p>}</section><section aria-labelledby="evidence-title"><h3 id="evidence-title">Source evidence</h3>{version.evidence.length ? <ul className="evidence-list">{version.evidence.map((evidence, index) => <li key={`${evidence.field}-${index}`}><strong>{evidence.field}, page {evidence.page_number}</strong><q>{evidence.excerpt}</q></li>)}</ul> : <p>No source evidence was supplied.</p>}</section><section aria-labelledby="diagnostics-title"><h3 id="diagnostics-title">Sanitized diagnostics</h3>{version.diagnostics.length ? <ul>{version.diagnostics.map((diagnostic, index) => <li key={`${diagnostic.code}-${index}`}><strong>{diagnostic.code}</strong>: {diagnostic.message}</li>)}</ul> : <p>No diagnostics were retained.</p>}</section><section aria-labelledby="audit-title"><h3 id="audit-title">Audit history</h3><ol className="audit-list">{audit.map((event) => <li key={event.sequence}><strong>{event.action.replaceAll('_', ' ')}</strong><span>{new Date(event.occurred_at).toLocaleString()} · {event.actor}</span></li>)}</ol></section></div>;
}

function ReviewMessage({ tone, title, message, onRetry }: { tone: 'info' | 'warning' | 'danger'; title: string; message: string; onRetry?: () => void }): ReactElement {
  return <main id="main-content" className="app-main" tabIndex={-1}><section className="review-message"><StatusTag tone={tone}>{title}</StatusTag><h1>{title}</h1><p>{message}</p>{onRetry ? <button className="button button-quiet" type="button" onClick={onRetry}>Reload review</button> : null}</section></main>;
}
