import { useCallback, useEffect, useMemo, useReducer, type ReactElement } from 'react';
import {
  approveReviewDocument,
  downloadCSV,
  getReviewDocument,
  rejectReviewDocument,
  saveHumanReview,
  triggerWebhookExport,
  UploadRequestError,
  type ExportRecord,
} from '../api/documents';
import { StatusTag, type StatusTone } from './StatusTag';
import { ConfirmDialog } from './review/ConfirmDialog';
import { ReviewContext } from './review/ReviewContext';
import { ReviewForm } from './review/ReviewForm';
import { ReviewMessage } from './review/ReviewMessage';
import { SourcePanel } from './review/SourcePanel';
import { initialReviewState, reviewReducer, type PendingAction } from './review/reviewState';

/* This component drives the review state machine and the transitions a human
   can trigger. The state shape and every transition live in ./review/reviewState;
   presentation lives in ./review/*: the source panel, the editable form and its
   per-field warnings, the derived server context, and the confirmation dialog
   every irreversible action goes through. */

const webhookStatusRefreshIntervalMs = 1500;
const maxWebhookStatusRefreshes = 10;

export function ReviewWorkspace({ documentID }: { documentID: string }): ReactElement {
  const [state, dispatch] = useReducer(reviewReducer, initialReviewState);
  const {
    document,
    proposal,
    savedProposal,
    error,
    reload,
    pending,
    confirming,
    csvMessage,
    webhookMessage,
    webhookRefreshError,
    watchedWebhookExportID,
    webhookStatusRefreshes,
  } = state;

  useEffect(() => {
    const controller = new AbortController();
    const hasExistingDocument = document !== undefined;
    void getReviewDocument(documentID, controller.signal)
      .then((next) => dispatch({ type: 'loaded', document: next }))
      .catch((requestError: unknown) => {
        if (controller.signal.aborted) return;
        const message =
          requestError instanceof UploadRequestError
            ? requestError.message
            : 'InvoiceFlow could not load this review. Try again.';
        dispatch({ type: 'load_failed', message, background: hasExistingDocument });
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
    const timer = window.setTimeout(
      () => dispatch({ type: 'poll' }),
      webhookStatusRefreshIntervalMs,
    );
    return () => window.clearTimeout(timer);
  }, [dirty, watchedWebhookExportID, watchedWebhookTerminal, webhookStatusRefreshes]);

  /* Every action shares one shape: mark itself pending, run one request, and
     report either a failure or its own success transition. */
  const run = useCallback(
    async (
      action: PendingAction,
      request: () => Promise<void>,
      fallbackMessage: string,
    ): Promise<void> => {
      dispatch({ type: 'start', action });
      try {
        await request();
      } catch (requestError: unknown) {
        dispatch({
          type: 'action_failed',
          message:
            requestError instanceof UploadRequestError ? requestError.message : fallbackMessage,
        });
      }
    },
    [],
  );

  const save = (): Promise<void> => {
    if (!latest) return Promise.resolve();
    return run(
      'save',
      async () => {
        await saveHumanReview(documentID, latest.version_number, proposal);
        dispatch({ type: 'saved' });
      },
      'InvoiceFlow could not save this correction. Try again.',
    );
  };

  const reject = (): Promise<void> =>
    run(
      'reject',
      async () => {
        await rejectReviewDocument(documentID);
        dispatch({ type: 'completed' });
      },
      'InvoiceFlow could not reject this document. Try again.',
    );

  const approve = (): Promise<void> => {
    if (!latest) return Promise.resolve();
    return run(
      'approve',
      async () => {
        await approveReviewDocument(documentID, latest.version_number);
        dispatch({ type: 'completed' });
      },
      'InvoiceFlow could not approve this version. Try again.',
    );
  };

  const handleWebhookExport = (): Promise<void> =>
    run(
      'webhook',
      async () => {
        const record = await triggerWebhookExport(documentID);
        dispatch({
          type: 'webhook_enqueued',
          exportID: record.id,
          message: 'Webhook export queued. Waiting for the worker to report delivery status.',
        });
      },
      'InvoiceFlow could not trigger webhook export. Try again.',
    );

  const handleCSVExport = (): Promise<void> =>
    run(
      'csv',
      async () => {
        await downloadCSV(documentID);
        dispatch({
          type: 'csv_exported',
          message: `CSV v1 downloaded for approved version ${document?.approved_version_number ?? latest?.version_number}.`,
        });
      },
      'InvoiceFlow could not create the CSV export. Try again.',
    );

  if (error && !document) {
    return (
      <ReviewMessage
        tone="danger"
        title="Review unavailable"
        message={error}
        onRetry={() => dispatch({ type: 'reload' })}
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
        onRetry={() => dispatch({ type: 'reload' })}
      />
    );
  }
  if (!latest) {
    return (
      <ReviewMessage
        tone="warning"
        title="No review proposal"
        message="This document is still processing or did not produce an extraction snapshot."
        onRetry={() => dispatch({ type: 'reload' })}
      />
    );
  }

  const editable = document.status === 'needs_review';
  const approvedOrExported = document.status === 'approved' || document.status === 'exported';
  const approvedVersion = document.approved_version_number ?? latest.version_number;
  const busy = pending !== undefined;

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
            disabled={!editable || busy}
            onChange={(next) => dispatch({ type: 'edit', proposal: next })}
            warnings={latest.warnings}
          />
          <ReviewContext version={latest} audit={document.audit} exports={document.exports} />

          {editable ? (
            <div className="review-actions">
              <button
                className="button button-primary"
                type="button"
                disabled={busy || !dirty}
                onClick={() => void save()}
              >
                {pending === 'save' ? 'Saving correction…' : 'Save correction'}
              </button>
              <button
                className="button button-primary"
                type="button"
                disabled={busy || dirty}
                onClick={() => dispatch({ type: 'confirm', target: 'approve' })}
              >
                Approve version {latest.version_number}
              </button>
              <button
                className="button button-danger"
                type="button"
                disabled={busy}
                onClick={() => dispatch({ type: 'confirm', target: 'reject' })}
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
                  disabled={busy}
                  onClick={() => dispatch({ type: 'confirm', target: 'csv' })}
                >
                  Download CSV Export
                </button>
                <button
                  className="button button-primary"
                  type="button"
                  disabled={busy}
                  onClick={() => dispatch({ type: 'confirm', target: 'webhook' })}
                >
                  {pending === 'webhook' ? 'Enqueuing Webhook…' : 'Send Webhook Export'}
                </button>
                {watchedWebhookExportID ? (
                  <button
                    className="button button-quiet"
                    type="button"
                    disabled={busy}
                    onClick={() => dispatch({ type: 'refresh_webhook' })}
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

      {confirming === 'approve' ? (
        <ConfirmDialog
          title={`Approve Version ${latest.version_number}?`}
          onClose={() => dispatch({ type: 'dismiss' })}
          confirmLabel={pending === 'approve' ? 'Approving…' : 'Confirm approval'}
          onConfirm={() => void approve()}
          disabled={pending === 'approve'}
        >
          <p>
            Approving version {latest.version_number} locks this invoice and creates an immutable
            approved record for CSV and Webhook export. This action cannot be undone.
          </p>
        </ConfirmDialog>
      ) : null}

      {confirming === 'csv' ? (
        <ConfirmDialog
          title="Download CSV Export?"
          onClose={() => dispatch({ type: 'dismiss' })}
          confirmLabel={pending === 'csv' ? 'Creating CSV…' : 'Confirm CSV export'}
          onConfirm={() => void handleCSVExport()}
          disabled={pending === 'csv'}
        >
          <p>
            Creates the versioned InvoiceFlow CSV v1 from approved version {approvedVersion}. The
            download is deterministic and uses exact server-normalized money.
          </p>
        </ConfirmDialog>
      ) : null}

      {confirming === 'webhook' ? (
        <ConfirmDialog
          title="Send Webhook Export?"
          onClose={() => dispatch({ type: 'dismiss' })}
          confirmLabel={pending === 'webhook' ? 'Enqueuing…' : 'Confirm webhook export'}
          onConfirm={() => {
            dispatch({ type: 'dismiss' });
            void handleWebhookExport();
          }}
          disabled={pending === 'webhook'}
        >
          <p>Enqueues a durable HMAC-SHA256 webhook job for approved version {approvedVersion}.</p>
          <p>Destination: Server-configured webhook. The full URL and secret are never shown.</p>
        </ConfirmDialog>
      ) : null}

      {confirming === 'reject' ? (
        <ConfirmDialog
          title="Reject this document?"
          onClose={() => dispatch({ type: 'dismiss' })}
          confirmLabel={pending === 'reject' ? 'Rejecting…' : 'Confirm rejection'}
          onConfirm={() => void reject()}
          disabled={pending === 'reject'}
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
