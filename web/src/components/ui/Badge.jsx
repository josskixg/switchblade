// Every variant carries a 1px solid border, including `default` — without it
// a mixed row of badges is 2px misaligned in height.
// `info` is not a distinct color: it maps onto primary.
const variantStyles = {
  default: 'bg-surface-sunken text-text-secondary border-border',
  primary: 'bg-primary/10 text-primary border-primary/30',
  info: 'bg-primary/10 text-primary border-primary/30',
  success: 'bg-success/10 text-success border-success/30',
  warning: 'bg-warning/10 text-warning border-warning/30',
  danger: 'bg-danger/10 text-danger border-danger/30',
  neutral: 'bg-transparent text-muted border-border',
};

const sizeStyles = {
  sm: 'h-[18px] px-1.5 gap-1 text-micro uppercase',
  md: 'h-[22px] px-2 gap-1.5 text-caption font-medium',
};

export function Badge({ children, variant = 'default', size = 'md', className = '' }) {
  return (
    <span
      className={`
        inline-flex items-center rounded-xs border whitespace-nowrap
        ${variantStyles[variant] || variantStyles.default}
        ${sizeStyles[size] || sizeStyles.md}
        ${className}
      `}
    >
      {children}
    </span>
  );
}

export default Badge;
