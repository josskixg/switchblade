import { useEffect, useState } from 'react';
import { client } from '../../api/client';
import { useToast } from '../../components/ui/Toast';
import Card from '../../components/ui/Card';
import Button from '../../components/ui/Button';
import Input from '../../components/ui/Input';
import Modal from '../../components/ui/Modal';
import Skeleton from '../../components/ui/Skeleton';
import Pagination from '../../components/ui/Pagination';
import { Badge } from '../../components/ui/Badge';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '../../components/ui/Table';
import { Plus, Trash2, FilterX, Search } from 'lucide-react';

export default function Filters() {
  const [rules, setRules] = useState([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(0);
  const [modalOpen, setModalOpen] = useState(false);
  const [form, setForm] = useState({ rule_id: '', pattern: '', replacement: '', is_regex: false, sort_order: 0 });
  const toast = useToast();

  const PAGE_SIZE = 8;

  useEffect(() => { load(); }, []);

  async function load() {
    try {
      const res = await client.get('/api/filters');
      setRules(Array.isArray(res.data) ? res.data : []);
    } catch {
      toast.error('Failed to load filter rules');
    } finally {
      setLoading(false);
    }
  }

  async function add() {
    try {
      await client.post('/api/filters', form);
      toast('Filter rule created');
      setModalOpen(false);
      setForm({ rule_id: '', pattern: '', replacement: '', is_regex: false, sort_order: 0 });
      load();
    } catch {
      toast.error('Failed to create rule');
    }
  }

  async function remove(id) {
    if (!window.confirm('Delete this filter rule?')) return;
    try {
      await client.delete(`/api/filters/${id}`);
      toast('Rule deleted');
      load();
    } catch {
      toast.error('Delete failed');
    }
  }

  const filtered = rules.filter((r) => {
    const q = search.trim().toLowerCase();
    if (!q) return true;
    return (
      (r.rule_id || '').toLowerCase().includes(q) ||
      (r.pattern || '').toLowerCase().includes(q) ||
      (r.replacement || '').toLowerCase().includes(q)
    );
  });

  const totalPages = Math.ceil(filtered.length / PAGE_SIZE);
  const pageData = filtered.slice(page * PAGE_SIZE, (page + 1) * PAGE_SIZE);

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-text">Filter Rules</h1>
          <p className="text-sm text-muted mt-1">Request filtering and blocking rules</p>
        </div>
        <Button size="sm" onClick={() => setModalOpen(true)}>
          <Plus className="w-3.5 h-3.5" /> Add Rule
        </Button>
      </div>

      {rules.length > 0 && (
        <div className="max-w-xs">
          <Input
            placeholder="Search filter rules..."
            value={search}
            onChange={(e) => { setSearch(e.target.value); setPage(0); }}
            icon={<Search className="w-4 h-4 text-muted/50" />}
          />
        </div>
      )}

      <Card>
        {loading ? (
          <div className="space-y-2">
            {Array.from({ length: 3 }).map((_, i) => (
              <Skeleton key={i} className="h-12 w-full" />
            ))}
          </div>
        ) : filtered.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-12 gap-2 text-muted">
            <FilterX className="w-8 h-8 text-muted/30" />
            <p className="text-sm">{search ? 'No rules match search query.' : 'No filter rules configured'}</p>
          </div>
        ) : (
          <>
            <Table>
              <TableHead>
                <TableRow>
                  <TableHeader>Rule ID</TableHeader>
                  <TableHeader>Pattern</TableHeader>
                  <TableHeader>Replacement</TableHeader>
                  <TableHeader>Type</TableHeader>
                  <TableHeader>Status</TableHeader>
                  <TableHeader className="w-24 text-right">Actions</TableHeader>
                </TableRow>
              </TableHead>
              <TableBody>
                {pageData.map((r) => {
                  const id = r.rule_id || r.id;
                  return (
                    <TableRow key={id}>
                      <TableCell className="font-semibold text-text-secondary">{r.rule_id || '—'}</TableCell>
                      <TableCell className="font-mono text-xs text-text-secondary select-all">{r.pattern}</TableCell>
                      <TableCell className="font-mono text-xs text-muted select-all">{r.replacement || '—'}</TableCell>
                      <TableCell>
                        <Badge variant="default" className="text-[10px] uppercase font-sans">
                          {r.is_regex ? 'Regex' : 'Text'}
                        </Badge>
                      </TableCell>
                      <TableCell>
                        <Badge variant={r.is_active ? 'success' : 'default'}>
                          <span className={`w-1.5 h-1.5 rounded-full mr-1.5 ${r.is_active ? 'bg-success' : 'bg-muted'}`} />
                          {r.is_active ? 'Active' : 'Disabled'}
                        </Badge>
                      </TableCell>
                      <TableCell>
                        <div className="flex justify-end">
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => remove(id)}
                            className="text-danger hover:text-danger hover:bg-danger/10"
                          >
                            <Trash2 className="w-4 h-4" />
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  );
                })}
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

      <Modal open={modalOpen} onClose={() => setModalOpen(false)} title="Add Filter Rule">
        <div className="space-y-4">
          <Input label="Rule ID" value={form.rule_id} onChange={(e) => setForm({ ...form, rule_id: e.target.value })} placeholder="redact-secret" required />
          <Input label="Pattern" value={form.pattern} onChange={(e) => setForm({ ...form, pattern: e.target.value })} placeholder="secret" required />
          <Input label="Replacement" value={form.replacement} onChange={(e) => setForm({ ...form, replacement: e.target.value })} placeholder="[REDACTED]" />
          <Input label="Sort Order" type="number" value={form.sort_order} onChange={(e) => setForm({ ...form, sort_order: Number(e.target.value) })} />
          <label className="flex items-center gap-2 text-sm text-text cursor-pointer select-none">
            <input type="checkbox" checked={form.is_regex} onChange={(e) => setForm({ ...form, is_regex: e.target.checked })} className="rounded border-border bg-surface text-primary focus:ring-primary/50" />
            Regular expression
          </label>
          <div className="flex gap-2 justify-end pt-2">
            <Button variant="ghost" onClick={() => setModalOpen(false)}>Cancel</Button>
            <Button onClick={add} disabled={!form.rule_id.trim() || !form.pattern.trim()} variant="primary">Create Rule</Button>
          </div>
        </div>
      </Modal>
    </div>
  );
}
