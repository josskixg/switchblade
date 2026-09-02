import { useId } from 'react';
import { ChevronDown } from 'lucide-react';

export default function Select({
  label,
  error,
  hint,
  className = '',
  wrapperClassName = '',
  children,
  id,
  ...props
}) {
  const generatedId = useId();
  const selectId = id || generatedId;
  const errorId = `${selectId}-error`;
  const hintId = `${selectId}-hint`;
  const describedBy = error ? errorId : hint ? hintId : undefined;

  return (
    <div className={wrapperClassName}>
      {label && (
        <label htmlFor={selectId} className="block text-label text-text mb-1.5">
          {label}
        </label>
      )}
      <div className="relative">
        <select
          id={selectId}
          aria-invalid={error ? true : undefined}
          aria-describedby={describedBy}
          className={`
            w-full h-9 appearance-none rounded-sm border bg-surface text-text px-3 pr-9 text-body
            disabled:cursor-not-allowed disabled:bg-surface-sunken disabled:text-text-disabled
            transition-[background-color,border-color,box-shadow] duration-120 ease-swift cursor-pointer
            ${error ? 'border-danger' : 'border-border'}
            ${className}
          `}
          {...props}
        >
          {children}
        </select>
        <ChevronDown className="absolute right-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted pointer-events-none" aria-hidden="true" />
      </div>
      {error ? (
        <p id={errorId} className="mt-1.5 text-caption text-danger">{error}</p>
      ) : hint ? (
        <p id={hintId} className="mt-1.5 text-caption text-muted">{hint}</p>
      ) : null}
    </div>
  );
}

export { Select };
