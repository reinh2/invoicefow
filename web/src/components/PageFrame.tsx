import type { ReactElement, ReactNode } from 'react';

export function PageFrame({ children, app = false }: { children: ReactNode; app?: boolean }): ReactElement {
  const focusMainContent = (): void => document.getElementById('main-content')?.focus();

  return <div className={app ? 'site-shell site-shell-app' : 'site-shell'}>
    <a className="skip-link" href="#main-content" onClick={focusMainContent}>Skip to main content</a>
    <header className="site-header"><a className="wordmark" href="/" aria-label="InvoiceFlow home"><span aria-hidden="true">◫</span> InvoiceFlow</a><nav aria-label="Primary navigation"><a href="/">Overview</a><a href="/app">Workspace</a></nav></header>
    {children}
    <footer className="site-footer"><span>InvoiceFlow</span><span>Human review is required by design.</span></footer>
  </div>;
}
