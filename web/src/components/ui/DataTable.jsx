import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from './Table';
import Skeleton from './Skeleton';
import EmptyState from './EmptyState';
import ErrorState from './ErrorState';
import Pagination from './Pagination';

/**
 * Table + loading + empty + error + pagination behind one interface, so no
 * page hand-rolls the four states again.
 *
 * columns: [{ key, header, align, render(row, i), className, headerClassName }]
 * state:   'loading' | 'error' | 'ready'  (empty is derived from rows.length)
 */
export default function DataTable({
  columns = [],
  rows = [],
  state = 'ready',
  rowKey,
  error,
  onRetry,
  empty,
  page,
  totalPages,
  onPage,
  skeletonRows = 5,
  className = '',
}) {
  const showPager = typeof page === 'number' && typeof totalPages === 'number' && onPage;

  const shell = (children) => (
    <div className={`rounded-lg border border-border bg-surface shadow-1 overflow-hidden ${className}`}>
      {children}
    </div>
  );

  if (state === 'error') {
    return shell(
      <ErrorState
        title={error?.title || 'Could not load this table'}
        detail={error?.detail}
        requestId={error?.requestId}
        onRetry={onRetry}
      />,
    );
  }

  if (state !== 'loading' && rows.length === 0) {
    return shell(
      <EmptyState
        icon={empty?.icon}
        title={empty?.title || 'Nothing here yet'}
        body={empty?.body}
        action={empty?.action}
        secondaryAction={empty?.secondaryAction}
      />,
    );
  }

  return shell(
    <>
      <Table>
        <TableHead>
          <TableRow>
            {columns.map((c) => (
              <TableHeader key={c.key} align={c.align} className={c.headerClassName}>
                {c.header}
              </TableHeader>
            ))}
          </TableRow>
        </TableHead>
        <TableBody>
          {state === 'loading'
            ? Array.from({ length: skeletonRows }).map((_, i) => (
              <TableRow key={`sk-${i}`}>
                {columns.map((c) => (
                  <TableCell key={c.key} align={c.align}>
                    <Skeleton className="h-4 w-3/4" />
                  </TableCell>
                ))}
              </TableRow>
            ))
            : rows.map((row, i) => (
              <TableRow key={rowKey ? rowKey(row, i) : (row.id ?? i)}>
                {columns.map((c) => (
                  <TableCell key={c.key} align={c.align} className={c.className}>
                    {c.render ? c.render(row, i) : row[c.key]}
                  </TableCell>
                ))}
              </TableRow>
            ))}
        </TableBody>
      </Table>

      {showPager && (
        <div className="px-4 pb-4">
          <Pagination currentPage={page} totalPages={totalPages} onPageChange={onPage} />
        </div>
      )}
    </>,
  );
}

export { DataTable };
