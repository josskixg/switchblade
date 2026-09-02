import { useEffect, useState } from 'react';
import { client } from '../../api/client';
import { useToast } from '../../components/ui/Toast';
import Card from '../../components/ui/Card';
import Button from '../../components/ui/Button';
import Input from '../../components/ui/Input';
import Modal from '../../components/ui/Modal';
import Skeleton from '../../components/ui/Skeleton';
import Pagination from '../../components/ui/Pagination';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '../../components/ui/Table';
import { Plus, Copy, Trash2, Search } from 'lucide-react';

const scopeOptions = ['gpt-*', 'claude-*', '*'];

function normalizeKeys(data) {
  const keys = Array.isArray(data) ? data : data?.keys;
  return Array.isArray(keys) ? keys : [];
}

function normalizeCreatedKey(data) {
  return data?.key_value || data?.key || null;
}

// Ensure error fallback helper
function apiError(error, fallback) {
  return error.response?.data?.error || error.message || fallback;
}

function fromUnix(value) {
  return value ? new Date(value * 1000).toLocaleDateString() : '—';
}

export default function Keys() {
  const [keys, setKeys] = useState([]);
  const [loading, setLoading] = useState(false);
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(0);
  const [modalOpen, setModalOpen] = useState(false);
  const [name, setName] = useState('');
  const [tenantId, setTenantId] = useState('');
  const [scopes, setScopes] = useState([]);
  const [role, setRole] = useState('developer');
  const [expiresAt, setExpiresAt] = useState('');
  const [justCreated, setJustCreated] = useState(null);
  const [creating, setCreating] = useState(false);
  const toast = useToast();

  const PAGE_SIZE = 8;

  useEffect(() => {
    // If tenantId has a default in localstorage, load it
    const stored = localStorage.getItem('sb_active_tenant_id');
    if (stored) {
      setTenantId(stored);
      fetchKeys(stored);
    }
  }, []);

  async function fetchKeys(id = tenantId) {
    if (!id.trim()) {
      setKeys([]);
      setLoading(false);
      return;
    }
    setLoading(true);
    setPage(0);
    localStorage.setItem('sb_active_tenant_id', id.trim());
    try {
      const res = await client.get('/api/keys/v2', { params: { tenant_id: id.trim() } });
      setKeys(normalizeKeys(res.data));
    } catch (error) {
      setKeys([]);
      toast.error(apiError(error, 'Failed to load keys'));
    } finally {
      setLoading(false);
    }
  }

  function toggleScope(s) {
    setScopes((prev) => prev.includes(s) ? prev.filter((x) => x !== s) : [...prev, s]);
  }

  async function createKey() {
    if (!name.trim() || !tenantId.trim()) return;
    setCreating(true);
    try {
      const payload = { name: name.trim(), tenant_id: tenantId.trim(), scopes, role: role.trim() || 'developer' };
      if (expiresAt) payload.expires_at = Math.floor(new Date(expiresAt).getTime() / 1000);
      const res = await client.post('/api/keys/v2', payload);
      const keyValue = normalizeCreatedKey(res.data);
      if (!keyValue) throw new Error('API did not return the new key value');
      setJustCreated(keyValue);
      await fetchKeys(tenantId);
      toast('Key created');
      setModalOpen(false);
      setName('');
      setScopes([]);
      setRole('developer');
      setExpiresAt('');
    } catch (error) {
      toast.error(apiError(error, 'Failed to create key'));
    } finally {
      setCreating(false);
    }
  }

  async function removeKey(id) {
    if (!window.confirm('Delete this API key permanently?')) return;
    try {
      const res = await client.delete(`/api/keys/v2/${id}`);
      if (res.data?.deleted !== true) throw new Error('API did not confirm deletion');
      await fetchKeys();
      toast('Key deleted');
    } catch (error) {
      toast.error(apiError(error, 'Delete failed'));
    }
  }

  async function copyKey(key) {
    if (!key) {
      toast.error('Key values are only shown once, immediately after creation');
      return;
    }
    try {
      await navigator.clipboard.writeText(key);
      toast('Copied to clipboard');
    } catch (error) {
      toast.error(error.message || 'Copy failed');
    }
  }

  const filtered = keys.filter((k) => {
    const q = search.trim().toLowerCase();
    if (!q) return true;
    return (k.name || '').toLowerCase().includes(q);
  });

  const totalPages = Math.ceil(filtered.length / PAGE_SIZE);
  const pageData = filtered.slice(page * PAGE_SIZE, (page + 1) * PAGE_SIZE);

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex-1">
          <h1 className="text-2xl font-bold text-text">API Keys</h1>
          <p className="text-sm text-muted mt-1">Manage tenant access keys and model scopes</p>
          <div className="flex gap-2 mt-3 max-w-md">
            <Input aria-label="Tenant ID" value={tenantId} onChange={(e) => setTenantId(e.target.value)} placeholder="Enter Tenant ID..." className="h-9 py-1" />
            <Button size="sm" onClick={() => fetchKeys()} className="h-9 shrink-0">Load Keys</Button>
          </div>
        </div>
        <Button onClick={() => setModalOpen(true)} variant="primary" disabled={!tenantId.trim()} className="sm:self-end">
          <Plus className="w-4 h-4" /> New Key
        </Button>
      </div>

      {/* Just-created key display */}
      {justCreated && (
        <Card className="border-primary/30 bg-primary/5">
          <p className="text-sm text-primary font-medium mb-2">Your new API key — copy it now, you won't see it again.</p>
          <div className="flex items-center gap-2">
            <code className="flex-1 bg-background rounded-lg px-3 py-2 text-sm font-mono text-text overflow-x-auto">
              {justCreated}
            </code>
            <Button variant="ghost" size="sm" onClick={() => copyKey(justCreated)}>
              <Copy className="w-4 h-4" />
            </Button>
          </div>
          <button className="mt-2 text-xs text-muted hover:text-text transition-colors duration-150" onClick={() => setJustCreated(null)}>
            Dismiss
          </button>
        </Card>
      )}

      {tenantId.trim() && (
        <div className="max-w-xs">
          <Input
            placeholder="Filter key name..."
            value={search}
            onChange={(e) => { setSearch(e.target.value); setPage(0); }}
            icon={<Search className="w-4 h-4 text-muted/50" />}
          />
        </div>
      )}

      <Card>
        {!tenantId.trim() ? (
          <div className="py-12 text-center text-muted">
            <p className="text-sm">Please enter and load a Tenant ID to view its API keys.</p>
          </div>
        ) : loading ? (
          <div className="space-y-3">
            {[1, 2, 3].map((i) => <Skeleton key={i} className="h-12 w-full" />)}
          </div>
        ) : filtered.length === 0 ? (
          <div className="py-12 text-center">
            <p className="text-sm text-muted">
              {search ? 'No keys match your search filter.' : 'No keys generated for this tenant yet.'}{' '}
              {!search && (
                <button onClick={() => setModalOpen(true)} className="text-primary hover:underline font-medium">Create one →</button>
              )}
            </p>
          </div>
        ) : (
          <>
            <Table>
              <TableHead>
                <TableRow>
                  <TableHeader>Name</TableHeader>
                  <TableHeader>Key Value</TableHeader>
                  <TableHeader>Scopes</TableHeader>
                  <TableHeader>Created</TableHeader>
                  <TableHeader className="w-24 text-right">Actions</TableHeader>
                </TableRow>
              </TableHead>
              <TableBody>
                {pageData.map((k) => (
                  <TableRow key={k.id}>
                    <TableCell className="font-medium">{k.name}</TableCell>
                    <TableCell className="font-mono text-xs text-muted/70">Hidden after creation</TableCell>
                    <TableCell>
                      <span className="text-xs text-muted">
                        {(Array.isArray(k.scopes) ? k.scopes : []).join(', ') || '*'}
                      </span>
                    </TableCell>
                    <TableCell className="text-xs text-muted">{fromUnix(k.created_at)}</TableCell>
                    <TableCell>
                      <div className="flex justify-end">
                        <Button variant="ghost" size="sm" className="text-danger hover:text-danger hover:bg-danger/10" onClick={() => removeKey(k.id)}>
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

      <Modal open={modalOpen} onClose={() => setModalOpen(false)} title="Create API Key">
        <div className="space-y-4">
          <Input label="Tenant ID" value={tenantId} onChange={(e) => setTenantId(e.target.value)} required />
          <Input label="Key Name" value={name} onChange={(e) => setName(e.target.value)} placeholder="production" required />
          <Input label="Role" value={role} onChange={(e) => setRole(e.target.value)} placeholder="developer" />
          <Input label="Expires At" type="datetime-local" value={expiresAt} onChange={(e) => setExpiresAt(e.target.value)} />
          <div>
            <label className="block text-sm font-medium text-text mb-1.5">Scopes</label>
            <div className="space-y-2">
              {scopeOptions.map((s) => (
                <label key={s} className="flex items-center gap-2 text-sm text-muted cursor-pointer hover:text-text transition-colors duration-150">
                  <input
                    type="checkbox"
                    checked={scopes.includes(s)}
                    onChange={() => toggleScope(s)}
                    className="rounded border-border bg-surface text-primary focus:ring-primary/50"
                  />
                  <code className="text-xs font-mono">{s}</code>
                </label>
              ))}
            </div>
          </div>
          <div className="flex gap-2 justify-end pt-2">
            <Button variant="ghost" onClick={() => setModalOpen(false)}>Cancel</Button>
            <Button onClick={createKey} loading={creating} disabled={!name.trim() || !tenantId.trim()}>Create</Button>
          </div>
        </div>
      </Modal>
    </div>
  );
}
