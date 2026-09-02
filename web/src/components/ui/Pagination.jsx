import { ChevronLeft, ChevronRight } from 'lucide-react';
import Button from './Button';

export default function Pagination({
  currentPage,
  totalPages,
  onPageChange,
  className = '',
}) {
  if (totalPages <= 1) return null;

  // Page numbers to display, e.g. [0, 1, '...', 8, 9]
  const pages = [];
  const maxPagesToShow = 5;

  if (totalPages <= maxPagesToShow) {
    for (let i = 0; i < totalPages; i++) pages.push(i);
  } else {
    pages.push(0);

    let start = Math.max(1, currentPage - 1);
    let end = Math.min(totalPages - 2, currentPage + 1);

    if (currentPage <= 2) {
      end = 3;
    } else if (currentPage >= totalPages - 3) {
      start = totalPages - 4;
    }

    if (start > 1) pages.push('...');
    for (let i = start; i <= end; i++) pages.push(i);
    if (end < totalPages - 2) pages.push('...');

    pages.push(totalPages - 1);
  }

  return (
    <nav
      aria-label="Pagination"
      className={`flex flex-col sm:flex-row items-center justify-between gap-4 mt-6 pt-4 border-t border-border-subtle ${className}`}
    >
      <span className="text-caption text-muted tabular-nums lining-nums">
        Page <span className="text-text font-medium">{currentPage + 1}</span> of{' '}
        <span className="text-text font-medium">{totalPages}</span>
      </span>

      <div className="flex items-center gap-1">
        <Button
          size="icon"
          variant="secondary"
          disabled={currentPage === 0}
          onClick={() => onPageChange(currentPage - 1)}
          aria-label="Previous page"
        >
          <ChevronLeft className="w-4 h-4" aria-hidden="true" />
        </Button>

        {pages.map((p, idx) => {
          if (p === '...') {
            return (
              <span key={`dots-${idx}`} className="px-1 text-caption text-muted select-none" aria-hidden="true">
                …
              </span>
            );
          }
          const isActive = p === currentPage;
          return (
            <Button
              key={`page-${p}`}
              size="icon"
              variant={isActive ? 'primary' : 'ghost'}
              onClick={() => onPageChange(p)}
              aria-label={`Page ${p + 1}`}
              aria-current={isActive ? 'page' : undefined}
              className="text-caption tabular-nums lining-nums"
            >
              {p + 1}
            </Button>
          );
        })}

        <Button
          size="icon"
          variant="secondary"
          disabled={currentPage >= totalPages - 1}
          onClick={() => onPageChange(currentPage + 1)}
          aria-label="Next page"
        >
          <ChevronRight className="w-4 h-4" aria-hidden="true" />
        </Button>
      </div>
    </nav>
  );
}

export { Pagination };
