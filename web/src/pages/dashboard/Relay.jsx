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
import StatusIndicator from '../../components/ui/StatusIndicator';
import { Plus, Trash2, Activity, Network, Inbox, Search } from 'lucide-react';

export default function Relay() {
  const [nodes, setNodes] = useState([]);
  const [stats, setStats] = useState(null);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(0);
  const [modalOpen, setModalOpen] = useState(false);
  const [form, setForm] = useState({ name: '', url: '' });
  const toast = useToast();

  const PAGE_SIZE = 8;

  useEffect(() => { load(); }, []);

  async function load() {
    try {
      const [nodesRes, statsRes] = await Promise.all([
        client.get('/api/relay/nodes').catch(() => ({ data: [] })),
        client.get('/api/relay/stats').catch(() => ({ data: null })),
      ]);
      setNodes(Array.isArray(nodesRes.data) ? nodesRes.data : []);
      setStats(statsRes.data);
    } catch {
      toast.error('Failed to load relay nodes');
    } finally {
      setLoading(false);
    }
  }

  async function add() {
    try {
      await client.post('/api/relay/nodes', form);
      toast('Node created');
      setModalOpen(false);
      setForm({ name: '', url: '' });
      load();
    } catch {
      toast.error('Failed to create node');
    }
  }

  async function remove(id) {
    if (!window.confirm('Delete this relay node?')) return;
    try {
      await client.delete(`/api/relay/nodes/${id}`);
      toast('Node deleted');
      load();
    } catch {
      toast.error('Delete failed');
    }
  }

  const filtered = nodes.filter((n) => {
    const q = search.trim().toLowerCase();
    if (!q) return true;
    return (
      (n.name || '').toLowerCase().includes(q) ||
      (n.url || '').toLowerCase().includes(q)
    );
  });

  const totalPages = Math.ceil(filtered.length / PAGE_SIZE);
  const pageData = filtered.slice(page * PAGE_SIZE, (page + 1) * PAGE_SIZE);

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-text">Relay & Tunnel</h1>
          <p className="text-sm text-muted mt-1">Manage relay nodes and tunnel connections</p>
        </div>
        <Button size="sm" onClick={() => setModalOpen(true)}>
          <Plus className="w-3.5 h-3.5" /> Add Node
        </Button>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <Card>
          {loading ? (
            <Skeleton className="h-16 w-full" />
          ) : (
            <div className="flex items-center gap-4">
              <div className="w-11 h-11 rounded-lg flex items-center justify-center bg-emerald-400/10">
                <Activity className="w-5 h-5 text-emerald-400" />
              </div>
              <div>
                <p className="text-xs text-muted uppercase tracking-wider">Active Nodes</p>
                <p className="text-2xl font-bold text-text tabular-nums">{stats?.active ?? 0}</p>
              </div>
            </div>
          )}
        </Card>
        <Card>
          {loading ? (
            <Skeleton className="h-16 w-full" />
          ) : (
            <div className="flex items-center gap-4">
              <div className="w-11 h-11 rounded-lg flex items-center justify-center bg-blue-400/10">
                <Network className="w-5 h-5 text-blue-400" />
              </div>
              <div>
                <p className="text-xs text-muted uppercase tracking-wider">Inactive Nodes</p>
                <p className="text-2xl font-bold text-text tabular-nums">{stats?.inactive ?? 0}</p>
              </div>
            </div>
          )}
        </Card>
      </div>

      {nodes.length > 0 && (
        <div className="max-w-xs">
          <Input
            placeholder="Search nodes..."
            value={search}
            onChange={(e) => { setSearch(e.target.value); setPage(0); }}
            icon={<Search className="w-4 h-4 text-muted/50" />}
          />
        </div>
      )}

      <Card>
        <h3 className="text-base font-semibold text-text mb-4 font-sans">Nodes</h3>
        {loading ? (
          <div className="space-y-2">
            {Array.from({ length: 3 }).map((_, i) => (
              <Skeleton key={i} className="h-12 w-full" />
            ))}
          </div>
        ) : filtered.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-12 gap-2 text-muted">
            <Inbox className="w-8 h-8 text-muted/30" />
            <p className="text-sm">{search ? 'No nodes match search query.' : 'No relay nodes configured'}</p>
          </div>
        ) : (
          <>
            <Table>
              <TableHead>
                <TableRow>
                  <TableHeader>Name</TableHeader>
                  <TableHeader>URL</TableHeader>
                  <TableHeader>Created</TableHeader>
                  <TableHeader>Status</TableHeader>
                  <TableHeader className="w-24 text-right">Actions</TableHeader>
                </TableRow>
              </TableHead>
              <TableBody>
                {pageData.map((n) => (
                  <TableRow key={n.id}>
                    <TableCell className="font-medium text-text-secondary">{n.name || '—'}</TableCell>
                    <TableCell className="font-mono text-xs text-muted truncate max-w-[200px]">{n.url || '—'}</TableCell>
                    <TableCell className="text-xs text-muted">
                      {n.created_at ? new Date(n.created_at * 1000).toLocaleString() : '—'}
                    </TableCell>
                    <TableCell>
                      <StatusIndicator
                        state={n.active ? 'running' : 'stopped'}
                        label={n.active ? 'active' : 'inactive'}
                      />
                    </TableCell>
                    <TableCell>
                      <div className="flex justify-end">
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => remove(n.id)}
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

      <Modal open={modalOpen} onClose={() => setModalOpen(false)} title="Add Relay Node">
        <div className="space-y-4">
          <Input label="Name" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="relay-us-east" required />
          <Input label="URL" value={form.url} onChange={(e) => setForm({ ...form, url: e.target.value })} placeholder="https://relay.example.com" required />
          <div className="flex gap-2 justify-end pt-2">
            <Button variant="ghost" onClick={() => setModalOpen(false)}>Cancel</Button>
            <Button onClick={add} disabled={!form.url.trim() || !form.name.trim()} variant="primary">Create</Button>
          </div>
        </div>
      </Modal>
    </div>
  );
}
