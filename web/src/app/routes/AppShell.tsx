import { useState, type ReactElement } from 'react';
import { DocumentList } from '../../components/DocumentList';
import { DocumentUpload } from '../../components/DocumentUpload';
import { PageFrame } from '../../components/PageFrame';
import { StatusTag } from '../../components/StatusTag';
import { ReviewWorkspace } from '../../components/ReviewWorkspace';

export function AppShell({
  documentID,
  onOpenDocument,
}: {
  documentID?: string;
  onOpenDocument?: (documentID: string) => void;
}): ReactElement {
  // A completed upload refreshes the list so the new document appears without a
  // manual reload, whether or not the caller navigates straight into it.
  const [uploads, setUploads] = useState(0);

  if (documentID)
    return (
      <PageFrame app>
        <ReviewWorkspace documentID={documentID} />
      </PageFrame>
    );
  return (
    <PageFrame app>
      <main id="main-content" className="app-main" tabIndex={-1}>
        <header className="page-heading">
          <div>
            <p className="eyebrow">Processing workspace</p>
            <h1>Prepare an invoice for review.</h1>
            <p>
              Upload one document to send it through the secure intake service. Its review workspace
              opens once it has been accepted.
            </p>
          </div>
          <StatusTag tone="info">Intake ready</StatusTag>
        </header>
        <DocumentUpload
          onQueued={(id) => {
            setUploads((value) => value + 1);
            onOpenDocument?.(id);
          }}
        />
        <DocumentList onOpenDocument={onOpenDocument} reloadToken={uploads} />
      </main>
    </PageFrame>
  );
}
