export type UploadErrorCode =
  | 'invalid_request'
  | 'invalid_file'
  | 'file_too_large'
  | 'duplicate_document'
  | 'storage_error'
  | 'internal_error'
  | 'network_error'
  | 'upload_failed'
  | 'invalid_response';

export interface QueuedDocument {
  id: string;
  status: 'queued';
}

export interface UploadDocumentResponse {
  document: QueuedDocument;
}

export interface EditableLineItem {
  description: string;
  quantity: string;
  unit_price: string;
  tax_amount: string;
  total: string;
}

export interface EditableProposal {
  supplier_name: string;
  supplier_email: string;
  invoice_number: string;
  issue_date: string;
  due_date: string;
  currency: string;
  subtotal: string;
  tax_amount: string;
  total: string;
  line_items: EditableLineItem[];
}

export interface ReviewVersion {
  version_number: number;
  source: 'extraction' | 'human_review';
  created_at: string;
  proposal: unknown;
  normalized: unknown;
  warnings: Array<{ code: string; field: string; message: string }>;
  evidence: Array<{ field: string; page_number: number; excerpt: string; bounding_box?: { left: number; top: number; right: number; bottom: number } }>;
  diagnostics: Array<{ code: string; message: string }>;
  rounding_policy_version: string;
  editable: EditableProposal;
}

export interface ReviewDocument {
  id: string;
  status: 'queued' | 'processing' | 'needs_review' | 'rejected' | 'failed';
  created_at: string;
  updated_at: string;
  media_type: 'application/pdf' | 'image/jpeg' | 'image/png';
  versions: ReviewVersion[];
  audit: Array<{ sequence: number; action: string; actor: string; payload: unknown; occurred_at: string }>;
}

interface ErrorEnvelope {
  error: { code: string; message: string; request_id?: string };
}

export class UploadRequestError extends Error {
  readonly code: string;

  constructor(code: string, message: string) {
    super(message);
    this.name = 'UploadRequestError';
    this.code = code;
  }
}

function isErrorEnvelope(value: unknown): value is ErrorEnvelope {
  if (typeof value !== 'object' || value === null || !('error' in value)) return false;
  const error = value.error;
  return typeof error === 'object' && error !== null
    && 'code' in error && typeof error.code === 'string'
    && 'message' in error && typeof error.message === 'string';
}

function isUploadDocumentResponse(value: unknown): value is UploadDocumentResponse {
  if (typeof value !== 'object' || value === null || !('document' in value)) return false;
  const document = value.document;
  return typeof document === 'object' && document !== null
    && 'id' in document && typeof document.id === 'string'
    && 'status' in document && document.status === 'queued';
}

async function readJSON(response: Response): Promise<unknown> {
  try { return await response.json(); } catch { return undefined; }
}

export async function uploadDocument(file: File, signal?: AbortSignal): Promise<QueuedDocument> {
  const formData = new FormData();
  formData.set('file', file);
  let response: Response;
  try {
    response = await fetch('/api/v1/documents', { method: 'POST', body: formData, signal });
  } catch {
    throw new UploadRequestError('network_error', 'The upload could not reach InvoiceFlow. Check your connection and try again.');
  }
  const payload = await readJSON(response);
  if (!response.ok) {
    if (isErrorEnvelope(payload)) throw new UploadRequestError(payload.error.code, payload.error.message);
    throw new UploadRequestError('upload_failed', 'InvoiceFlow could not accept this file. Please try again.');
  }
  if (!isUploadDocumentResponse(payload)) {
    throw new UploadRequestError('invalid_response', 'InvoiceFlow returned an unexpected upload response. Please try again.');
  }
  return payload.document;
}

function isEditableProposal(value: unknown): value is EditableProposal {
  if (typeof value !== 'object' || value === null) return false;
  const candidate = value as Record<string, unknown>;
  return ['supplier_name', 'supplier_email', 'invoice_number', 'issue_date', 'due_date', 'currency', 'subtotal', 'tax_amount', 'total'].every((key) => typeof candidate[key] === 'string')
    && Array.isArray(candidate.line_items);
}

function isReviewDocumentResponse(value: unknown): value is { document: ReviewDocument } {
  if (typeof value !== 'object' || value === null || !('document' in value) || typeof value.document !== 'object' || value.document === null) return false;
  const document = value.document as Record<string, unknown>;
  return typeof document.id === 'string' && typeof document.status === 'string' && typeof document.media_type === 'string'
    && Array.isArray(document.versions) && document.versions.every((version) => typeof version === 'object' && version !== null && typeof (version as Record<string, unknown>).version_number === 'number' && isEditableProposal((version as Record<string, unknown>).editable))
    && Array.isArray(document.audit);
}

async function requestJSON(path: string, init?: RequestInit): Promise<unknown> {
  let response: Response;
  try { response = await fetch(path, init); } catch { throw new UploadRequestError('network_error', 'InvoiceFlow could not reach the review service. Try again.'); }
  const payload = await readJSON(response);
  if (!response.ok) {
    if (isErrorEnvelope(payload)) throw new UploadRequestError(payload.error.code, payload.error.message);
    throw new UploadRequestError('review_failed', 'InvoiceFlow could not complete that review request. Try again.');
  }
  return payload;
}

export async function getReviewDocument(documentID: string, signal?: AbortSignal): Promise<ReviewDocument> {
  const payload = await requestJSON(`/api/v1/documents/${documentID}`, { signal });
  if (!isReviewDocumentResponse(payload)) throw new UploadRequestError('invalid_response', 'InvoiceFlow returned an unexpected review response. Try again.');
  return payload.document;
}

export async function saveHumanReview(documentID: string, baseVersion: number, proposal: EditableProposal): Promise<void> {
  await requestJSON(`/api/v1/documents/${documentID}/human-reviews`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ base_version: baseVersion, proposal }) });
}

export async function rejectReviewDocument(documentID: string): Promise<void> {
  await requestJSON(`/api/v1/documents/${documentID}/reject`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ confirm: true }) });
}
