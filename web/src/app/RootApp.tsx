import { useEffect, useState, type ReactElement } from 'react';
import { AppShell } from './routes/AppShell';
import { LandingPage } from './routes/LandingPage';
import { RouteErrorBoundary } from './RouteErrorBoundary';

type Route = { kind: 'landing' } | { kind: 'app' } | { kind: 'review'; documentID: string };

function currentRoute(pathname: string): Route {
  const match = /^\/app\/documents\/([0-9a-f-]+)$/.exec(pathname);
  if (match) return { kind: 'review', documentID: match[1] };
  return pathname === '/app' ? { kind: 'app' } : { kind: 'landing' };
}

export function RootApp(): ReactElement {
  const [route, setRoute] = useState<Route>(() => currentRoute(window.location.pathname));

  useEffect(() => {
    const onPopState = (): void => setRoute(currentRoute(window.location.pathname));
    window.addEventListener('popstate', onPopState);
    return () => window.removeEventListener('popstate', onPopState);
  }, []);

  const openDocument = (documentID: string): void => {
    window.history.pushState({}, '', `/app/documents/${documentID}`);
    setRoute({ kind: 'review', documentID });
  };
  return (
    <RouteErrorBoundary key={route.kind === 'review' ? route.documentID : route.kind}>
      {route.kind === 'landing' ? (
        <LandingPage />
      ) : (
        <AppShell
          documentID={route.kind === 'review' ? route.documentID : undefined}
          onOpenDocument={openDocument}
        />
      )}
    </RouteErrorBoundary>
  );
}
