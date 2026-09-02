import { useEffect, useState } from 'react';
import { client } from '../../api/client';
import { useToast } from '../../components/ui/Toast';
import Card from '../../components/ui/Card';
import Button from '../../components/ui/Button';
import Input from '../../components/ui/Input';
import Select from '../../components/ui/Select';
import Skeleton from '../../components/ui/Skeleton';
import Pagination from '../../components/ui/Pagination';
import { Badge } from '../../components/ui/Badge';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '../../components/ui/Table';
import { Inbox, Search } from 'lucide-react';

export default function Requests() {
  const [logs, setLogs] = useState([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(0);
  const [providerFilter, setProviderFilter] = useState('');
  const [statusFilter, setStatusFilter] = useState('');
  const toast = useToast();

  const PAGE_SIZE = 10;

  useEffect(() => { load(); }, []);

  async function load() {
    try {
      const res = await client.get('/api/logs', { params: { limit: 500 } });
      setLogs(Array.isArray(res.data) ? res.data : []);
    } catch {
      toast.error('Failed to load request logs');
    } finally {
      setLoading(false);
    }
  }

  const filtered = (logs || []).filter((l) => {
    if (providerFilter && (l.provider || '').toLowerCase() !== providerFilter.toLowerCase()) return false;
    if (statusFilter && (l.status || '') !== statusFilter) return false;
    
    const q = search.trim().toLowerCase();
    if (q) {
      const matchesSearch =
        (l.provider || '').toLowerCase().includes(q) ||
        (l.model || '').toLowerCase().includes(q) ||
        (l.error_message || '').toLowerCase().includes(q);
      if (!matchesSearch) return false;
    }
    return true;
  });

  const totalPages = Math.ceil(filtered.length / PAGE_SIZE);
  const pageData = filtered.slice(page * PAGE_SIZE, (page + 1) * PAGE_SIZE);

  const providers = [...new Set((logs || []).map((l) => l.provider).filter(Boolean))];

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-text">Requests</h1>
        <p className="text-sm text-muted mt-1">API request logs and history</p>
      </div>

      <Card>
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center justify-between mb-4">
          <div className="flex flex-wrap gap-2 items-center flex-1 max-w-2xl">
            <div className="w-full sm:w-60">
              <Input
                placeholder="Search logs..."
                value={search}
                onChange={(e) => { setSearch(e.target.value); setPage(0); }}
                icon={<Search className="w-4 h-4 text-muted/50" />}
                className="h-9 py-1"
              />
            </div>
            <div className="w-40">
              <Select
                value={providerFilter}
                onChange={(e) => { setProviderFilter(e.target.value); setPage(0); }}
                className="h-9 py-1"
              >
                <option value="">All Providers</option>
                {providers.map((p) => (
                  <option key={p} value={p}>{p}</option>
                ))}
              </Select>
            </div>
            <div className="w-40">
              <Select
                value={statusFilter}
                onChange={(e) => { setStatusFilter(e.target.value); setPage(0); }}
                className="h-9 py-1"
              >
                <option value="">All Statuses</option>
                <option value="success">Success</option>
                <option value="error">Error</option>
                <option value="timeout">Timeout</option>
              </Select>
            </div>
          </div>
          <span className="text-xs font-medium text-muted self-end sm:self-center shrink-0">
            Showing {filtered.length} of {logs.length} logs
          </span>
        </div>

        {loading ? (
          <div className="space-y-2">
            {Array.from({ length: 6 }).map((_, i) => (
              <Skeleton key={i} className="h-12 w-full" />
            ))}
          </div>
        ) : pageData.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-12 gap-2 text-muted">
            <Inbox className="w-8 h-8 text-muted/30" />
            <p className="text-sm">No request logs found</p>
          </div>
        ) : (
          <>
            <Table>
              <TableHead>
                <TableRow>
                  <TableHeader>Time</TableHeader>
                  <TableHeader>Provider</TableHeader>
                  <TableHeader>Model</TableHeader>
                  <TableHeader>Status</TableHeader>
                  <TableHeader>Tokens</TableHeader>
                  <TableHeader>Error</TableHeader>
                </TableRow>
              </TableHead>
              <TableBody>
                {pageData.map((l, i) => (
                  <TableRow key={l.id || i}>
                    <TableCell className="text-xs text-muted whitespace-nowrap">
                      {l.created_at ? new Date(l.created_at * 1000).toLocaleString() : '—'}
                    </TableCell>
                    <TableCell>
                      <Badge variant="default" className="capitalize">{l.provider || '—'}</Badge>
                    </TableCell>
                    <TableCell className="font-mono text-xs text-text-secondary">{l.model || '—'}</TableCell>
                    <TableCell>
                      <Badge variant={l.status === 'success' ? 'success' : 'default'}>
                        <span className={`w-1.5 h-1.5 rounded-full mr-1.5 ${
                          l.status === 'success' ? 'bg-success' : 'bg-muted'
                        }`} />
                        {l.status || '—'}
                      </Badge>
                    </TableCell>
                    <TableCell className="tabular-nums text-xs font-semibold text-text-secondary">
                      {l.total_tokens || '—'}
                    </TableCell>
                    <TableCell className="text-xs text-danger truncate max-w-[200px]" title={l.error_message}>
                      {l.error_message || '—'}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>

            <Pagination
              currentPage={page}
              totalPages={totalPages}
              onPageChange={setPage}
            />
          </>
        )}
      </Card>
    </div>
  );
}
