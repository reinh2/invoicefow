import { useEffect, useMemo, useRef, useState, type ChangeEvent, type ReactElement } from 'react';
import { approveReviewDocument, downloadCSV, getReviewDocument, rejectReviewDocument, saveHumanReview, triggerWebhookExport, type EditableLineItem, type EditableProposal, type ExportRecord, type ReviewDocument, type ReviewVersion, UploadRequestError } from '../api/documents';
import { StatusTag, type StatusTone } from './StatusTag';
import { useReducedMotion } from '../motion/useReducedMotion';

const blankLine = (): EditableLineItem => ({ description: '', quantity: '', unit_price: '', tax_amount: '', total: '' });
const blankProposal = (): EditableProposal => ({ supplier_name: '', supplier_email: '', invoice_number: '', issue_date: '', due_date: '', currency: '', subtotal: '', tax_amount: '', total: '', line_items: [] });
const cloneProposal = (proposal: EditableProposal): EditableProposal => ({ ...proposal, line_items: proposal.line_items.map((line) => ({ ...line })) });
const webhookStatusRefreshIntervalMs = 1500;
const maxWebhookStatusRefreshes = 10;

export function ReviewWorkspace({ documentID }: { documentID: string }): ReactElement {
  const [document, setDocument] = useState<ReviewDocument>();
  const [error, setError] = useState<string>();
  const [reload, setReload] = useState(0);
  const [proposal, setProposal] = useState<EditableProposal>(blankProposal);
  const [savedProposal, setSavedProposal] = useState<EditableProposal>(blankProposal);
  const [saving, setSaving] = useState(false);
  const [showReject, setShowReject] = useState(false);
  const [rejecting, setRejecting] = useState(false);
  const [showApprove, setShowApprove] = useState(false);
  const [approving, setApproving] = useState(false);
  const [showWebhookConfirm, setShowWebhookConfirm] = useState(false);
  const [showCSVConfirm, setShowCSVConfirm] = useState(false);
	const [exportingWebhook, setExportingWebhook] = useState(false);
	const [exportingCSV, setExportingCSV] = useState(false);
	const [webhookMessage, setWebhookMessage] = useState<string>();
	const [csvMessage, setCSVMessage] = useState<string>();
	const [watchedWebhookExportID, setWatchedWebhookExportID] = useState<string>();
	const [webhookStatusRefreshes, setWebhookStatusRefreshes] = useState(0);
	const [webhookRefreshError, setWebhookRefreshError] = useState<string>();

	useEffect(() => {
		const controller = new AbortController();
		const hasExistingDocument = document !== undefined;
		setError(undefined);
		void getReviewDocument(documentID, controller.signal).then((next) => {
			setDocument(next);
			setWebhookRefreshError(undefined);
			const editable = next.versions[0]?.editable ?? blankProposal();
			setProposal(cloneProposal(editable)); setSavedProposal(cloneProposal(editable));
		}).catch((requestError: unknown) => {
			if (controller.signal.aborted) return;
			const message = requestError instanceof UploadRequestError ? requestError.message : 'InvoiceFlow could not load this review. Try again.';
			if (hasExistingDocument) setWebhookRefreshError(message);
			else setError(message);
		});
		return () => controller.abort();
	}, [documentID, reload]);

  const latest = document?.versions[0];
  const dirty = useMemo(() => JSON.stringify(proposal) !== JSON.stringify(savedProposal), [proposal, savedProposal]);
	useEffect(() => {
		const warn = (event: BeforeUnloadEvent): void => { if (dirty) { event.preventDefault(); event.returnValue = ''; } };
		window.addEventListener('beforeunload', warn); return () => window.removeEventListener('beforeunload', warn);
	}, [dirty]);
	const watchedWebhookExport = document?.exports?.find((record) => record.id === watchedWebhookExportID);
	const watchedWebhookTerminal = watchedWebhookExport?.status === 'succeeded' || watchedWebhookExport?.status === 'failed' || watchedWebhookExport?.status === 'dead_letter';
	useEffect(() => {
		if (!watchedWebhookExportID || watchedWebhookTerminal || dirty || webhookStatusRefreshes >= maxWebhookStatusRefreshes) return;
		const timer = window.setTimeout(() => {
			setWebhookStatusRefreshes((value) => value + 1);
			setReload((value) => value + 1);
		}, webhookStatusRefreshIntervalMs);
		return () => window.clearTimeout(timer);
	}, [dirty, watchedWebhookExportID, watchedWebhookTerminal, webhookStatusRefreshes]);

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
  const approve = async (): Promise<void> => {
    if (!latest) return;
    setApproving(true); setError(undefined);
    try { await approveReviewDocument(documentID, latest.version_number); setShowApprove(false); setReload((value) => value + 1); }
    catch (requestError: unknown) { setError(requestError instanceof UploadRequestError ? requestError.message : 'InvoiceFlow could not approve this version. Try again.'); }
    finally { setApproving(false); }
  };
	const handleWebhookExport = async (): Promise<void> => {
		setExportingWebhook(true); setError(undefined); setWebhookRefreshError(undefined); setWebhookMessage(undefined);
		try {
			const rec = await triggerWebhookExport(documentID);
			setWatchedWebhookExportID(rec.id);
			setWebhookStatusRefreshes(0);
			setWebhookMessage('Webhook export queued. Waiting for the worker to report delivery status.');
			setReload((value) => value + 1);
		} catch (requestError: unknown) {
			setError(requestError instanceof UploadRequestError ? requestError.message : 'InvoiceFlow could not trigger webhook export. Try again.');
		} finally { setExportingWebhook(false); }
	};
	const refreshWebhookStatus = (): void => {
		setWebhookRefreshError(undefined);
		setWebhookStatusRefreshes(0);
		setReload((value) => value + 1);
	};
  const handleCSVExport = async (): Promise<void> => {
    setExportingCSV(true); setError(undefined); setCSVMessage(undefined);
    try { await downloadCSV(documentID); setCSVMessage(`CSV v1 downloaded for approved version ${document?.approved_version_number ?? latest?.version_number}.`); setShowCSVConfirm(false); setReload((value) => value + 1); }
    catch (requestError: unknown) { setError(requestError instanceof UploadRequestError ? requestError.message : 'InvoiceFlow could not create the CSV export. Try again.'); }
    finally { setExportingCSV(false); }
  };

  if (error && !document) return <ReviewMessage tone="danger" title="Review unavailable" message={error} onRetry={() => setReload((value) => value + 1)} />;
  if (!document) return <ReviewMessage tone="info" title="Loading review" message="Loading the immutable extraction proposal and source document." />;
  if (document.status === 'failed') return <ReviewMessage tone="danger" title="Processing failed" message="This document has no review proposal." onRetry={() => setReload((value) => value + 1)} />;
  if (!latest) return <ReviewMessage tone="warning" title="No review proposal" message="This document is still processing or did not produce an extraction snapshot." onRetry={() => setReload((value) => value + 1)} />;

  const editable = document.status === 'needs_review';
  const approvedOrExported = document.status === 'approved' || document.status === 'exported';

	const statusTone: StatusTone = document.status === 'rejected' ? 'danger' : (document.status === 'approved' || document.status === 'exported') ? 'success' : 'warning';
	const statusLabel = document.status === 'rejected' ? 'Rejected' : document.status === 'approved' ? 'Approved' : document.status === 'exported' ? 'Exported' : 'Needs review';
	const webhookLifecycleMessage = webhookStatusMessage(watchedWebhookExport, webhookStatusRefreshes, watchedWebhookExportID !== undefined) ?? webhookMessage;

  return <main id="main-content" className="app-main" tabIndex={-1}>
    <header className="page-heading"><div><p className="eyebrow">Human review & export</p><h1>Compare the source and proposal.</h1><p>Corrections create immutable review versions. Explicit approval enables CSV & Webhook export.</p></div><StatusTag tone={statusTone}>{statusLabel}</StatusTag></header>
    {error ? <p className="review-error" role="alert">{error}</p> : null}
    {dirty ? <p className="review-unsaved" role="status">Unsaved changes. Save a correction before leaving or approving this review.</p> : null}
		{webhookLifecycleMessage ? <p className="review-success" role="status">{webhookLifecycleMessage}</p> : null}
		{csvMessage ? <p className="review-success" role="status">{csvMessage}</p> : null}
		{webhookRefreshError ? <p className="review-error" role="alert">Webhook status could not be refreshed: {webhookRefreshError}</p> : null}
    <section className="review-workspace" aria-label="Invoice review workspace">
      <SourcePanel documentID={documentID} mediaType={document.media_type} />
      <section className="review-panel" aria-label="Extracted invoice proposal">
        <div className="review-version-bar"><div><p className="eyebrow">Version {latest.version_number}</p><h2>{latest.source === 'human_review' ? 'Human-reviewed proposal' : 'Extracted proposal'}</h2></div><span className={`version-source version-source-${latest.source}`}>{latest.source === 'human_review' ? 'Human edited' : 'AI extracted'}</span></div>
        <ReviewForm value={proposal} disabled={!editable || saving} onChange={setProposal} />
        <ReviewContext version={latest} audit={document.audit} exports={document.exports} />
        {editable ? <div className="review-actions">
          <button className="button button-primary" type="button" disabled={saving || !dirty} onClick={() => void save()}>{saving ? 'Saving correction…' : 'Save correction'}</button>
          <button className="button button-primary" type="button" disabled={saving || dirty} onClick={() => setShowApprove(true)}>Approve version {latest.version_number}</button>
          <button className="button button-danger" type="button" disabled={saving} onClick={() => setShowReject(true)}>Reject document</button>
		</div> : approvedOrExported ? <div className="review-export-actions">
		  <p className="eyebrow">Approved for Export (Version {document.approved_version_number ?? latest.version_number})</p>
		  <div className="review-actions">
			<button className="button button-primary" type="button" disabled={exportingCSV} onClick={() => setShowCSVConfirm(true)}>Download CSV Export</button>
			<button className="button button-primary" type="button" disabled={exportingWebhook} onClick={() => setShowWebhookConfirm(true)}>{exportingWebhook ? 'Enqueuing Webhook…' : 'Send Webhook Export'}</button>
			{watchedWebhookExportID ? <button className="button button-quiet" type="button" disabled={exportingWebhook} onClick={refreshWebhookStatus}>Refresh webhook status</button> : null}
		  </div>
        </div> : <p className="review-readonly" role="status">This document is rejected. Its extraction and review versions remain read-only.</p>}
      </section>
    </section>
    {showApprove ? <ConfirmDialog title={`Approve Version ${latest.version_number}?`} onClose={() => setShowApprove(false)} confirmLabel={approving ? 'Approving…' : 'Confirm approval'} onConfirm={() => void approve()} disabled={approving}><p>Approving version {latest.version_number} locks this invoice and creates an immutable approved record for CSV and Webhook export. This action cannot be undone.</p></ConfirmDialog> : null}
    {showCSVConfirm ? <ConfirmDialog title="Download CSV Export?" onClose={() => setShowCSVConfirm(false)} confirmLabel={exportingCSV ? 'Creating CSV…' : 'Confirm CSV export'} onConfirm={() => void handleCSVExport()} disabled={exportingCSV}><p>Creates the versioned InvoiceFlow CSV v1 from approved version {document.approved_version_number ?? latest.version_number}. The download is deterministic and uses exact server-normalized money.</p></ConfirmDialog> : null}
    {showWebhookConfirm ? <ConfirmDialog title="Send Webhook Export?" onClose={() => setShowWebhookConfirm(false)} confirmLabel={exportingWebhook ? 'Enqueuing…' : 'Confirm webhook export'} onConfirm={() => { setShowWebhookConfirm(false); void handleWebhookExport(); }} disabled={exportingWebhook}><p>Enqueues a durable HMAC-SHA256 webhook job for approved version {document.approved_version_number ?? latest.version_number}.</p><p>Destination: Server-configured webhook. The full URL and secret are never shown.</p></ConfirmDialog> : null}
	{showReject ? <ConfirmDialog title="Reject this document?" onClose={() => setShowReject(false)} confirmLabel={rejecting ? 'Rejecting…' : 'Confirm rejection'} onConfirm={() => void reject()} disabled={rejecting} danger><p>This is a terminal transition. It does not delete the source or review history.</p></ConfirmDialog> : null}
	</main>;
}

