import { useEffect, useMemo, useState, type ReactElement } from 'react';
import {
  approveReviewDocument,
  downloadCSV,
  getReviewDocument,
  rejectReviewDocument,
  saveHumanReview,
  triggerWebhookExport,
  UploadRequestError,
  type EditableProposal,
  type ExportRecord,
  type ReviewDocument,
} from '../api/documents';
import { StatusTag, type StatusTone } from './StatusTag';
import { ConfirmDialog } from './review/ConfirmDialog';
import { ReviewContext } from './review/ReviewContext';
import { ReviewForm } from './review/ReviewForm';
import { ReviewMessage } from './review/ReviewMessage';
import { SourcePanel } from './review/SourcePanel';

/* This component owns review state and the transitions a human can trigger.
   Presentation lives in ./review/*: the source panel, the editable form and its
   per-field warnings, the derived server context, and the confirmation dialog
   every irreversible action goes through. */

const blankProposal = (): EditableProposal => ({
  supplier_name: '',
  supplier_email: '',
  invoice_number: '',
  issue_date: '',
  due_date: '',
  currency: '',
  subtotal: '',
  tax_amount: '',
  total: '',
  line_items: [],
});

const cloneProposal = (proposal: EditableProposal): EditableProposal => ({
  ...proposal,
  line_items: proposal.line_items.map((line) => ({ ...line })),
});

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
    void getReviewDocument(documentID, controller.signal)
      .then((next) => {
        setDocument(next);
        setError(undefined);
        setWebhookRefreshError(undefined);
        const editable = next.versions[0]?.editable ?? blankProposal();
        setProposal(cloneProposal(editable));
        setSavedProposal(cloneProposal(editable));
      })
      .catch((requestError: unknown) => {
        if (controller.signal.aborted) return;
        const message =
          requestError instanceof UploadRequestError
            ? requestError.message
            : 'InvoiceFlow could not load this review. Try again.';
        // A failed background refresh must not blank out a review the user is
        // already working in.
        if (hasExistingDocument) setWebhookRefreshError(message);
        else setError(message);
      });
    return () => controller.abort();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [documentID, reload]);

  const latest = document?.versions[0];
  const dirty = useMemo(
    () => JSON.stringify(proposal) !== JSON.stringify(savedProposal),
    [proposal, savedProposal],
  );

  useEffect(() => {
    const warn = (event: BeforeUnloadEvent): void => {
      if (dirty) {
        event.preventDefault();
        event.returnValue = '';
      }
    };
    window.addEventListener('beforeunload', warn);
    return () => window.removeEventListener('beforeunload', warn);
  }, [dirty]);

  const watchedWebhookExport = document?.exports?.find(
    (record) => record.id === watchedWebhookExportID,
  );
  const watchedWebhookTerminal =
    watchedWebhookExport?.status === 'succeeded' ||
    watchedWebhookExport?.status === 'failed' ||
    watchedWebhookExport?.status === 'dead_letter';

  /* Delivery is asynchronous, so the UI polls a bounded number of times rather
     than claiming success the moment the job is enqueued. Polling stops while
     the form is dirty so a refresh cannot discard unsaved edits. */
  useEffect(() => {
    if (
      !watchedWebhookExportID ||
      watchedWebhookTerminal ||
      dirty ||
      webhookStatusRefreshes >= maxWebhookStatusRefreshes
    ) {
      return;
    }
    const timer = window.setTimeout(() => {
      setWebhookStatusRefreshes((value) => value + 1);
      setReload((value) => value + 1);
    }, webhookStatusRefreshIntervalMs);
    return () => window.clearTimeout(timer);
  }, [dirty, watchedWebhookExportID, watchedWebhookTerminal, webhookStatusRefreshes]);

  const save = async (): Promise<void> => {
    if (!latest) return;
    setSaving(true);
    setError(undefined);
    try {
      await saveHumanReview(documentID, latest.version_number, proposal);
      setReload((value) => value + 1);
    } catch (requestError: unknown) {
      setError(
        requestError instanceof UploadRequestError
          ? requestError.message
          : 'InvoiceFlow could not save this correction. Try again.',
      );
    } finally {
      setSaving(false);
    }
  };

  const reject = async (): Promise<void> => {
    setRejecting(true);
    setError(undefined);
    try {
      await rejectReviewDocument(documentID);
      setShowReject(false);
      setReload((value) => value + 1);
    } catch (requestError: unknown) {
      setError(
        requestError instanceof UploadRequestError
          ? requestError.message
          : 'InvoiceFlow could not reject this document. Try again.',
      );
    } finally {
      setRejecting(false);
    }
  };

  const approve = async (): Promise<void> => {
    if (!latest) return;
    setApproving(true);
    setError(undefined);
    try {
      await approveReviewDocument(documentID, latest.version_number);
      setShowApprove(false);
      setReload((value) => value + 1);
    } catch (requestError: unknown) {
      setError(
        requestError instanceof UploadRequestError
          ? requestError.message
          : 'InvoiceFlow could not approve this version. Try again.',
      );
    } finally {
      setApproving(false);
    }
  };

  const handleWebhookExport = async (): Promise<void> => {
    setExportingWebhook(true);
    setError(undefined);
    setWebhookRefreshError(undefined);
    setWebhookMessage(undefined);
    try {
      const record = await triggerWebhookExport(documentID);
      setWatchedWebhookExportID(record.id);
      setWebhookStatusRefreshes(0);
      setWebhookMessage('Webhook export queued. Waiting for the worker to report delivery status.');
      setReload((value) => value + 1);
    } catch (requestError: unknown) {
      setError(
        requestError instanceof UploadRequestError
          ? requestError.message
          : 'InvoiceFlow could not trigger webhook export. Try again.',
      );
    } finally {
      setExportingWebhook(false);
    }
  };

  const refreshWebhookStatus = (): void => {
    setWebhookRefreshError(undefined);
    setWebhookStatusRefreshes(0);
    setReload((value) => value + 1);
  };

  const handleCSVExport = async (): Promise<void> => {
    setExportingCSV(true);
    setError(undefined);
    setCSVMessage(undefined);
    try {
      await downloadCSV(documentID);
      setCSVMessage(
        `CSV v1 downloaded for approved version ${document?.approved_version_number ?? latest?.version_number}.`,
      );
      setShowCSVConfirm(false);
      setReload((value) => value + 1);
    } catch (requestError: unknown) {
      setError(
        requestError instanceof UploadRequestError
          ? requestError.message
          : 'InvoiceFlow could not create the CSV export. Try again.',
      );
    } finally {
      setExportingCSV(false);
    }
  };

  if (error && !document) {
    return (
      <ReviewMessage
        tone="danger"
        title="Review unavailable"
        message={error}
        onRetry={() => setReload((value) => value + 1)}
      />
    );
  }
  if (!document) {
    return (
      <ReviewMessage
        tone="info"
        title="Loading review"
        message="Loading the immutable extraction proposal and source document."
      />
    );
  }
  if (document.status === 'failed') {
    return (
      <ReviewMessage
        tone="danger"
        title="Processing failed"
        message="This document has no review proposal."
        onRetry={() => setReload((value) => value + 1)}
      />
    );
  }
  if (!latest) {
    return (
      <ReviewMessage
        tone="warning"
        title="No review proposal"
        message="This document is still processing or did not produce an extraction snapshot."
        onRetry={() => setReload((value) => value + 1)}
      />
    );
  }

  const editable = document.status === 'needs_review';
  const approvedOrExported = document.status === 'approved' || document.status === 'exported';
  const approvedVersion = document.approved_version_number ?? latest.version_number;

  const statusTone: StatusTone =
    document.status === 'rejected' ? 'danger' : approvedOrExported ? 'success' : 'warning';
  const statusLabel =
    document.status === 'rejected'
      ? 'Rejected'
      : document.status === 'approved'
        ? 'Approved'
        : document.status === 'exported'
          ? 'Exported'
          : 'Needs review';
  const webhookLifecycleMessage =
    webhookStatusMessage(
      watchedWebhookExport,
      webhookStatusRefreshes,
      watchedWebhookExportID !== undefined,
    ) ?? webhookMessage;

  return (
    <main id="main-content" className="app-main" tabIndex={-1}>
      <header className="page-heading">
        <div>
          <p className="eyebrow">Human review &amp; export</p>
          <h1>Compare the source and proposal.</h1>
          <p>
            Corrections create immutable review versions. Explicit approval enables CSV &amp;
            Webhook export.
          </p>
        </div>
        <StatusTag tone={statusTone}>{statusLabel}</StatusTag>
      </header>

      {error ? (
        <p className="review-error" role="alert">
          {error}
        </p>
      ) : null}
      {dirty ? (
        <p className="review-unsaved" role="status">
          Unsaved changes. Save a correction before leaving or approving this review.
        </p>
      ) : null}
      {webhookLifecycleMessage ? (
        <p className="review-success" role="status">
          {webhookLifecycleMessage}
        </p>
      ) : null}
      {csvMessage ? (
        <p className="review-success" role="status">
          {csvMessage}
        </p>
      ) : null}
      {webhookRefreshError ? (
        <p className="review-error" role="alert">
          Webhook status could not be refreshed: {webhookRefreshError}
        </p>
      ) : null}

      <section className="review-workspace" aria-label="Invoice review workspace">
        <SourcePanel documentID={documentID} mediaType={document.media_type} />
        <section className="review-panel" aria-label="Extracted invoice proposal">
          <div className="review-version-bar">
            <div>
              <p className="eyebrow">Version {latest.version_number}</p>
              <h2>
                {latest.source === 'human_review'
                  ? 'Human-reviewed proposal'
                  : 'Extracted proposal'}
              </h2>
            </div>
            <span className={`version-source version-source-${latest.source}`}>
              {latest.source === 'human_review' ? 'Human edited' : 'AI extracted'}
            </span>
          </div>

          <ReviewForm
            value={proposal}
            disabled={!editable || saving}
            onChange={setProposal}
            warnings={latest.warnings}
          />
          <ReviewContext version={latest} audit={document.audit} exports={document.exports} />

          {editable ? (
            <div className="review-actions">
              <button
                className="button button-primary"
                type="button"
                disabled={saving || !dirty}
                onClick={() => void save()}
              >
                {saving ? 'Saving correction…' : 'Save correction'}
              </button>
              <button
                className="button button-primary"
                type="button"
                disabled={saving || dirty}
                onClick={() => setShowApprove(true)}
              >
                Approve version {latest.version_number}
              </button>
              <button
                className="button button-danger"
                type="button"
                disabled={saving}
                onClick={() => setShowReject(true)}
              >
                Reject document
              </button>
            </div>
          ) : approvedOrExported ? (
            <div className="review-export-actions">
              <p className="eyebrow">Approved for Export (Version {approvedVersion})</p>
              <div className="review-actions">
                <button
                  className="button button-primary"
                  type="button"
                  disabled={exportingCSV}
                  onClick={() => setShowCSVConfirm(true)}
                >
                  Download CSV Export
                </button>
                <button
                  className="button button-primary"
                  type="button"
                  disabled={exportingWebhook}
                  onClick={() => setShowWebhookConfirm(true)}
                >
                  {exportingWebhook ? 'Enqueuing Webhook…' : 'Send Webhook Export'}
                </button>
                {watchedWebhookExportID ? (
                  <button
                    className="button button-quiet"
                    type="button"
                    disabled={exportingWebhook}
                    onClick={refreshWebhookStatus}
                  >
                    Refresh webhook status
                  </button>
                ) : null}
              </div>
            </div>
          ) : (
            <p className="review-readonly" role="status">
              This document is rejected. Its extraction and review versions remain read-only.
            </p>
          )}
        </section>
      </section>

      {showApprove ? (
        <ConfirmDialog
          title={`Approve Version ${latest.version_number}?`}
          onClose={() => setShowApprove(false)}
          confirmLabel={approving ? 'Approving…' : 'Confirm approval'}
          onConfirm={() => void approve()}
          disabled={approving}
        >
          <p>
            Approving version {latest.version_number} locks this invoice and creates an immutable
            approved record for CSV and Webhook export. This action cannot be undone.
          </p>
        </ConfirmDialog>
      ) : null}

      {showCSVConfirm ? (
        <ConfirmDialog
          title="Download CSV Export?"
          onClose={() => setShowCSVConfirm(false)}
          confirmLabel={exportingCSV ? 'Creating CSV…' : 'Confirm CSV export'}
          onConfirm={() => void handleCSVExport()}
          disabled={exportingCSV}
        >
          <p>
            Creates the versioned InvoiceFlow CSV v1 from approved version {approvedVersion}. The
            download is deterministic and uses exact server-normalized money.
          </p>
        </ConfirmDialog>
      ) : null}

      {showWebhookConfirm ? (
        <ConfirmDialog
          title="Send Webhook Export?"
          onClose={() => setShowWebhookConfirm(false)}
          confirmLabel={exportingWebhook ? 'Enqueuing…' : 'Confirm webhook export'}
          onConfirm={() => {
            setShowWebhookConfirm(false);
            void handleWebhookExport();
          }}
          disabled={exportingWebhook}
        >
          <p>Enqueues a durable HMAC-SHA256 webhook job for approved version {approvedVersion}.</p>
          <p>Destination: Server-configured webhook. The full URL and secret are never shown.</p>
        </ConfirmDialog>
      ) : null}

      {showReject ? (
        <ConfirmDialog
          title="Reject this document?"
          onClose={() => setShowReject(false)}
          confirmLabel={rejecting ? 'Rejecting…' : 'Confirm rejection'}
          onConfirm={() => void reject()}
          disabled={rejecting}
          danger
        >
          <p>This is a terminal transition. It does not delete the source or review history.</p>
        </ConfirmDialog>
      ) : null}
    </main>
  );
}

function webhookStatusMessage(
  record: ExportRecord | undefined,
  refreshes: number,
  watching: boolean,
): string | undefined {
  if (!watching) return undefined;
  if (!record) return 'Webhook export queued. Waiting for the worker to report delivery status.';
  const retryLimitMessage =
    refreshes >= maxWebhookStatusRefreshes
      ? ' Automatic refresh paused; use Refresh webhook status to check again.'
      : '';
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
