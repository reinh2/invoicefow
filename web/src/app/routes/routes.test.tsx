import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { AppShell } from './AppShell';
import { LandingPage } from './LandingPage';

const originalFetch = globalThis.fetch;

afterEach(() => {
  globalThis.fetch = originalFetch;
  vi.restoreAllMocks();
});

function response(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

describe('routes', () => {
  it('renders an honest landing page with a skip link', () => {
    render(<LandingPage />);
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('deliberate place');
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
});