function webhookStatusMessage(record: ExportRecord | undefined, refreshes: number, watching: boolean): string | undefined {
	if (!watching) return undefined;
	if (!record) return 'Webhook export queued. Waiting for the worker to report delivery status.';
	const retryLimitMessage = refreshes >= maxWebhookStatusRefreshes ? ' Automatic refresh paused; use Refresh webhook status to check again.' : '';
	switch (record.status) {
		case 'pending':
			return `Webhook delivery is pending (attempt ${record.attempts}).${retryLimitMessage}`;
		case 'retrying':
			return `Webhook delivery is retrying after attempt ${record.attempts}.${retryLimitMessage}`;
		case 'succeeded':
			return `Webhook export succeeded after ${record.attempts} attempt${record.attempts === 1 ? '' : 's'}.`;
		case 'failed':
			return `Webhook export failed.${record.error_summary ? ` ${record.error_summary}` : ''}`;
		case 'dead_letter':
			return `Webhook export could not be delivered after ${record.attempts} attempt${record.attempts === 1 ? '' : 's'}.${record.error_summary ? ` ${record.error_summary}` : ''}`;
	}
}

function SourcePanel({ documentID, mediaType }: { documentID: string; mediaType: string }): ReactElement {
  const source = `/api/v1/documents/${documentID}/source`;
  return <section className="source-panel" aria-label="Original source document"><div className="source-heading"><p className="eyebrow">Original source</p><span>Read-only</span></div>{mediaType === 'application/pdf' ? <object className="source-object" data={source} type="application/pdf" aria-label="Original invoice PDF"><a href={source}>Open the original PDF</a></object> : <img className="source-image" src={source} alt="Original invoice" />}</section>;
}

