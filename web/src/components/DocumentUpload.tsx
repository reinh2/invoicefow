import { useState, type ChangeEvent, type DragEvent, type ReactElement } from 'react';
import { uploadDocument, UploadRequestError } from '../api/documents';
import { StatusTag } from './StatusTag';

type UploadState =
  | { kind: 'idle' }
  | { kind: 'uploading'; fileName: string }
  | { kind: 'queued' }
  | { kind: 'duplicate'; message: string }
  | { kind: 'error'; message: string };

const acceptedTypes = '.pdf,.jpg,.jpeg,.png,application/pdf,image/jpeg,image/png';

export function DocumentUpload({ onQueued }: { onQueued?: (documentID: string) => void }): ReactElement {
  const [state, setState] = useState<UploadState>({ kind: 'idle' });
  const isUploading = state.kind === 'uploading';

  const submitFile = async (file: File): Promise<void> => {
    setState({ kind: 'uploading', fileName: file.name });
    try {
      const document = await uploadDocument(file);
      setState({ kind: 'queued' });
      onQueued?.(document.id);
    } catch (error: unknown) {
      if (error instanceof UploadRequestError && error.code === 'duplicate_document') {
        setState({ kind: 'duplicate', message: error.message });
        return;
      }
      setState({ kind: 'error', message: error instanceof UploadRequestError ? error.message : 'InvoiceFlow could not accept this file. Please try again.' });
    }
  };

  const selectFile = (files: FileList | null): void => {
    const file = files?.[0];
    if (file && !isUploading) void submitFile(file);
  };

  const handleChange = (event: ChangeEvent<HTMLInputElement>): void => {
    selectFile(event.currentTarget.files);
    event.currentTarget.value = '';
  };

  const handleDrop = (event: DragEvent<HTMLLabelElement>): void => {
    event.preventDefault();
    selectFile(event.dataTransfer.files);
  };

  return <section className="upload-workspace" aria-labelledby="workspace-title">
    <div className="paper-panel" aria-hidden="true"><span>Original document</span><div className="paper-lines" /></div>
    <div className="workspace-copy">
      <p className="eyebrow">Document intake</p>
      <h2 id="workspace-title">Upload an invoice to begin processing.</h2>
      <p>Choose a PDF, JPG, or PNG. The server validates every file before it is accepted.</p>
      <form className="upload-form" aria-busy={isUploading} onSubmit={(event) => event.preventDefault()}>
        <label className="upload-dropzone" onDragOver={(event) => event.preventDefault()} onDrop={handleDrop}>
          <span className="upload-dropzone-title">Select an invoice file</span>
          <span className="upload-dropzone-hint">PDF, JPG, or PNG. You can also drop one here.</span>
          <input className="upload-input" type="file" name="file" aria-label="Invoice file" accept={acceptedTypes} disabled={isUploading} onChange={handleChange} />
        </label>
      </form>
      <p className="field-note">Accepted types are a browser hint only; server validation decides whether a file can be accepted.</p>
      <UploadStatus state={state} />
    </div>
  </section>;
}

function UploadStatus({ state }: { state: UploadState }): ReactElement {
  switch (state.kind) {
    case 'idle': return <p className="upload-status" aria-live="polite">No document selected.</p>;
    case 'uploading': return <p className="upload-status" aria-live="polite"><StatusTag tone="info">Uploading</StatusTag><span>Sending {state.fileName} for server validation.</span></p>;
    case 'queued': return <p className="upload-status" aria-live="polite"><StatusTag tone="info">Queued</StatusTag><span>The document was accepted and queued for processing. Opening its review workspace.</span></p>;
    case 'duplicate': return <p className="upload-status upload-status-warning" role="status" aria-live="polite"><StatusTag tone="warning">Duplicate</StatusTag><span>{state.message}</span></p>;
    case 'error': return <p className="upload-status upload-status-error" role="alert"><StatusTag tone="danger">Upload failed</StatusTag><span>{state.message}</span></p>;
  }
}
