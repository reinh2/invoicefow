import { afterEach, expect, it, vi } from 'vitest';
import { triggerWebhookExport } from './documents';

const originalFetch = globalThis.fetch;

afterEach(() => {
  globalThis.fetch = originalFetch;
  vi.restoreAllMocks();
});

it('rejects a malformed webhook export response as invalid_response', async () => {
  globalThis.fetch = vi.fn().mockResolvedValue(new Response(JSON.stringify({ export: { id: 42, status: 'pending' } }), { status: 202, headers: { 'Content-Type': 'application/json' } }));

  await expect(triggerWebhookExport('0d0c2342-2486-4f10-a858-e75bc763f3e4')).rejects.toMatchObject({ code: 'invalid_response' });
});

it('accepts a complete safe export projection', async () => {
  const exportRecord = { id: 'exp-12345678', document_id: '0d0c2342-2486-4f10-a858-e75bc763f3e4', version_number: 1, export_type: 'webhook', status: 'pending', idempotency_key: 'webhook_export:doc:v1', destination_ref: 'server:webhook:v1', destination_label: 'Server-configured webhook', attempts: 0, created_at: '2026-07-24T12:00:00Z', updated_at: '2026-07-24T12:00:00Z' };
  globalThis.fetch = vi.fn().mockResolvedValue(new Response(JSON.stringify({ export: exportRecord }), { status: 202, headers: { 'Content-Type': 'application/json' } }));

  await expect(triggerWebhookExport(exportRecord.document_id)).resolves.toEqual(exportRecord);
});
