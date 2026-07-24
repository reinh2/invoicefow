import type { ChangeEvent, ReactElement } from 'react';
import type { EditableLineItem, EditableProposal, ReviewVersion } from '../../api/documents';

export type ReviewWarning = ReviewVersion['warnings'][number];

const blankLine = (): EditableLineItem => ({
  description: '',
  quantity: '',
  unit_price: '',
  tax_amount: '',
  total: '',
});

/* Server warnings carry the exact field they concern ("total",
   "line_items.0.unit_price"), so they are shown on the input itself rather than
   only in the summary list further down the page. That is what lets a reviewer
   see which value the server could not verify without cross-referencing. */
function warningsByField(warnings: ReviewWarning[]): Map<string, ReviewWarning[]> {
  const index = new Map<string, ReviewWarning[]>();
  for (const warning of warnings) {
    const existing = index.get(warning.field);
    if (existing) existing.push(warning);
    else index.set(warning.field, [warning]);
  }
  return index;
}

const warningElementID = (field: string): string => `warning-${field.replaceAll('.', '-')}`;

function FieldWarnings({
  field,
  warnings,
}: {
  field: string;
  warnings: ReviewWarning[];
}): ReactElement | null {
  if (warnings.length === 0) return null;
  return (
    <ul className="field-warning" id={warningElementID(field)}>
      {warnings.map((warning, index) => (
        <li key={`${warning.code}-${index}`}>{warning.message}</li>
      ))}
    </ul>
  );
}

export function ReviewForm({
  value,
  disabled,
  onChange,
  warnings,
}: {
  value: EditableProposal;
  disabled: boolean;
  onChange: (proposal: EditableProposal) => void;
  warnings: ReviewWarning[];
}): ReactElement {
  const index = warningsByField(warnings);
  const forField = (field: string): ReviewWarning[] => index.get(field) ?? [];

  /* The warning list sits outside the <label> on purpose: nesting it would fold
     the warning text into the input's accessible name ("Total 12.00 does not
     match…") instead of leaving it as a description. */
  const field = (
    name: Exclude<keyof EditableProposal, 'line_items'>,
    label: string,
    type = 'text',
  ): ReactElement => {
    const fieldWarnings = forField(name);
    const flagged = fieldWarnings.length > 0;
    return (
      <div className={`review-field${flagged ? ' review-field-warned' : ''}`}>
        <label>
          <span>{label}</span>
          <input
            type={type}
            value={value[name]}
            disabled={disabled}
            aria-invalid={flagged || undefined}
            aria-describedby={flagged ? warningElementID(name) : undefined}
            onChange={(event) => onChange({ ...value, [name]: event.currentTarget.value })}
          />
        </label>
        <FieldWarnings field={name} warnings={fieldWarnings} />
      </div>
    );
  };

  const lineChange = (
    lineIndex: number,
    key: keyof EditableLineItem,
    event: ChangeEvent<HTMLInputElement>,
  ): void => {
    const next = event.currentTarget.value;
    onChange({
      ...value,
      line_items: value.line_items.map((line, currentIndex) =>
        currentIndex === lineIndex ? { ...line, [key]: next } : line,
      ),
    });
  };

  const lineField = (
    lineIndex: number,
    key: keyof EditableLineItem,
    label: string,
    line: EditableLineItem,
  ): ReactElement => {
    const name = `line_items.${lineIndex}.${key}`;
    const fieldWarnings = forField(name);
    const flagged = fieldWarnings.length > 0;
    return (
      <div className={`review-field${flagged ? ' review-field-warned' : ''}`}>
        <label>
          <span>{label}</span>
          <input
            value={line[key]}
            inputMode={key === 'description' ? undefined : 'decimal'}
            aria-invalid={flagged || undefined}
            aria-describedby={flagged ? warningElementID(name) : undefined}
            onChange={(event) => lineChange(lineIndex, key, event)}
          />
        </label>
        <FieldWarnings field={name} warnings={fieldWarnings} />
      </div>
    );
  };

  return (
    <form
      className="review-form"
      onSubmit={(event) => event.preventDefault()}
      aria-label="Editable invoice proposal"
    >
      <fieldset disabled={disabled}>
        <legend>Invoice metadata</legend>
        <div className="review-grid">
          {field('supplier_name', 'Supplier name')}
          {field('supplier_email', 'Supplier email', 'email')}
          {field('invoice_number', 'Invoice number')}
          {field('issue_date', 'Issue date', 'date')}
          {field('due_date', 'Due date', 'date')}
          {field('currency', 'Currency')}
        </div>
        <div className="review-grid review-money">
          {field('subtotal', 'Subtotal')}
          {field('tax_amount', 'Tax amount')}
          {field('total', 'Total')}
        </div>
        <div className="line-items-heading">
          <h3>Line items</h3>
          <button
            className="button button-quiet"
            type="button"
            onClick={() => onChange({ ...value, line_items: [...value.line_items, blankLine()] })}
          >
            Add line item
          </button>
        </div>
        {value.line_items.length === 0 ? (
          <p className="field-note">No line items were extracted.</p>
        ) : (
          <ol className="line-items">
            {value.line_items.map((line, lineIndex) => (
              <li key={lineIndex}>
                <div className="review-grid line-item-grid">
                  {lineField(lineIndex, 'description', 'Description', line)}
                  {lineField(lineIndex, 'quantity', 'Quantity', line)}
                  {lineField(lineIndex, 'unit_price', 'Unit price', line)}
                  {lineField(lineIndex, 'tax_amount', 'Tax amount', line)}
                  {lineField(lineIndex, 'total', 'Line total', line)}
                </div>
                <button
                  className="remove-line"
                  type="button"
                  onClick={() =>
                    onChange({
                      ...value,
                      line_items: value.line_items.filter((_, index) => index !== lineIndex),
                    })
                  }
                >
                  Remove line item {lineIndex + 1}
                </button>
              </li>
            ))}
          </ol>
        )}
      </fieldset>
    </form>
  );
}
