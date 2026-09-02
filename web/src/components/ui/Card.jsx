import { createContext, useContext } from 'react';

// CardHeader / CardContent / CardFooter used to be unstyled <div>s — an API
// that advertised composition and delivered nothing. They carry real geometry
// now, but only when the Card opts out of its own padding
// (`<Card padded={false}>`), so the existing padded call sites keep their
// current spacing instead of double-padding.
const CardCtx = createContext({ composed: false });

export function CardHeader({ children, className = '', divider = false }) {
  const { composed } = useContext(CardCtx);
  const base = composed ? `px-5 pt-5 pb-4 ${divider ? 'border-b border-border-subtle' : ''}` : '';
  return <div className={`${base} ${className}`}>{children}</div>;
}

export function CardContent({ children, className = '' }) {
  const { composed } = useContext(CardCtx);
  return <div className={`${composed ? 'p-5' : ''} ${className}`}>{children}</div>;
}

export function CardFooter({ children, className = '' }) {
  const { composed } = useContext(CardCtx);
  const base = composed ? 'px-5 py-4 bg-surface-sunken border-t border-border-subtle rounded-b-lg' : '';
  return <div className={`${base} ${className}`}>{children}</div>;
}

function Card({
  title,
  description,
  children,
  className = '',
  headerAction,
  interactive = false,
  padded = true,
}) {
  return (
    <CardCtx.Provider value={{ composed: !padded }}>
      <div
        className={`
          rounded-lg border border-border bg-surface shadow-1
          ${padded ? 'p-5' : ''}
          ${interactive
            ? 'transition-[background-color,border-color,box-shadow] duration-120 ease-swift hover:bg-surface-hover hover:border-border-strong focus-within:border-border-strong'
            : ''}
          ${className}
        `}
      >
        {(title || description) && (
          <div className={`flex items-start justify-between gap-4 ${padded ? 'mb-4' : 'px-5 pt-5 pb-4'}`}>
            <div>
              {title && <h3 className="text-card-title text-text">{title}</h3>}
              {description && <p className="text-body text-muted mt-1">{description}</p>}
            </div>
            {headerAction && <div className="shrink-0">{headerAction}</div>}
          </div>
        )}
        {children}
      </div>
    </CardCtx.Provider>
  );
}

export { Card };
export default Card;
