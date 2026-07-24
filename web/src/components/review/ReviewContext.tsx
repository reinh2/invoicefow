import type { ReactElement } from 'react';
import type { ReviewDocument, ReviewVersion } from '../../api/documents';

/* Everything the server derived and the human did not type: warnings it raised,
   evidence it verified against the source page, sanitized adapter diagnostics,
   export attempts, and the append-only audit trail. */
export function ReviewContext({
  version,
  audit,
  exports,
}: {
  version: ReviewVersion;
  audit: ReviewDocument['audit'];
  exports?: ReviewDocument['exports'];
}): ReactElement {
  const auditVersion = (payload: unknown): string =>
    typeof payload === 'object' &&
    payload !== null &&
    'version_number' in payload &&
    typeof payload.version_number === 'number'
      ? ` · version ${payload.version_number}`
      : '';

  return (
    <div className="review-context">
      <section aria-labelledby="warnings-title">
        <h3 id="warnings-title">Server validation warnings</h3>
        {version.warnings.length ? (
          <ul className="warning-list">
            {version.warnings.map((warning, index) => (
              <li key={`${warning.code}-${index}`}>
                <strong>{warning.field}</strong>
                <span>{warning.message}</span>
              </li>
            ))}
          </ul>
        ) : (
          <p>No server validation warnings on this version.</p>
        )}
      </section>

      <section aria-labelledby="evidence-title">
        <h3 id="evidence-title">Source evidence</h3>
        {version.evidence.length ? (
          <ul className="evidence-list">
            {version.evidence.map((evidence, index) => (
              <li key={`${evidence.field}-${index}`}>
                <strong>
                  {evidence.field}, page {evidence.page_number}
                </strong>
                <q>{evidence.excerpt}</q>
              </li>
            ))}
          </ul>
        ) : (
          <p>No source evidence was supplied.</p>
        )}
      </section>

      <section aria-labelledby="diagnostics-title">
        <h3 id="diagnostics-title">Sanitized diagnostics</h3>
        {version.diagnostics.length ? (
          <ul>
            {version.diagnostics.map((diagnostic, index) => (
              <li key={`${diagnostic.code}-${index}`}>
                <strong>{diagnostic.code}</strong>: {diagnostic.message}
              </li>
            ))}
          </ul>
        ) : (
          <p>No diagnostics were retained.</p>
        )}
      </section>

      <section aria-labelledby="exports-title">
        <h3 id="exports-title">Export records</h3>
        {exports?.length ? (
          <ul className="exports-list">
            {exports.map((record) => (
              <li key={record.id}>
                <strong>
                  {record.export_type.toUpperCase()} ({record.status})
                </strong>
                <span>
                  {record.destination_label} · attempt {record.attempts} ·{' '}
                  {new Date(record.created_at).toLocaleString()}
                </span>
                {record.next_attempt_at ? (
                  <span role="status">
                    Retry scheduled for {new Date(record.next_attempt_at).toLocaleString()}
                  </span>
                ) : null}
                {record.error_summary ? (
                  <p className="export-error-summary">{record.error_summary}</p>
                ) : null}
              </li>
            ))}
          </ul>
        ) : (
          <p>No export records yet.</p>
        )}
      </section>

      <section aria-labelledby="audit-title">
        <h3 id="audit-title">Audit history</h3>
        <ol className="audit-list">
          {audit.map((event) => (
            <li key={event.sequence}>
              <strong>
                {event.action.replaceAll('_', ' ')}
                {auditVersion(event.payload)}
              </strong>
              <span>
                {new Date(event.occurred_at).toLocaleString()} · {event.actor}
              </span>
            </li>
          ))}
        </ol>
      </section>
    </div>
  );
}
