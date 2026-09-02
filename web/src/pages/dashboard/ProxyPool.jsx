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
import { Plus, Trash2, Power, Network, Search } from 'lucide-react';

export default function ProxyPool() {
  const [proxies, setProxies] = useState([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(0);
  const [modalOpen, setModalOpen] = useState(false);
  const [form, setForm] = useState({ url: '', type: 'http', label: '' });
  const toast = useToast();

  const PAGE_SIZE = 8;

  useEffect(() => { load(); }, []);

  async function load() {
    try {
      const res = await client.get('/api/proxy-pool');
      setProxies(Array.isArray(res.data) ? res.data : []);
    } catch {
      toast.error('Failed to load proxy pool');
    } finally {
      setLoading(false);
    }
  }

  async function add() {
    try {
      await client.post('/api/proxy-pool', form);
      toast('Proxy added');
      setModalOpen(false);
      setForm({ url: '', type: 'http', label: '' });
      load();
    } catch {
      toast.error('Failed to add proxy');
    }
  }

  async function toggle(p) {
    try {
      const status = p.status === 'active' ? 'disabled' : 'active';
      await client.put(`/api/proxy-pool/${p.id}`, { status });
      toast(status === 'active' ? 'Proxy enabled' : 'Proxy disabled');
      load();
    } catch {
      toast.error('Toggle failed');
    }
  }

  async function remove(id) {
    if (!window.confirm('Delete this proxy?')) return;
    try {
      await client.delete(`/api/proxy-pool/${id}`);
      toast('Proxy deleted');
      load();
    } catch {
      toast.error('Delete failed');
    }
  }

  const filtered = proxies.filter((p) => {
    const q = search.trim().toLowerCase();
    if (!q) return true;
    return (
      (p.url || '').toLowerCase().includes(q) ||
      (p.label || '').toLowerCase().includes(q) ||
      (p.type || '').toLowerCase().includes(q)
    );
  });

  const totalPages = Math.ceil(filtered.length / PAGE_SIZE);
  const pageData = filtered.slice(page * PAGE_SIZE, (page + 1) * PAGE_SIZE);

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-text">Proxy Pool</h1>
          <p className="text-sm text-muted mt-1">Manage rotating proxy servers</p>
        </div>
        <Button size="sm" onClick={() => setModalOpen(true)}>
          <Plus className="w-3.5 h-3.5" /> Add Proxy
        </Button>
      </div>

      {proxies.length > 0 && (
        <div className="max-w-xs">
          <Input
            placeholder="Search proxy pool..."
            value={search}
            onChange={(e) => { setSearch(e.target.value); setPage(0); }}
            icon={<Search className="w-4 h-4 text-muted/50" />}
          />
        </div>
      )}

      <Card>
        {loading ? (
          <div className="space-y-2">
            {Array.from({ length: 4 }).map((_, i) => (
              <Skeleton key={i} className="h-12 w-full" />
            ))}
          </div>
        ) : filtered.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-12 gap-2 text-muted">
            <Network className="w-8 h-8 text-muted/30" />
            <p className="text-sm">{search ? 'No proxies match your search filter.' : 'No proxies configured'}</p>
          </div>
        ) : (
          <>
            <Table>
              <TableHead>
                <TableRow>
                  <TableHeader>URL</TableHeader>
                  <TableHeader>Type</TableHeader>
                  <TableHeader>Label</TableHeader>
                  <TableHeader>Status</TableHeader>
                  <TableHeader className="w-24 text-right">Actions</TableHeader>
                </TableRow>
              </TableHead>
              <TableBody>
                {pageData.map((p) => (
                  <TableRow key={p.id}>
                    <TableCell className="font-mono text-xs select-all text-text-secondary">{p.url}</TableCell>
                    <TableCell>
                      <Badge variant="default" className="uppercase text-[10px]">{p.type || 'http'}</Badge>
                    </TableCell>
                    <TableCell className="text-xs text-muted font-medium">{p.label || '—'}</TableCell>
                    <TableCell>
                      <button onClick={() => toggle(p)} className="focus:outline-none">
                        {p.status === 'active' ? (
                          <Badge variant="success">
                            <span className="w-1.5 h-1.5 rounded-full bg-success mr-1.5" />
                            Active
                          </Badge>
                        ) : (
                          <Badge variant="default">
                            <span className="w-1.5 h-1.5 rounded-full bg-muted mr-1.5" />
                            Disabled
                          </Badge>
                        )}
                      </button>
                    </TableCell>
                    <TableCell>
                      <div className="flex justify-end">
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => remove(p.id)}
                          className="text-danger hover:text-danger hover:bg-danger/10"
                        >
                          <Trash2 className="w-4 h-4" />
                        </Button>
                      </div>
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

      <Modal open={modalOpen} onClose={() => setModalOpen(false)} title="Add Proxy">
        <div className="space-y-4">
          <Input label="URL" value={form.url} onChange={(e) => setForm({ ...form, url: e.target.value })} placeholder="http://user:pass@proxy.example.com:8080" required />
          <Input label="Type" value={form.type} onChange={(e) => setForm({ ...form, type: e.target.value })} placeholder="http" />
          <Input label="Label" value={form.label} onChange={(e) => setForm({ ...form, label: e.target.value })} placeholder="US East" />
          <div className="flex gap-2 justify-end pt-2">
            <Button variant="ghost" onClick={() => setModalOpen(false)}>Cancel</Button>
            <Button onClick={add} disabled={!form.url.trim()} variant="primary">Add Proxy</Button>
          </div>
        </div>
      </Modal>
    </div>
  );
}
