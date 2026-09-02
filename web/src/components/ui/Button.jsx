// One filled primary surface per viewport. Everything else is secondary or
// ghost. No shadow, no glow, no transform on hover — hover is a background
// and border change.
const variantStyles = {
  primary:
    'bg-primary text-primary-foreground border border-transparent hover:bg-primary-hover active:bg-primary-dim',
  secondary:
    'bg-transparent border border-border-strong text-text hover:bg-surface-hover',
  ghost:
    'bg-transparent border border-transparent text-text-secondary hover:bg-surface-hover hover:text-text',
  // True destructive confirmation.
  danger:
    'bg-danger text-danger-foreground border border-transparent hover:opacity-90',
  // In-table destructive trigger — reads as dangerous without shouting.
  'danger-subtle':
    'bg-danger/10 text-danger border border-danger hover:bg-danger/20',
};

const sizeStyles = {
  sm: 'h-8 px-3 text-label rounded-sm',
  md: 'h-9 px-3.5 text-body rounded-md',
  lg: 'h-11 px-5 text-body rounded-md',
  icon: 'h-8 w-8 p-0 shrink-0 rounded-sm',
};

function Button({
  children,
  variant = 'primary',
  size = 'md',
  disabled = false,
  loading = false,
  className = '',
  // 90 of 94 call sites inherited type="submit" and quietly submitted the
  // form they sat in. Submits are explicit now.
  type = 'button',
  ...props
}) {
  const variantClass = variantStyles[variant] || variantStyles.primary;
  const sizeClass = sizeStyles[size] || sizeStyles.md;

  return (
    <button
      type={type}
      disabled={disabled || loading}
      aria-busy={loading || undefined}
      className={`
        inline-flex items-center justify-center gap-2 font-medium whitespace-nowrap
        transition-[background-color,border-color,color,box-shadow] duration-120 ease-swift
        disabled:cursor-not-allowed disabled:bg-transparent disabled:text-text-disabled
        disabled:border-border disabled:hover:bg-transparent
        ${variantClass} ${sizeClass} ${className}
      `}
      {...props}
    >
      {loading && (
        <svg className="animate-spin h-4 w-4 shrink-0" viewBox="0 0 24 24" fill="none" aria-hidden="true">
          <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
          <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
        </svg>
      )}
      {children}
    </button>
  );
}

export { Button };
export default Button;
