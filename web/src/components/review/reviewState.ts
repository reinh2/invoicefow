import type { EditableProposal, ReviewDocument } from '../../api/documents';

/* The review workspace has one state machine, not sixteen independent flags.
   Two facts drive most of the interface and were previously spread across eight
   booleans: which irreversible action the user is confirming, and which one is
   currently in flight. Both are single-valued by construction — the confirmation
   dialog is modal and every action starts from it — so representing them as one
   value each removes states that could never be reached but still had to be
   reasoned about. */

/** The irreversible actions that require confirmation before they run. */
export type ConfirmTarget = 'approve' | 'reject' | 'csv' | 'webhook';

/** The actions that can be in flight. 'save' needs no confirmation. */
export type PendingAction = ConfirmTarget | 'save';

export interface ReviewState {
  document?: ReviewDocument;
  /** The edited proposal, and the last version the server accepted. */
  proposal: EditableProposal;
  savedProposal: EditableProposal;
  /** A failure that concerns the review itself. */
  error?: string;
  /** Bumped to re-run the document request. */
  reload: number;
  pending?: PendingAction;
  confirming?: ConfirmTarget;
  csvMessage?: string;
  webhookMessage?: string;
  /** A failed background refresh, which must not blank out the review. */
  webhookRefreshError?: string;
  watchedWebhookExportID?: string;
  webhookStatusRefreshes: number;
}

export type ReviewAction =
  | { type: 'loaded'; document: ReviewDocument }
  /** background distinguishes a failed poll from a failed first load. */
  | { type: 'load_failed'; message: string; background: boolean }
  | { type: 'edit'; proposal: EditableProposal }
  | { type: 'reload' }
  | { type: 'confirm'; target: ConfirmTarget }
  | { type: 'dismiss' }
  | { type: 'start'; action: PendingAction }
  | { type: 'action_failed'; message: string }
  | { type: 'saved' }
  | { type: 'completed' }
  | { type: 'csv_exported'; message: string }
  | { type: 'webhook_enqueued'; exportID: string; message: string }
  | { type: 'poll' }
  | { type: 'refresh_webhook' };

export const blankProposal = (): EditableProposal => ({
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

export const cloneProposal = (proposal: EditableProposal): EditableProposal => ({
  ...proposal,
  line_items: proposal.line_items.map((line) => ({ ...line })),
});

export const initialReviewState: ReviewState = {
  proposal: blankProposal(),
  savedProposal: blankProposal(),
  reload: 0,
  webhookStatusRefreshes: 0,
};

export function reviewReducer(state: ReviewState, action: ReviewAction): ReviewState {
  switch (action.type) {
    case 'loaded': {
      /* The server's newest version replaces both the edited and the saved
         proposal, so a reload after a successful save starts clean. */
      const editable = action.document.versions[0]?.editable ?? blankProposal();
      return {
        ...state,
        document: action.document,
        error: undefined,
        webhookRefreshError: undefined,
        proposal: cloneProposal(editable),
        savedProposal: cloneProposal(editable),
      };
    }
    case 'load_failed':
      /* A failed background refresh must not blank out a review the user is
         already working in, so it reports separately from a failed first load. */
      return action.background
        ? { ...state, webhookRefreshError: action.message }
        : { ...state, error: action.message };
    case 'edit':
      return { ...state, proposal: action.proposal };
    case 'reload':
      return { ...state, reload: state.reload + 1 };
    case 'confirm':
      return { ...state, confirming: action.target };
    case 'dismiss':
      return { ...state, confirming: undefined };
    case 'start':
      /* Starting an action clears the previous failure, and starting a webhook
         export also clears the previous delivery messages. */
      return action.action === 'webhook'
        ? {
            ...state,
            pending: 'webhook',
            error: undefined,
            webhookRefreshError: undefined,
            webhookMessage: undefined,
          }
        : { ...state, pending: action.action, error: undefined };
    case 'action_failed':
      /* The confirmation dialog stays open so the failure is shown where the
         user triggered it. */
      return { ...state, pending: undefined, error: action.message };
    case 'saved':
      return { ...state, pending: undefined, reload: state.reload + 1 };
    case 'completed':
      return {
        ...state,
        pending: undefined,
        confirming: undefined,
        reload: state.reload + 1,
      };
    case 'csv_exported':
      return {
        ...state,
        pending: undefined,
        confirming: undefined,
        csvMessage: action.message,
        reload: state.reload + 1,
      };
    case 'webhook_enqueued':
      return {
        ...state,
        pending: undefined,
        confirming: undefined,
        watchedWebhookExportID: action.exportID,
        webhookStatusRefreshes: 0,
        webhookMessage: action.message,
        reload: state.reload + 1,
      };
    case 'poll':
      return {
        ...state,
        webhookStatusRefreshes: state.webhookStatusRefreshes + 1,
        reload: state.reload + 1,
      };
    case 'refresh_webhook':
      /* A manual refresh restarts the bounded automatic polling. */
      return {
        ...state,
        webhookRefreshError: undefined,
        webhookStatusRefreshes: 0,
        reload: state.reload + 1,
      };
  }
}