function ConfirmDialog({ title, children, confirmLabel, onConfirm, onClose, disabled, danger = false }: { title: string; children: ReactElement | ReactElement[]; confirmLabel: string; onConfirm: () => void; onClose: () => void; disabled: boolean; danger?: boolean }): ReactElement {
  const dialogRef = useRef<HTMLElement>(null);
  const reducedMotion = useReducedMotion();
  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null;
    const dialog = dialogRef.current;
    const focusables = (): HTMLElement[] => dialog ? Array.from(dialog.querySelectorAll<HTMLElement>('button:not([disabled]), [href], input:not([disabled]), [tabindex]:not([tabindex="-1"])')) : [];
    focusables()[0]?.focus();
    const keydown = (event: KeyboardEvent): void => {
      if (event.key === 'Escape') { event.preventDefault(); if (!disabled) onClose(); return; }
      if (event.key !== 'Tab') return;
      const elements = focusables();
      if (!elements.length) return;
      const first = elements[0]; const last = elements[elements.length - 1];
      if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus(); }
      else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus(); }
    };
    document.addEventListener('keydown', keydown);
    return () => { document.removeEventListener('keydown', keydown); previous?.focus(); };
  }, [disabled, onClose]);
  return <section ref={dialogRef} className={`confirm-dialog${reducedMotion ? ' confirm-dialog-reduced-motion' : ''}`} role="dialog" aria-modal="true" aria-labelledby="confirm-title" tabIndex={-1}><h2 id="confirm-title">{title}</h2>{children}<div className="review-actions"><button className={`button ${danger ? 'button-danger' : 'button-primary'}`} type="button" disabled={disabled} onClick={onConfirm}>{confirmLabel}</button><button className="button button-quiet" type="button" disabled={disabled} onClick={onClose}>Cancel</button></div></section>;
}

