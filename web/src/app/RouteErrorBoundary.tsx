import { Component, type ErrorInfo, type ReactNode } from 'react';

interface Props { children: ReactNode }
interface State { hasError: boolean }

export class RouteErrorBoundary extends Component<Props, State> {
  public state: State = { hasError: false };

  public static getDerivedStateFromError(): State { return { hasError: true }; }

  public componentDidCatch(_error: Error, _info: ErrorInfo): void {
    // A server-side error reporting integration has not been configured for this foundation.
  }

  public render(): ReactNode {
    if (this.state.hasError) {
      return <main className="route-error" aria-labelledby="route-error-title"><h1 id="route-error-title">This view could not load</h1><p>Refresh the page to try again.</p></main>;
    }
    return this.props.children;
  }
}
