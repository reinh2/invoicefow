import { DocumentUpload } from '../../components/DocumentUpload';
import { PageFrame } from '../../components/PageFrame';
import { StatusTag } from '../../components/StatusTag';
import { ReviewWorkspace } from '../../components/ReviewWorkspace';
import type { ReactElement } from 'react';

export function AppShell({ documentID, onOpenDocument }: { documentID?: string; onOpenDocument?: (documentID: string) => void }): ReactElement {
  if (documentID) return <PageFrame app><ReviewWorkspace documentID={documentID} /></PageFrame>;
  return <PageFrame app>
    <main id="main-content" className="app-main" tabIndex={-1}>
      <header className="page-heading"><div><p className="eyebrow">Processing workspace</p><h1>Prepare an invoice for review.</h1><p>Upload one document to send it through the secure intake service. Its review workspace opens once it has been accepted.</p></div><StatusTag tone="info">Intake ready</StatusTag></header>
      <DocumentUpload onQueued={onOpenDocument} />
    </main>
  </PageFrame>;
}