function ReviewForm({ value, disabled, onChange }: { value: EditableProposal; disabled: boolean; onChange: (proposal: EditableProposal) => void }): ReactElement {
  const field = (name: Exclude<keyof EditableProposal, 'line_items'>, label: string, type = 'text'): ReactElement => <label className="review-field"><span>{label}</span><input type={type} value={value[name]} disabled={disabled} onChange={(event) => onChange({ ...value, [name]: event.currentTarget.value })} /></label>;
  const lineChange = (index: number, key: keyof EditableLineItem, event: ChangeEvent<HTMLInputElement>): void => { const lines = value.line_items.map((line, lineIndex) => lineIndex === index ? { ...line, [key]: event.currentTarget.value } : line); onChange({ ...value, line_items: lines }); };
  return <form className="review-form" onSubmit={(event) => event.preventDefault()} aria-label="Editable invoice proposal"><fieldset disabled={disabled}><legend>Invoice metadata</legend><div className="review-grid">{field('supplier_name', 'Supplier name')}{field('supplier_email', 'Supplier email', 'email')}{field('invoice_number', 'Invoice number')}{field('issue_date', 'Issue date', 'date')}{field('due_date', 'Due date', 'date')}{field('currency', 'Currency')}</div><div className="review-grid review-money">{field('subtotal', 'Subtotal')}{field('tax_amount', 'Tax amount')}{field('total', 'Total')}</div><div className="line-items-heading"><h3>Line items</h3><button className="button button-quiet" type="button" onClick={() => onChange({ ...value, line_items: [...value.line_items, blankLine()] })}>Add line item</button></div>{value.line_items.length === 0 ? <p className="field-note">No line items were extracted.</p> : <ol className="line-items">{value.line_items.map((line, index) => <li key={index}><div className="review-grid line-item-grid"><label className="review-field"><span>Description</span><input value={line.description} onChange={(event) => lineChange(index, 'description', event)} /></label><label className="review-field"><span>Quantity</span><input value={line.quantity} inputMode="decimal" onChange={(event) => lineChange(index, 'quantity', event)} /></label><label className="review-field"><span>Unit price</span><input value={line.unit_price} inputMode="decimal" onChange={(event) => lineChange(index, 'unit_price', event)} /></label><label className="review-field"><span>Tax amount</span><input value={line.tax_amount} inputMode="decimal" onChange={(event) => lineChange(index, 'tax_amount', event)} /></label><label className="review-field"><span>Line total</span><input value={line.total} inputMode="decimal" onChange={(event) => lineChange(index, 'total', event)} /></label></div><button className="remove-line" type="button" onClick={() => onChange({ ...value, line_items: value.line_items.filter((_, lineIndex) => lineIndex !== index) })}>Remove line item {index + 1}</button></li>)}</ol>}</fieldset></form>;
}

