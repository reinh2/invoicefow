import { describe, expect, it } from 'vitest';
import type { ReviewDocument } from '../../api/documents';
import { blankProposal, initialReviewState, reviewReducer, type ReviewState } from './reviewState';

function documentWith(editable: Partial<ReturnType<typeof blankProposal>>): ReviewDocument {
  return {
    id: 'doc-1',
    status: 'needs_review',
    created_at: '2026-07-25T10:00:00Z',
    updated_at: '2026-07-25T10:00:00Z',
    media_type: 'application/pdf',
    versions: [
      {
        version_number: 1,
        source: 'extraction',
        created_at: '2026-07-25T10:00:00Z',
        rounding_policy_version: 'money-v1',
        warnings: [],
        editable: { ...blankProposal(), ...editable },
      },
    ],
    audit: [],
  } as unknown as ReviewDocument;
}

describe('reviewReducer', () => {
  it('replaces both the edited and saved proposal on load, so a reload starts clean', () => {
    const dirty: ReviewState = {
      ...initialReviewState,
      proposal: { ...blankProposal(), total: '99.00' },
      error: 'stale failure',
      webhookRefreshError: 'stale refresh failure',
    };
    const next = reviewReducer(dirty, {
      type: 'loaded',
      document: documentWith({ total: '24.00' }),
    });

    expect(next.proposal.total).toBe('24.00');
    expect(next.savedProposal.total).toBe('24.00');
    expect(next.error).toBeUndefined();
    expect(next.webhookRefreshError).toBeUndefined();
  });

  it('keeps the edited and saved proposals independent so editing does not clear the dirty flag', () => {
    const loaded = reviewReducer(initialReviewState, {
      type: 'loaded',
      document: documentWith({ total: '24.00', line_items: [] }),
    });
    const edited = reviewReducer(loaded, {
      type: 'edit',
      proposal: { ...loaded.proposal, total: '25.00' },
    });

    expect(edited.proposal.total).toBe('25.00');
    expect(edited.savedProposal.total).toBe('24.00');
  });

  it('reports a failed background refresh separately from a failed first load', () => {
    const background = reviewReducer(initialReviewState, {
      type: 'load_failed',
      message: 'gone',
      background: true,
    });
    expect(background.webhookRefreshError).toBe('gone');
    expect(background.error).toBeUndefined();

    const first = reviewReducer(initialReviewState, {
      type: 'load_failed',
      message: 'gone',
      background: false,
    });
    expect(first.error).toBe('gone');
    expect(first.webhookRefreshError).toBeUndefined();
  });

  it('leaves the confirmation dialog open when an action fails', () => {
    const confirming = reviewReducer(initialReviewState, { type: 'confirm', target: 'approve' });
    const started = reviewReducer(confirming, { type: 'start', action: 'approve' });
    expect(started.pending).toBe('approve');

    const failed = reviewReducer(started, { type: 'action_failed', message: 'conflict' });
    expect(failed.pending).toBeUndefined();
    expect(failed.confirming).toBe('approve');
    expect(failed.error).toBe('conflict');
  });

  it('closes the dialog and reloads only when an action completes', () => {
    const started = reviewReducer(
      reviewReducer(initialReviewState, { type: 'confirm', target: 'reject' }),
      { type: 'start', action: 'reject' },
    );
    const completed = reviewReducer(started, { type: 'completed' });

    expect(completed.pending).toBeUndefined();
    expect(completed.confirming).toBeUndefined();
    expect(completed.reload).toBe(initialReviewState.reload + 1);
  });

  it('starting a webhook export clears the previous delivery messages', () => {
    const stale: ReviewState = {
      ...initialReviewState,
      webhookMessage: 'previous attempt queued',
      webhookRefreshError: 'previous refresh failed',
      error: 'previous failure',
    };
    const started = reviewReducer(stale, { type: 'start', action: 'webhook' });

    expect(started.webhookMessage).toBeUndefined();
    expect(started.webhookRefreshError).toBeUndefined();
    expect(started.error).toBeUndefined();
  });

  it('watches the enqueued export and restarts the bounded poll counter', () => {
    const polled = reviewReducer(
      { ...initialReviewState, webhookStatusRefreshes: 7 },
      { type: 'poll' },
    );
    expect(polled.webhookStatusRefreshes).toBe(8);

    const enqueued = reviewReducer(polled, {
      type: 'webhook_enqueued',
      exportID: 'exp-1',
      message: 'queued',
    });
    expect(enqueued.watchedWebhookExportID).toBe('exp-1');
    expect(enqueued.webhookStatusRefreshes).toBe(0);

    const refreshed = reviewReducer(
      { ...enqueued, webhookStatusRefreshes: 10, webhookRefreshError: 'timed out' },
      { type: 'refresh_webhook' },
    );
    expect(refreshed.webhookStatusRefreshes).toBe(0);
    expect(refreshed.webhookRefreshError).toBeUndefined();
    // The watched export is kept: a manual refresh checks the same delivery.
    expect(refreshed.watchedWebhookExportID).toBe('exp-1');
  });

  it('never leaves two actions pending or two dialogs open at once', () => {
    const first = reviewReducer(initialReviewState, { type: 'start', action: 'csv' });
    const second = reviewReducer(first, { type: 'start', action: 'webhook' });
    expect(second.pending).toBe('webhook');

    const dialog = reviewReducer(
      reviewReducer(initialReviewState, { type: 'confirm', target: 'csv' }),
      { type: 'confirm', target: 'reject' },
    );
    expect(dialog.confirming).toBe('reject');
    expect(reviewReducer(dialog, { type: 'dismiss' }).confirming).toBeUndefined();
  });
});
