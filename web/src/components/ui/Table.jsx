import { Children, isValidElement } from 'react';

export function Table({ children, className = '', containerClassName = '' }) {
  return (
    <div className={`overflow-x-auto ${containerClassName}`}>
      <table className={`w-full text-body border-collapse ${className}`}>
        {children}
      </table>
    </div>
  );
}

export function TableRow({ children, className = '', ...props }) {
  return (
    <tr
      className={`border-b border-border-subtle last:border-b-0 hover:bg-surface-hover transition-colors duration-120 ease-swift ${className}`}
      {...props}
    >
      {children}
    </tr>
  );
}

// Canonical form is <TableHead><TableRow><TableHeader/>…</TableRow></TableHead>.
// The bare form (<TableHead><TableHeader/>…</TableHead>) is still accepted so
// call sites can migrate one at a time.
export function TableHead({ children, className = '' }) {
  const wrapped = Children.toArray(children).some(
    (child) => isValidElement(child) && (child.type === TableRow || child.type === 'tr'),
  );

  return (
    <thead className={className}>
      {wrapped ? children : <tr className="border-b border-border-subtle">{children}</tr>}
    </thead>
  );
}

export function TableHeader({ children, className = '', align = 'left' }) {
  return (
    <th
      scope="col"
      className={`
        sticky top-0 z-10 bg-surface px-4 py-2 text-micro uppercase text-muted font-semibold
        border-b border-border-subtle
        ${align === 'numeric' || align === 'right' ? 'text-right' : align === 'center' ? 'text-center' : 'text-left'}
        ${className}
      `}
    >
      {children}
    </th>
  );
}

export function TableBody({ children, className = '' }) {
  return <tbody className={className}>{children}</tbody>;
}

export function TableCell({ children, className = '', align = 'left', colSpan, ...props }) {
  const numeric = align === 'numeric';
  return (
    <td
      colSpan={colSpan}
      className={`
        px-4 py-[var(--row-py)] text-text align-middle
        ${numeric ? 'text-right tabular-nums lining-nums font-mono text-code' : align === 'right' ? 'text-right' : align === 'center' ? 'text-center' : 'text-left'}
        ${className}
      `}
      {...props}
    >
      {children}
    </td>
  );
}

export default Table;