function ReviewContext({ version, audit, exports }: { version: ReviewVersion; audit: ReviewDocument['audit']; exports?: ReviewDocument['exports'] }): ReactElement {
	const auditVersion = (payload: unknown): string => typeof payload === 'object' && payload !== null && 'version_number' in payload && typeof payload.version_number === 'number' ? ` · version ${payload.version_number}` : '';
	return <div className="review-context"><section aria-labelledby="warnings-title"><h3 id="warnings-title">Server validation warnings</h3>{version.warnings.length ? <ul className="warning-list">{version.warnings.map((warning, index) => <li key={`${warning.code}-${index}`}><strong>{warning.field}</strong><span>{warning.message}</span></li>)}</ul> : <p>No server validation warnings on this version.</p>}</section><section aria-labelledby="evidence-title"><h3 id="evidence-title">Source evidence</h3>{version.evidence.length ? <ul className="evidence-list">{version.evidence.map((evidence, index) => <li key={`${evidence.field}-${index}`}><strong>{evidence.field}, page {evidence.page_number}</strong><q>{evidence.excerpt}</q></li>)}</ul> : <p>No source evidence was supplied.</p>}</section><section aria-labelledby="diagnostics-title"><h3 id="diagnostics-title">Sanitized diagnostics</h3>{version.diagnostics.length ? <ul>{version.diagnostics.map((diagnostic, index) => <li key={`${diagnostic.code}-${index}`}><strong>{diagnostic.code}</strong>: {diagnostic.message}</li>)}</ul> : <p>No diagnostics were retained.</p>}</section><section aria-labelledby="exports-title"><h3 id="exports-title">Export records</h3>{exports?.length ? <ul className="exports-list">{exports.map((exp) => <li key={exp.id}><strong>{exp.export_type.toUpperCase()} ({exp.status})</strong><span>{exp.destination_label} · attempt {exp.attempts} · {new Date(exp.created_at).toLocaleString()}</span>{exp.next_attempt_at ? <span role="status">Retry scheduled for {new Date(exp.next_attempt_at).toLocaleString()}</span> : null}{exp.error_summary ? <p className="export-error-summary">{exp.error_summary}</p> : null}</li>)}</ul> : <p>No export records yet.</p>}</section><section aria-labelledby="audit-title"><h3 id="audit-title">Audit history</h3><ol className="audit-list">{audit.map((event) => <li key={event.sequence}><strong>{event.action.replaceAll('_', ' ')}{auditVersion(event.payload)}</strong><span>{new Date(event.occurred_at).toLocaleString()} · {event.actor}</span></li>)}</ol></section></div>;
}

function ReviewMessage({ tone, title, message, onRetry }: { tone: 'info' | 'warning' | 'danger'; title: string; message: string; onRetry?: () => void }): ReactElement {
  return <main id="main-content" className="app-main" tabIndex={-1}><section className="review-message"><StatusTag tone={tone}>{title}</StatusTag><h1>{title}</h1><p>{message}</p>{onRetry ? <button className="button button-quiet" type="button" onClick={onRetry}>Reload review</button> : null}</section></main>;
}
