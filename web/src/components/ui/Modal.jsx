import { useEffect, useId, useRef } from 'react';
import { X } from 'lucide-react';

const FOCUSABLE =
  'a[href], button:not([disabled]), textarea:not([disabled]), input:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])';

function Modal({ open, onClose, title, description, children, footer, width = 'max-w-lg' }) {
  const overlayRef = useRef(null);
  const panelRef = useRef(null);
  const restoreRef = useRef(null);
  // Two mounted modals used to collide on a hardcoded id="modal-title".
  const titleId = useId();
  const descId = useId();

  useEffect(() => {
    if (!open) return undefined;

    restoreRef.current = document.activeElement;

    // Scroll lock.
    const prevOverflow = document.body.style.overflow;
    document.body.style.overflow = 'hidden';

    const onKeyDown = (e) => {
      if (e.key === 'Escape') {
        onClose();
        return;
      }
      if (e.key !== 'Tab') return;

      // Focus trap.
      const panel = panelRef.current;
      if (!panel) return;
      const items = Array.from(panel.querySelectorAll(FOCUSABLE)).filter(
        (el) => el.offsetParent !== null || el === document.activeElement,
      );
      if (items.length === 0) {
        e.preventDefault();
        return;
      }
      const first = items[0];
      const last = items[items.length - 1];
      if (e.shiftKey && document.activeElement === first) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault();
        first.focus();
      }
    };

    document.addEventListener('keydown', onKeyDown);

    // Move focus into the dialog.
    const raf = requestAnimationFrame(() => {
      const panel = panelRef.current;
      if (!panel) return;
      const target = panel.querySelector(FOCUSABLE);
      (target || panel).focus();
    });

    return () => {
      document.removeEventListener('keydown', onKeyDown);
      cancelAnimationFrame(raf);
      document.body.style.overflow = prevOverflow;
      const restore = restoreRef.current;
      if (restore && typeof restore.focus === 'function') restore.focus();
    };
  }, [open, onClose]);

  if (open === false) return null;

  return (
    <div
      ref={overlayRef}
      className="fixed inset-0 z-[100] flex items-center justify-center bg-overlay/55 backdrop-blur-sm p-4 animate-fade-in"
      onClick={(e) => {
        if (e.target === overlayRef.current) onClose();
      }}
    >
      <div
        ref={panelRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        aria-describedby={description ? descId : undefined}
        tabIndex={-1}
        className={`
          w-full ${width} max-h-[calc(100vh-2rem)] flex flex-col overflow-hidden
          rounded-xl bg-surface-elevated shadow-3 dark:border dark:border-border-strong
          animate-scale-in focus:outline-none
        `}
      >
        <div className="flex items-start justify-between gap-4 px-5 pt-5 pb-4 border-b border-border-subtle shrink-0">
          <div>
            <h2 id={titleId} className="text-section text-text">{title}</h2>
            {description && (
              <p id={descId} className="text-body text-muted mt-1">{description}</p>
            )}
          </div>
          <button
            type="button"
            onClick={onClose}
            aria-label="Close dialog"
            className="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-sm text-muted hover:bg-surface-hover hover:text-text transition-[background-color,color] duration-120 ease-swift"
          >
            <X className="w-4 h-4" aria-hidden="true" />
          </button>
        </div>

        <div className="p-5 overflow-y-auto">{children}</div>

        {footer && (
          <div className="px-5 py-4 bg-surface-sunken border-t border-border-subtle shrink-0">
            {footer}
          </div>
        )}
      </div>
    </div>
  );
}

export { Modal };
export default Modal;
