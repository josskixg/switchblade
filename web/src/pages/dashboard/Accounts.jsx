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
import { Plus, Pencil, Trash2, Search, Users } from 'lucide-react';

const emptyForm = {
  provider: '', email: '', password: '', tokens: '', quota_limit: '', status: 'pending', enabled: true,
};

function normalizeAccounts(data) {
  const accounts = Array.isArray(data) ? data : data?.accounts;
  return Array.isArray(accounts) ? accounts : [];
}

function apiError(error, fallback) {
  return error.response?.data?.error || error.message || fallback;
}

export default function Accounts() {
  const [accounts, setAccounts] = useState([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(0);
  const [modalOpen, setModalOpen] = useState(false);
  const [editing, setEditing] = useState(null);
  const [form, setForm] = useState(emptyForm);
  const [saving, setSaving] = useState(false);
  const toast = useToast();

  const PAGE_SIZE = 8;

  useEffect(() => { fetchAccounts(); }, []);

  async function fetchAccounts() {
    try {
      const res = await client.get('/api/accounts');
      setAccounts(normalizeAccounts(res.data));
    } catch (error) {
      setAccounts([]);
      toast.error(apiError(error, 'Failed to load accounts'));
    } finally {
      setLoading(false);
    }
  }

  function openCreate() {
    setEditing(null);
    setForm(emptyForm);
    setModalOpen(true);
  }

  function openEdit(acc) {
    setEditing(acc);
    setForm({ ...emptyForm, status: acc.status || 'pending', enabled: Boolean(acc.enabled) });
    setModalOpen(true);
  }

  async function save() {
    setSaving(true);
    try {
      if (editing) {
        await client.put(`/api/accounts/${editing.id}`, { status: form.status, enabled: form.enabled });
        toast('Account updated');
      } else {
        await client.post('/api/accounts', {
          provider: form.provider.trim(),
          email: form.email.trim(),
          password: form.password,
          tokens: form.tokens,
          quota_limit: Number(form.quota_limit) || 0,
        });
        toast('Account created');
      }
      await fetchAccounts();
      setModalOpen(false);
    } catch (error) {
      toast.error(apiError(error, editing ? 'Update failed' : 'Create failed'));
    } finally {
      setSaving(false);
    }
  }

  async function remove(id) {
    if (!window.confirm('Are you sure you want to delete this account?')) return;
    try {
      await client.delete(`/api/accounts/${id}`);
      await fetchAccounts();
      toast('Account deleted');
    } catch (error) {
      toast.error(apiError(error, 'Delete failed'));
    }
  }

  const filtered = accounts.filter((acc) => {
    const q = search.trim().toLowerCase();
    if (!q) return true;
    return (
      (acc.email || '').toLowerCase().includes(q) ||
      (acc.provider || '').toLowerCase().includes(q)
    );
  });

  const totalPages = Math.ceil(filtered.length / PAGE_SIZE);
  const pageData = filtered.slice(page * PAGE_SIZE, (page + 1) * PAGE_SIZE);

  const handleSearchChange = (e) => {
    setSearch(e.target.value);
    setPage(0);
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-text">Accounts</h1>
          <p className="text-sm text-muted mt-1">Manage multiple credentials per provider</p>
        </div>
        <Button onClick={openCreate} variant="primary">
          <Plus className="w-4 h-4" /> New Account
        </Button>
      </div>

      <div className="max-w-xs">
        <Input
          placeholder="Search accounts or providers..."
          value={search}
          onChange={handleSearchChange}
          icon={<Search className="w-4 h-4 text-muted/50" />}
        />
      </div>

      <Card>
        {loading ? (
          <div className="space-y-3">
            {[1, 2, 3, 4].map((i) => <Skeleton key={i} className="h-12 w-full" />)}
          </div>
        ) : filtered.length === 0 ? (
          <div className="py-12 text-center">
            <p className="text-sm text-muted">
              {search ? 'No accounts match your search.' : 'No accounts configured yet.'}{' '}
              {!search && (
                <button onClick={openCreate} className="text-primary hover:underline font-medium">Create one →</button>
              )}
            </p>
          </div>
        ) : (
          <>
            <Table>
              <TableHead>
                <TableRow>
                  <TableHeader>Email</TableHeader>
                  <TableHeader>Provider</TableHeader>
                  <TableHeader>Quota Remaining / Limit</TableHeader>
                  <TableHeader>Status</TableHeader>
                  <TableHeader className="w-24 text-right">Actions</TableHeader>
                </TableRow>
              </TableHead>
              <TableBody>
                {pageData.map((acc) => (
                  <TableRow key={acc.id}>
                    <TableCell className="font-medium">{acc.email || '—'}</TableCell>
                    <TableCell>
                      <Badge variant="default" className="capitalize">{acc.provider}</Badge>
                    </TableCell>
                    <TableCell className="text-xs text-muted tabular-nums font-mono">
                      {acc.quota_remaining ?? 0} / {acc.quota_limit ?? 0}
                    </TableCell>
                    <TableCell>
                      <Badge variant={acc.enabled && acc.status === 'active' ? 'success' : 'default'}>
                        <span className={`w-1.5 h-1.5 rounded-full mr-1.5 ${acc.enabled && acc.status === 'active' ? 'bg-success' : 'bg-muted'}`} />
                        {acc.enabled ? acc.status : 'disabled'}
                      </Badge>
                    </TableCell>
                    <TableCell>
                      <div className="flex gap-1 justify-end">
                        <Button variant="ghost" size="sm" onClick={() => openEdit(acc)}>
                          <Pencil className="w-3.5 h-3.5" />
                        </Button>
                        <Button variant="ghost" size="sm" className="text-danger hover:text-danger hover:bg-danger/10" onClick={() => remove(acc.id)}>
                          <Trash2 className="w-3.5 h-3.5" />
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

      <Modal open={modalOpen} onClose={() => setModalOpen(false)} title={editing ? 'Edit Account' : 'New Account'}>
        <div className="space-y-4">
          {editing ? (
            <>
              <Input label="Status" value={form.status} onChange={(e) => setForm({ ...form, status: e.target.value })} placeholder="active" />
              <label className="flex items-center gap-2 text-sm text-text cursor-pointer">
                <input type="checkbox" checked={form.enabled} onChange={(e) => setForm({ ...form, enabled: e.target.checked })} className="rounded border-border bg-surface text-primary focus:ring-primary/50" />
                Enabled
              </label>
            </>
          ) : (
            <>
              <Input label="Provider" value={form.provider} onChange={(e) => setForm({ ...form, provider: e.target.value })} placeholder="openai" required />
              <Input label="Email" type="email" value={form.email} onChange={(e) => setForm({ ...form, email: e.target.value })} placeholder="account@example.com" required />
              <Input label="Password" type="password" value={form.password} onChange={(e) => setForm({ ...form, password: e.target.value })} required />
              <Input label="Tokens (JSON)" value={form.tokens} onChange={(e) => setForm({ ...form, tokens: e.target.value })} placeholder="{}" />
              <Input label="Quota Limit" type="number" min="0" step="any" value={form.quota_limit} onChange={(e) => setForm({ ...form, quota_limit: e.target.value })} placeholder="0" />
            </>
          )}
          <div className="flex gap-2 justify-end pt-2">
            <Button variant="ghost" onClick={() => setModalOpen(false)}>Cancel</Button>
            <Button onClick={save} loading={saving} disabled={!editing && (!form.provider.trim() || !form.email.trim() || !form.password)}>{editing ? 'Update' : 'Create'}</Button>
          </div>
        </div>
      </Modal>
    </div>
  );
}
