import { Fragment } from 'react';
import { ChevronRight } from 'lucide-react';

/**
 * Twenty pages duplicated the same `<div><h1 className="text-2xl font-bold">…`
 * block, which is why AdminPanel had already drifted to text-xl.
 *
 * `breadcrumb` is [{ label, href }]; the last entry renders as plain text.
 */
export default function PageHeader({ title, description, actions, breadcrumb, className = '' }) {
  return (
    <header className={`flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between ${className}`}>
      <div className="min-w-0">
        {Array.isArray(breadcrumb) && breadcrumb.length > 0 && (
          <nav aria-label="Breadcrumb" className="mb-1.5 flex items-center gap-1 text-caption text-muted">
            {breadcrumb.map((crumb, i) => (
              <Fragment key={`${crumb.label}-${i}`}>
                {i > 0 && <ChevronRight className="w-3 h-3 shrink-0" aria-hidden="true" />}
                {crumb.href && i < breadcrumb.length - 1 ? (
                  <a href={crumb.href} className="hover:text-text transition-colors duration-120 ease-swift">
                    {crumb.label}
                  </a>
                ) : (
                  <span className={i === breadcrumb.length - 1 ? 'text-text-secondary' : ''}>{crumb.label}</span>
                )}
              </Fragment>
            ))}
          </nav>
        )}
        <h1 className="text-page-title text-text truncate">{title}</h1>
        {description && <p className="mt-1 text-body text-muted max-w-[68ch]">{description}</p>}
      </div>
      {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
    </header>
  );
}

export { PageHeader };
