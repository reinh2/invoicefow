import { useEffect, useRef, type ReactElement } from 'react';
import { useReducedMotion } from '../../motion/useReducedMotion';

/* Approval, rejection, and both exports are irreversible or outward-facing, so
   each one goes through this dialog. It traps Tab inside itself and restores
   focus to the control that opened it, which is what makes those confirmations
   reachable without a mouse. */
export function ConfirmDialog({
  title,
  children,
  confirmLabel,
  onConfirm,
  onClose,
  disabled,
  danger = false,
}: {
  title: string;
  children: ReactElement | ReactElement[];
  confirmLabel: string;
  onConfirm: () => void;
  onClose: () => void;
  disabled: boolean;
  danger?: boolean;
}): ReactElement {
  const dialogRef = useRef<HTMLElement>(null);
  const reducedMotion = useReducedMotion();

  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null;
    const dialog = dialogRef.current;
    const focusables = (): HTMLElement[] =>
      dialog
        ? Array.from(
            dialog.querySelectorAll<HTMLElement>(
              'button:not([disabled]), [href], input:not([disabled]), [tabindex]:not([tabindex="-1"])',
            ),
          )
        : [];
    focusables()[0]?.focus();

    const keydown = (event: KeyboardEvent): void => {
      if (event.key === 'Escape') {
        event.preventDefault();
        if (!disabled) onClose();
        return;
      }
      if (event.key !== 'Tab') return;
      const elements = focusables();
      if (!elements.length) return;
      const first = elements[0];
      const last = elements[elements.length - 1];
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    };

    document.addEventListener('keydown', keydown);
    return () => {
      document.removeEventListener('keydown', keydown);
      previous?.focus();
    };
  }, [disabled, onClose]);

  return (
    <div className="confirm-overlay">
      <div className="confirm-backdrop" aria-hidden="true" />
      <section
        ref={dialogRef}
        className={`confirm-dialog${reducedMotion ? ' confirm-dialog-reduced-motion' : ''}`}
        role="dialog"
        aria-modal="true"
        aria-labelledby="confirm-title"
        tabIndex={-1}
      >
        <h2 id="confirm-title">{title}</h2>
        {children}
        <div className="review-actions">
          <button
            className={`button ${danger ? 'button-danger' : 'button-primary'}`}
            type="button"
            disabled={disabled}
            onClick={onConfirm}
          >
            {confirmLabel}
          </button>
          <button
            className="button button-quiet"
            type="button"
            disabled={disabled}
            onClick={onClose}
          >
            Cancel
          </button>
        </div>
      </section>
    </div>
  );
}
