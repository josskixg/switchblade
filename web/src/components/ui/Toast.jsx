import { createContext, useContext, useState, useCallback, useMemo } from 'react';
import { CheckCircle2, AlertTriangle, XCircle, Info, X } from 'lucide-react';

const ToastCtx = createContext();

export function useToast() {
  return useContext(ToastCtx);
}

const kinds = {
  success: { Icon: CheckCircle2, chip: 'bg-success/12 text-success' },
  error: { Icon: XCircle, chip: 'bg-danger/12 text-danger' },
  warning: { Icon: AlertTriangle, chip: 'bg-warning/12 text-warning' },
  info: { Icon: Info, chip: 'bg-primary/12 text-primary' },
};

export function ToastProvider({ children }) {
  const [toasts, setToasts] = useState([]);

  const dismiss = useCallback((id) => {
    setToasts((prev) => prev.filter((t) => t.id !== id));
  }, []);

  const show = useCallback((type, message, options) => {
    const id = `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
    setToasts((prev) => [...prev, { id, type, message, action: options?.action }]);
    const ttl = options?.duration ?? (type === 'error' ? 7000 : 4000);
    setTimeout(() => setToasts((prev) => prev.filter((t) => t.id !== id)), ttl);
    return id;
  }, []);

  // The context value is the callable `toast(message)` with .error/.warning/
  // .info hung off it — unchanged from the original shape.
  const toast = useMemo(() => {
    const fn = (message, options) => show('success', message, options);
    fn.success = (message, options) => show('success', message, options);
    fn.error = (message, options) => show('error', message, options);
    fn.warning = (message, options) => show('warning', message, options);
    fn.info = (message, options) => show('info', message, options);
    fn.dismiss = dismiss;
    return fn;
  }, [show, dismiss]);

  return (
    <ToastCtx.Provider value={toast}>
      {children}
      <div
        aria-live="polite"
        aria-atomic="false"
        className="fixed bottom-4 right-4 left-4 sm:left-auto z-[110] flex flex-col gap-2 w-auto sm:w-80 max-w-sm ml-auto pointer-events-none"
      >
        {toasts.map((t) => {
          const { Icon, chip } = kinds[t.type] || kinds.info;
          return (
            <div
              key={t.id}
              className="pointer-events-auto flex items-start gap-3 p-3 rounded-lg border border-border bg-surface-elevated shadow-2 animate-slide-in-right"
            >
              <span className={`inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-xs ${chip}`}>
                <Icon className="w-3.5 h-3.5" aria-hidden="true" />
              </span>
              <div className="flex-1 min-w-0">
                <p className="text-body text-text break-words">{t.message}</p>
                {t.action && (
                  <button
                    type="button"
                    onClick={() => {
                      t.action.onClick?.();
                      dismiss(t.id);
                    }}
                    className="mt-1.5 text-label text-primary hover:underline"
                  >
                    {t.action.label}
                  </button>
                )}
              </div>
              <button
                type="button"
                onClick={() => dismiss(t.id)}
                aria-label="Dismiss notification"
                className="inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-xs text-muted hover:bg-surface-hover hover:text-text transition-[background-color,color] duration-120 ease-swift"
              >
                <X className="w-3.5 h-3.5" aria-hidden="true" />
              </button>
            </div>
          );
        })}
      </div>
    </ToastCtx.Provider>
  );
}

export default ToastProvider;
