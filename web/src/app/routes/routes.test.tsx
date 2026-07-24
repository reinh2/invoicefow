import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { AppShell } from './AppShell';
import { LandingPage } from './LandingPage';

const originalFetch = globalThis.fetch;

afterEach(() => {
	globalThis.fetch = originalFetch;
	vi.useRealTimers();
	vi.restoreAllMocks();
});

function response(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

describe('routes', () => {
  it('renders an honest landing page with a skip link', () => {
    render(<LandingPage />);
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('a person still approves');
    expect(screen.getByRole('link', { name: 'Skip to main content' })).toHaveAttribute('href', '#main-content');
    expect(screen.queryByText(/%|documents processed/i)).not.toBeInTheDocument();
  });

  it('moves keyboard focus to the main landmark when the skip link is activated', () => {
    render(<LandingPage />);
    fireEvent.click(screen.getByRole('link', { name: 'Skip to main content' }));
    expect(screen.getByRole('main')).toHaveFocus();
  });

  it('provides a keyboard-accessible native file input and an idle state', () => {
    render(<AppShell />);
    const input = screen.getByLabelText('Invoice file');
    expect(input).toHaveAttribute('type', 'file');
    expect(input).toHaveAttribute('accept', expect.stringContaining('.pdf'));
    expect(screen.getByText('No document selected.')).toBeVisible();
  });

  it('submits multipart data and shows queued only from the server response', async () => {
    const fetchMock = vi.fn().mockResolvedValue(response(201, { document: { id: '0d0c2342-2486-4f10-a858-e75bc763f3e4', status: 'queued' } }));
    globalThis.fetch = fetchMock;
    render(<AppShell />);
    const file = new File(['fictional'], 'invoice.pdf', { type: 'application/pdf' });
    fireEvent.change(screen.getByLabelText('Invoice file'), { target: { files: [file] } });
    expect(await screen.findByText(/sending invoice\.pdf/i)).toBeVisible();
    await waitFor(() => expect(screen.getByText(/accepted and queued for processing/i)).toBeVisible());
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/documents', expect.objectContaining({ method: 'POST' }));
    const request = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(request.body).toBeInstanceOf(FormData);
    expect((request.body as FormData).get('file')).toBe(file);
  });

  it('shows a duplicate state without inventing existing-document details', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue(response(409, { error: { code: 'duplicate_document', message: 'This document has already been accepted.', request_id: 'request-123' } }));
    render(<AppShell />);
    fireEvent.change(screen.getByLabelText('Invoice file'), { target: { files: [new File(['fictional'], 'duplicate.pdf', { type: 'application/pdf' })] } });
    expect(await screen.findByText('Duplicate')).toBeVisible();
    expect(screen.getByText('This document has already been accepted.')).toBeVisible();
    expect(screen.queryByText('request-123')).not.toBeInTheDocument();
  });

  it('shows a safe error response and restores the file control', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue(response(415, { error: { code: 'unsupported_media_type', message: 'This file type is not supported.' } }));
    render(<AppShell />);
    const input = screen.getByLabelText('Invoice file');
    fireEvent.change(input, { target: { files: [new File(['fictional'], 'invoice.txt', { type: 'text/plain' })] } });
    expect(await screen.findByRole('alert')).toHaveTextContent('This file type is not supported.');
    expect(input).not.toBeDisabled();
  });

  it('renders the split review, saves an immutable correction, and confirms rejection', async () => {
    const documentID = '0d0c2342-2486-4f10-a858-e75bc763f3e4';
    const review = {
      document: {
        id: documentID, status: 'needs_review', created_at: '2026-07-23T12:00:00Z', updated_at: '2026-07-23T12:00:00Z', media_type: 'application/pdf', audit: [{ sequence: 3, action: 'processing_completed', actor: 'system', payload: {}, occurred_at: '2026-07-23T12:00:00Z' }],
        versions: [{ version_number: 1, source: 'extraction', created_at: '2026-07-23T12:00:00Z', proposal: {}, normalized: {}, warnings: [{ code: 'subtotal_tax_total_mismatch', field: 'total', message: 'Subtotal plus tax does not equal total.' }], evidence: [{ field: 'total', page_number: 1, excerpt: '24.00' }], diagnostics: [{ code: 'fake_fixture', message: 'Fixture proposal returned.' }], rounding_policy_version: 'money-v1', editable: { supplier_name: 'Fictional Vendor', supplier_email: '', invoice_number: 'INV-1', issue_date: '2026-07-20', due_date: '', currency: 'USD', subtotal: '20.00', tax_amount: '4.00', total: '24.00', line_items: [] } }],
      },
    };
    let detailRequests = 0;
    globalThis.fetch = vi.fn().mockImplementation((path: string) => {
      if (path.endsWith('/human-reviews')) return Promise.resolve(response(201, { version_number: 2 }));
      if (path.endsWith('/reject')) return Promise.resolve(new Response(null, { status: 204 }));
      detailRequests += 1;
      return Promise.resolve(response(200, review));
    });
    render(<AppShell documentID={documentID} />);
    expect(await screen.findByRole('heading', { name: 'Extracted proposal' })).toBeVisible();
    expect(screen.getByLabelText('Original invoice PDF')).toBeVisible();
    const total = screen.getByLabelText('Total');
    fireEvent.change(total, { target: { value: '25.00' } });
    expect(screen.getByText(/unsaved changes/i)).toBeVisible();
    fireEvent.click(screen.getByRole('button', { name: 'Save correction' }));
    await waitFor(() => expect(detailRequests).toBeGreaterThan(1));
    expect(globalThis.fetch).toHaveBeenCalledWith(`/api/v1/documents/${documentID}/human-reviews`, expect.objectContaining({ method: 'POST' }));
    fireEvent.click(screen.getByRole('button', { name: 'Reject document' }));
    expect(screen.getByRole('dialog', { name: 'Reject this document?' })).toBeVisible();
    fireEvent.click(screen.getByRole('button', { name: 'Confirm rejection' }));
    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalledWith(`/api/v1/documents/${documentID}/reject`, expect.objectContaining({ method: 'POST' })));
  });

  it('shows a safe failed processing and retry state', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue(response(200, { document: { id: '0d0c2342-2486-4f10-a858-e75bc763f3e4', status: 'failed', created_at: '2026-07-23T12:00:00Z', updated_at: '2026-07-23T12:00:00Z', media_type: 'application/pdf', versions: [], audit: [] } }));
    render(<AppShell documentID="0d0c2342-2486-4f10-a858-e75bc763f3e4" />);
    expect(await screen.findByRole('heading', { name: 'Processing failed' })).toBeVisible();
    expect(screen.getByRole('button', { name: 'Reload review' })).toBeVisible();
  });

	it('supports version approval and exposes export options', async () => {
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => undefined);
    const documentID = '0d0c2342-2486-4f10-a858-e75bc763f3e4';
    const reviewNeedsReview = {
      document: {
        id: documentID, status: 'needs_review', created_at: '2026-07-23T12:00:00Z', updated_at: '2026-07-23T12:00:00Z', media_type: 'application/pdf', audit: [],
        versions: [{ version_number: 1, source: 'extraction', created_at: '2026-07-23T12:00:00Z', proposal: {}, normalized: {}, warnings: [], evidence: [], diagnostics: [], rounding_policy_version: 'money-v1', editable: { supplier_name: 'Fictional Vendor', supplier_email: '', invoice_number: 'INV-1', issue_date: '2026-07-20', due_date: '', currency: 'USD', subtotal: '20.00', tax_amount: '4.00', total: '24.00', line_items: [] } }],
      },
    };
    const reviewApproved = {
      document: {
        ...reviewNeedsReview.document, status: 'approved', approved_version_number: 1, approved_at: '2026-07-23T12:05:00Z',
      },
    };
    let currentResponse = reviewNeedsReview;
    globalThis.fetch = vi.fn().mockImplementation((path: string) => {
      if (path.endsWith('/approve')) {
        currentResponse = reviewApproved;
        return Promise.resolve(response(200, { document: { id: documentID, status: 'approved', approved_version_number: 1 } }));
      }
      if (path.endsWith('/export/csv')) {
        return Promise.resolve(new Response('supplier_name\r\nVendor\r\n', { status: 200, headers: { 'Content-Type': 'text/csv; charset=utf-8', 'Content-Disposition': 'attachment; filename="invoice-doc-v1.csv"' } }));
      }
		if (path.endsWith('/export/webhook')) {
			return Promise.resolve(response(202, { export: { id: 'exp-12345678', document_id: documentID, version_number: 1, export_type: 'webhook', status: 'pending', idempotency_key: 'webhook_export:doc:v1', destination_ref: 'server:webhook:v1', destination_label: 'Server-configured webhook', attempts: 0, created_at: '2026-07-23T12:05:00Z', updated_at: '2026-07-23T12:05:00Z' } }));
		}
      return Promise.resolve(response(200, currentResponse));
    });

    render(<AppShell documentID={documentID} />);
    expect(await screen.findByRole('button', { name: 'Approve version 1' })).toBeVisible();
    fireEvent.click(screen.getByRole('button', { name: 'Approve version 1' }));
    expect(screen.getByRole('dialog', { name: 'Approve Version 1?' })).toBeVisible();
    fireEvent.click(screen.getByRole('button', { name: 'Confirm approval' }));

    await waitFor(() => expect(screen.getByRole('button', { name: 'Download CSV Export' })).toBeVisible());
    const csvButton = screen.getByRole('button', { name: 'Download CSV Export' });
    csvButton.focus();
    fireEvent.click(csvButton);
    expect(screen.getByRole('dialog', { name: 'Download CSV Export?' })).toBeVisible();
    expect(screen.getByRole('button', { name: 'Confirm CSV export' })).toHaveFocus();
    fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape' });
    expect(screen.queryByRole('dialog', { name: 'Download CSV Export?' })).not.toBeInTheDocument();
    expect(csvButton).toHaveFocus();
    fireEvent.click(screen.getByRole('button', { name: 'Download CSV Export' }));
    fireEvent.click(screen.getByRole('button', { name: 'Confirm CSV export' }));
    await waitFor(() => expect(screen.getByText(/CSV v1 downloaded/i)).toBeVisible());

    fireEvent.click(screen.getByRole('button', { name: 'Send Webhook Export' }));
    expect(screen.getByRole('dialog', { name: 'Send Webhook Export?' })).toBeVisible();
    fireEvent.click(screen.getByRole('button', { name: 'Confirm webhook export' }));
		await waitFor(() => expect(screen.getByText(/Webhook export queued/i)).toBeVisible());
	});

	it('polls webhook delivery from pending through retrying to succeeded without a false success', async () => {
		const documentID = '0d0c2342-2486-4f10-a858-e75bc763f3e4';
		const version = { version_number: 1, source: 'extraction', created_at: '2026-07-23T12:00:00Z', proposal: {}, normalized: {}, warnings: [], evidence: [], diagnostics: [], rounding_policy_version: 'money-v1', editable: { supplier_name: 'Fictional Vendor', supplier_email: '', invoice_number: 'INV-1', issue_date: '2026-07-20', due_date: '', currency: 'USD', subtotal: '20.00', tax_amount: '4.00', total: '24.00', line_items: [] } };
		const record = (status: 'pending' | 'retrying' | 'succeeded', attempts: number, error_summary?: string) => ({ id: 'exp-12345678', document_id: documentID, version_number: 1, export_type: 'webhook', status, idempotency_key: 'webhook_export:doc:v1', destination_ref: 'server:webhook:v1', destination_label: 'Server-configured webhook', attempts, ...(status === 'retrying' ? { next_attempt_at: '2026-07-23T12:05:02Z' } : {}), ...(error_summary ? { error_summary } : {}), created_at: '2026-07-23T12:05:00Z', updated_at: '2026-07-23T12:05:00Z' });
		const approved = { document: { id: documentID, status: 'approved', created_at: '2026-07-23T12:00:00Z', updated_at: '2026-07-23T12:05:00Z', media_type: 'application/pdf', approved_version_number: 1, approved_at: '2026-07-23T12:05:00Z', versions: [version], audit: [], exports: [] } };
		const states = [approved, { document: { ...approved.document, exports: [record('pending', 0)] } }, { document: { ...approved.document, exports: [record('retrying', 1, 'webhook delivery temporary failure')] } }, { document: { ...approved.document, status: 'exported', exports: [record('succeeded', 2)] } }];
		let detailIndex = 0;
		globalThis.fetch = vi.fn().mockImplementation((path: string) => {
			if (path.endsWith('/export/webhook')) return Promise.resolve(response(202, { export: record('pending', 0) }));
			const next = states[Math.min(detailIndex, states.length - 1)];
			detailIndex += 1;
			return Promise.resolve(response(200, next));
		});
		render(<AppShell documentID={documentID} />);
		expect(await screen.findByRole('button', { name: 'Send Webhook Export' })).toBeVisible();
		fireEvent.click(screen.getByRole('button', { name: 'Send Webhook Export' }));
		fireEvent.click(screen.getByRole('button', { name: 'Confirm webhook export' }));
		expect(await screen.findByText(/Webhook delivery is pending/i)).toBeVisible();
		expect(screen.queryByText(/Webhook export succeeded/i)).not.toBeInTheDocument();
		expect(await screen.findByText(/Webhook delivery is retrying after attempt 1/i, {}, { timeout: 3000 })).toBeVisible();
		expect(screen.getByText(/webhook delivery temporary failure/i)).toBeVisible();
		expect(await screen.findByText(/Webhook export succeeded after 2 attempts/i, {}, { timeout: 3000 })).toBeVisible();
	}, 10000);

	it('shows a webhook dead-letter and retains an accessible manual refresh after polling', async () => {
		const documentID = '0d0c2342-2486-4f10-a858-e75bc763f3e4';
		const version = { version_number: 1, source: 'extraction', created_at: '2026-07-23T12:00:00Z', proposal: {}, normalized: {}, warnings: [], evidence: [], diagnostics: [], rounding_policy_version: 'money-v1', editable: { supplier_name: 'Fictional Vendor', supplier_email: '', invoice_number: 'INV-1', issue_date: '2026-07-20', due_date: '', currency: 'USD', subtotal: '20.00', tax_amount: '4.00', total: '24.00', line_items: [] } };
		const pending = { id: 'exp-deadletter', document_id: documentID, version_number: 1, export_type: 'webhook', status: 'pending', idempotency_key: 'webhook_export:doc:v1', destination_ref: 'server:webhook:v1', destination_label: 'Server-configured webhook', attempts: 0, created_at: '2026-07-23T12:05:00Z', updated_at: '2026-07-23T12:05:00Z' };
		const deadLetter = { ...pending, status: 'dead_letter', attempts: 5, error_summary: 'webhook delivery failed (exhausted retries)' };
		const states = [{ document: { id: documentID, status: 'approved', media_type: 'application/pdf', approved_version_number: 1, versions: [version], audit: [], exports: [] } }, { document: { id: documentID, status: 'approved', media_type: 'application/pdf', approved_version_number: 1, versions: [version], audit: [], exports: [deadLetter] } }];
		let detailIndex = 0;
		globalThis.fetch = vi.fn().mockImplementation((path: string) => {
			if (path.endsWith('/export/webhook')) return Promise.resolve(response(202, { export: pending }));
			const next = states[Math.min(detailIndex, states.length - 1)];
			detailIndex += 1;
			return Promise.resolve(response(200, next));
		});
		render(<AppShell documentID={documentID} />);
		expect(await screen.findByRole('button', { name: 'Send Webhook Export' })).toBeVisible();
		fireEvent.click(screen.getByRole('button', { name: 'Send Webhook Export' }));
		fireEvent.click(screen.getByRole('button', { name: 'Confirm webhook export' }));
		expect(await screen.findByText(/Webhook export could not be delivered after 5 attempts/i)).toBeVisible();
		expect(screen.getAllByText(/webhook delivery failed \(exhausted retries\)/i).length).toBeGreaterThan(0);
		expect(screen.getByRole('button', { name: 'Refresh webhook status' })).toBeVisible();
	});

	it('keeps the last export state visible when a webhook status refresh fails', async () => {
		const documentID = '0d0c2342-2486-4f10-a858-e75bc763f3e4';
		const version = { version_number: 1, source: 'extraction', created_at: '2026-07-23T12:00:00Z', proposal: {}, normalized: {}, warnings: [], evidence: [], diagnostics: [], rounding_policy_version: 'money-v1', editable: { supplier_name: 'Fictional Vendor', supplier_email: '', invoice_number: 'INV-1', issue_date: '2026-07-20', due_date: '', currency: 'USD', subtotal: '20.00', tax_amount: '4.00', total: '24.00', line_items: [] } };
		const pending = { id: 'exp-refresh-error', document_id: documentID, version_number: 1, export_type: 'webhook', status: 'pending', idempotency_key: 'webhook_export:doc:v1', destination_ref: 'server:webhook:v1', destination_label: 'Server-configured webhook', attempts: 0, created_at: '2026-07-23T12:05:00Z', updated_at: '2026-07-23T12:05:00Z' };
		let detailRequests = 0;
		globalThis.fetch = vi.fn().mockImplementation((path: string) => {
			if (path.endsWith('/export/webhook')) return Promise.resolve(response(202, { export: pending }));
			detailRequests += 1;
			if (detailRequests === 1) return Promise.resolve(response(200, { document: { id: documentID, status: 'approved', media_type: 'application/pdf', approved_version_number: 1, versions: [version], audit: [], exports: [] } }));
			return Promise.reject(new TypeError('offline'));
		});
		render(<AppShell documentID={documentID} />);
		expect(await screen.findByRole('button', { name: 'Send Webhook Export' })).toBeVisible();
		fireEvent.click(screen.getByRole('button', { name: 'Send Webhook Export' }));
		fireEvent.click(screen.getByRole('button', { name: 'Confirm webhook export' }));
		expect(await screen.findByRole('alert')).toHaveTextContent('Webhook status could not be refreshed');
		expect(screen.getByText(/Webhook export queued/i)).toBeVisible();
		expect(screen.getByRole('button', { name: 'Refresh webhook status' })).toBeVisible();
	});

	it('renders failed webhook history without presenting it as a delivery success', async () => {
		const documentID = '0d0c2342-2486-4f10-a858-e75bc763f3e4';
		globalThis.fetch = vi.fn().mockResolvedValue(response(200, { document: { id: documentID, status: 'approved', media_type: 'image/png', approved_version_number: 1, versions: [{ version_number: 1, source: 'extraction', created_at: '2026-07-23T12:00:00Z', proposal: {}, normalized: {}, warnings: [], evidence: [], diagnostics: [], rounding_policy_version: 'money-v1', editable: { supplier_name: 'Vendor', supplier_email: '', invoice_number: 'INV-1', issue_date: '', due_date: '', currency: 'USD', subtotal: '', tax_amount: '', total: '', line_items: [] } }], audit: [], exports: [{ id: 'exp-failed', document_id: documentID, version_number: 1, export_type: 'webhook', status: 'failed', idempotency_key: 'webhook_export:doc:v1', destination_ref: 'server:webhook:v1', destination_label: 'Server-configured webhook', attempts: 1, error_summary: 'webhook delivery failed', created_at: '2026-07-23T12:05:00Z', updated_at: '2026-07-23T12:05:00Z' }] } }));
		render(<AppShell documentID={documentID} />);
		expect(await screen.findByText('WEBHOOK (failed)')).toBeVisible();
		expect(screen.getByText('webhook delivery failed')).toBeVisible();
		expect(screen.queryByText(/Webhook export succeeded/i)).not.toBeInTheDocument();
		expect(screen.getByRole('img', { name: 'Original invoice' })).toBeVisible();
	});

	it('renders a JPEG original in the responsive source panel', async () => {
		const documentID = '0d0c2342-2486-4f10-a858-e75bc763f3e4';
		globalThis.fetch = vi.fn().mockResolvedValue(response(200, { document: { id: documentID, status: 'needs_review', media_type: 'image/jpeg', versions: [{ version_number: 1, source: 'extraction', created_at: '2026-07-23T12:00:00Z', proposal: {}, normalized: {}, warnings: [], evidence: [], diagnostics: [], rounding_policy_version: 'money-v1', editable: { supplier_name: 'Vendor', supplier_email: '', invoice_number: 'INV-1', issue_date: '', due_date: '', currency: 'USD', subtotal: '', tax_amount: '', total: '', line_items: [] } }], audit: [] } }));
		render(<AppShell documentID={documentID} />);
		const image = await screen.findByRole('img', { name: 'Original invoice' });
		expect(image).toHaveAttribute('src', `/api/v1/documents/${documentID}/source`);
		expect(image).toHaveClass('source-image');
	});

	it('marks export confirmation as reduced-motion without changing keyboard behavior', async () => {
    const originalMatchMedia = window.matchMedia;
    window.matchMedia = ((query: string) => ({ matches: query.includes('prefers-reduced-motion'), media: query, onchange: null, addEventListener: () => undefined, removeEventListener: () => undefined, addListener: () => undefined, removeListener: () => undefined, dispatchEvent: () => false })) as typeof window.matchMedia;
    const documentID = '0d0c2342-2486-4f10-a858-e75bc763f3e4';
    globalThis.fetch = vi.fn().mockResolvedValue(response(200, { document: { id: documentID, status: 'approved', media_type: 'application/pdf', approved_version_number: 1, versions: [{ version_number: 1, source: 'extraction', created_at: '2026-07-23T12:00:00Z', proposal: {}, normalized: {}, warnings: [], evidence: [], diagnostics: [], rounding_policy_version: 'money-v1', editable: { supplier_name: 'Vendor', supplier_email: '', invoice_number: 'INV-1', issue_date: '', due_date: '', currency: 'USD', subtotal: '', tax_amount: '', total: '', line_items: [] } }], audit: [], exports: [] } }));
    render(<AppShell documentID={documentID} />);
    expect(await screen.findByRole('button', { name: 'Download CSV Export' })).toBeVisible();
    fireEvent.click(screen.getByRole('button', { name: 'Download CSV Export' }));
    expect(screen.getByRole('dialog')).toHaveClass('confirm-dialog-reduced-motion');
    window.matchMedia = originalMatchMedia;
  });
});
