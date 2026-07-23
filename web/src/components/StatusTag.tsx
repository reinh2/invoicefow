import type { ReactElement, ReactNode } from 'react';

export type StatusTone = 'neutral' | 'info' | 'warning' | 'success' | 'danger';
export function StatusTag({ children, tone }: { children: ReactNode; tone: StatusTone }): ReactElement {
  return <span className={`status-tag status-tag-${tone}`}><span className="status-dot" aria-hidden="true" />{children}</span>;
}
