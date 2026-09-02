import { useId } from 'react';

export default function Input({
  label,
  error,
  hint,
  icon,
  trailing,
  type = 'text',
  className = '',
  wrapperClassName = '',
  id,
  ...props
}) {
  const generatedId = useId();
  const inputId = id || generatedId;
  const errorId = `${inputId}-error`;
  const hintId = `${inputId}-hint`;
  const describedBy = error ? errorId : hint ? hintId : undefined;

  return (
    <div className={wrapperClassName}>
      {label && (
        <label htmlFor={inputId} className="block text-label text-text mb-1.5">
          {label}
        </label>
      )}
      <div className="relative">
        {icon && (
          <div className="absolute left-3 top-1/2 -translate-y-1/2 text-muted pointer-events-none">
            {icon}
          </div>
        )}
        <input
          id={inputId}
          type={type}
          aria-invalid={error ? true : undefined}
          aria-describedby={describedBy}
          className={`
            w-full h-9 rounded-sm border bg-surface text-text px-3 text-body
            placeholder:text-muted
            disabled:cursor-not-allowed disabled:bg-surface-sunken disabled:text-text-disabled
            transition-[background-color,border-color,box-shadow] duration-120 ease-swift
            ${icon ? 'pl-10' : ''}
            ${trailing ? 'pr-10' : ''}
            ${error ? 'border-danger' : 'border-border'}
            ${className}
          `}
          {...props}
        />
        {trailing && (
          <div className="absolute right-3 top-1/2 -translate-y-1/2 text-muted">
            {trailing}
          </div>
        )}
      </div>
      {error ? (
        <p id={errorId} className="mt-1.5 text-caption text-danger">{error}</p>
      ) : hint ? (
        <p id={hintId} className="mt-1.5 text-caption text-muted">{hint}</p>
      ) : null}
    </div>
  );
}

export { Input };
