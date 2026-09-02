import { useState, useEffect } from 'react';
import { client } from '../../api/client';
import { Card, CardHeader, CardContent } from '../../components/ui/Card';
import { Button } from '../../components/ui/Button';
import { Badge } from '../../components/ui/Badge';
import { Skeleton } from '../../components/ui/Skeleton';
import { Table, TableHead, TableBody, TableRow, TableCell, TableHeader } from '../../components/ui/Table';
import { Modal } from '../../components/ui/Modal';
import Input from '../../components/ui/Input';
import Select from '../../components/ui/Select';
import { useToast } from '../../components/ui/Toast';
import {
  Users, Shield, Ban, CheckCircle, Trash2,
  TrendingUp, DollarSign, Activity,
  UserPlus, Inbox
} from 'lucide-react';

export default function AdminPanel() {
  const toast = useToast();
  const [tenants, setTenants] = useState([]);
  const [stats, setStats] = useState(null);
  const [loading, setLoading] = useState(true);
  const [editingTenant, setEditingTenant] = useState(null);
  const [tierModal, setTierModal] = useState(false);
  const [tiers, setTiers] = useState([]);
  const [selectedTier, setSelectedTier] = useState('');
  const [users, setUsers] = useState([]);
  const [showUserModal, setShowUserModal] = useState(false);
  const [newUser, setNewUser] = useState({ username: '', password: '', email: '', role: 'developer' });

  useEffect(() => { load(); }, []);

  async function load() {
    try {
      setLoading(true);
      const [tRes, sRes, tierRes] = await Promise.all([
        client.get('/api/tenants'),
        client.get('/api/admin/stats'),
        client.get('/api/tiers'),
      ]);
      setTenants(tRes.data || []);
      setStats(sRes.data);
      setTiers(tierRes.data || []);
      try {
        const uRes = await client.get('/api/auth/users');
        setUsers(uRes.data || []);
      } catch { /* non-owner can't list — fine */ }
    } catch (e) {
      toast.error(e.message);
    } finally {
      setLoading(false);
    }
  }

  async function createUser() {
    try {
      await client.post('/api/auth/users', newUser);
      toast('User created');
      setShowUserModal(false);
      setNewUser({ username: '', password: '', email: '', role: 'developer' });
      load();
    } catch (e) {
      toast.error(e.response?.data?.error || e.message);
    }
  }

  async function toggleStatus(id, current) {
    try {
      const next = current === 'active' ? 'suspended' : 'active';
      await client.put(`/api/tenants/${id}`, { status: next });
      if (next === 'active') toast('Tenant activated');
      else toast.warning('Tenant suspended');
      load();
    } catch (e) {
      toast.error(e.message);
    }
  }

  async function assignTier(id) {
    if (!selectedTier) return;
    try {
      await client.put(`/api/tenants/${id}/tier`, { tier_id: selectedTier });
      toast('Tier assigned');
      setTierModal(false);
      setSelectedTier('');
      load();
    } catch (e) {
      toast.error(e.message);
    }
  }

  async function deleteTenant(id) {
    if (!confirm('Delete this tenant and all their data?')) return;
    try {
      await client.delete(`/api/tenants/${id}`);
      toast('Tenant deleted');
      load();
    } catch (e) {
      toast.error(e.message);
    }
  }

  if (loading) {
    return (
      <div className="space-y-6">
        <Skeleton className="h-8 w-48" />
        <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
          {[...Array(4)].map((_, i) => (
            <Skeleton key={i} className="h-28" />
          ))}
        </div>
        <Skeleton className="h-80" />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold text-text">Owner Admin</h1>
          <p className="text-sm text-muted">Manage tenants, tiers, and platform health</p>
        </div>
        <Button variant="primary" size="sm" onClick={load}>
          Refresh
        </Button>
      </div>

      {stats && (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
          <StatCard icon={Users} label="Active Tenants" value={stats.active_tenants ?? 0} color="text-primary" />
          <StatCard icon={TrendingUp} label="Total Requests (24h)" value={stats.total_requests_24h ?? 0} color="text-green-400" />
          <StatCard icon={Activity} label="Tokens Used (24h)" value={formatNum(stats.tokens_used_24h ?? 0)} color="text-blue-400" />
          <StatCard icon={DollarSign} label="Est. Revenue (mo)" value={`$${(stats.revenue_estimate_cents ?? 0) / 100}`} color="text-yellow-400" />
        </div>
      )}

      {users.length > 0 && (
        <Card>
          <CardHeader className="flex flex-row items-center justify-between mb-4">
            <h2 className="text-lg font-medium text-text">Users</h2>
            <Button variant="primary" size="sm" onClick={() => setShowUserModal(true)}>
              <UserPlus className="w-4 h-4 mr-1" /> Add User
            </Button>
          </CardHeader>
          <CardContent>
            <Table>
              <TableHead>
                <TableHeader>Username</TableHeader>
                <TableHeader>Email</TableHeader>
                <TableHeader>Role</TableHeader>
                <TableHeader>Status</TableHeader>
              </TableHead>
              <TableBody>
                {(users || []).map((u) => (
                  <TableRow key={u.username}>
                    <TableCell className="font-medium text-text">{u.username}</TableCell>
                    <TableCell className="text-muted">{u.email || '—'}</TableCell>
                    <TableCell>
                      <Badge variant={u.role === 'owner' ? 'success' : 'default'}>{u.role}</Badge>
                    </TableCell>
                    <TableCell>
                      <Badge variant={u.status === 'active' ? 'success' : 'danger'}>{u.status}</Badge>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      )}

      <Card>
        <CardHeader className="mb-4">
          <h2 className="text-lg font-medium text-text">Tenants</h2>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHead>
              <TableHeader>Name</TableHeader>
              <TableHeader>Email</TableHeader>
              <TableHeader>Tier</TableHeader>
              <TableHeader>Status</TableHeader>
              <TableHeader>Requests (24h)</TableHeader>
              <TableHeader>Created</TableHeader>
              <TableHeader className="text-right">Actions</TableHeader>
            </TableHead>
            <TableBody>
              {tenants.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={7}>
                    <div className="flex flex-col items-center justify-center py-10 gap-2 text-muted">
                      <Inbox className="w-8 h-8 text-muted/30" />
                      <p className="text-sm">No tenants yet</p>
                    </div>
                  </TableCell>
                </TableRow>
              ) : (
                (tenants || []).map((t) => (
                  <TableRow key={t.id}>
                    <TableCell className="font-medium text-text">{t.name}</TableCell>
                    <TableCell className="text-muted">{t.email || '—'}</TableCell>
                    <TableCell>
                      <Badge variant="default">{t.tier_name || 'free'}</Badge>
                    </TableCell>
                    <TableCell>
                      <Badge variant={t.status === 'active' ? 'success' : 'danger'}>{t.status}</Badge>
                    </TableCell>
                    <TableCell className="text-muted">{t.requests_24h ?? 0}</TableCell>
                    <TableCell className="text-muted text-xs">{t.created_at?.slice(0, 10)}</TableCell>
                    <TableCell>
                      <div className="flex justify-end gap-1">
                        <Button variant="ghost" size="sm" onClick={() => { setEditingTenant(t); setTierModal(true); }} title="Assign tier">
                          <Shield className="w-3.5 h-3.5" />
                        </Button>
                        <Button variant="ghost" size="sm" onClick={() => toggleStatus(t.id, t.status)} title={t.status === 'active' ? 'Suspend' : 'Activate'}>
                          {t.status === 'active'
                            ? <Ban className="w-3.5 h-3.5 text-danger" />
                            : <CheckCircle className="w-3.5 h-3.5 text-green-400" />
                          }
                        </Button>
                        <Button variant="ghost" size="sm" onClick={() => deleteTenant(t.id)} title="Delete">
                          <Trash2 className="w-3.5 h-3.5 text-danger" />
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      <Modal open={tierModal} onClose={() => { setTierModal(false); setSelectedTier(''); }} title={`Assign Tier: ${editingTenant?.name || ''}`}>
        <div className="space-y-4">
          <Select
            label="Tier"
            value={selectedTier}
            onChange={(e) => setSelectedTier(e.target.value)}
          >
            <option value="">Select tier...</option>
            {(tiers || []).map((t) => (
              <option key={t.id} value={t.id}>{t.name} — ${((t.monthly_price_cents || 0) / 100).toFixed(0)}/mo</option>
            ))}
          </Select>
          <div className="flex gap-2 justify-end">
            <Button variant="ghost" size="sm" onClick={() => { setTierModal(false); setSelectedTier(''); }}>Cancel</Button>
            <Button variant="primary" size="sm" onClick={() => assignTier(editingTenant.id)} disabled={!selectedTier}>Assign</Button>
          </div>
        </div>
      </Modal>

      <Modal open={showUserModal} onClose={() => setShowUserModal(false)} title="Add User">
        <div className="space-y-4">
          <Input
            label="Username"
            placeholder="Enter username"
            value={newUser.username}
            onChange={(e) => setNewUser({ ...newUser, username: e.target.value })}
          />
          <Input
            label="Password"
            type="password"
            placeholder="Enter password"
            value={newUser.password}
            onChange={(e) => setNewUser({ ...newUser, password: e.target.value })}
          />
          <Input
            label="Email (optional)"
            placeholder="user@example.com"
            value={newUser.email}
            onChange={(e) => setNewUser({ ...newUser, email: e.target.value })}
          />
          <Select
            label="Role"
            value={newUser.role}
            onChange={(e) => setNewUser({ ...newUser, role: e.target.value })}
          >
            <option value="developer">Developer</option>
            <option value="viewer">Viewer</option>
            <option value="admin">Admin</option>
            <option value="owner">Owner</option>
          </Select>
          <div className="flex gap-2 justify-end pt-2">
            <Button variant="ghost" size="sm" onClick={() => setShowUserModal(false)}>Cancel</Button>
            <Button variant="primary" size="sm" onClick={createUser}>Create</Button>
          </div>
        </div>
      </Modal>
    </div>
  );
}

function StatCard({ icon: Icon, label, value, color }) {
  return (
    <Card>
      <CardContent className="p-4 flex items-center gap-4">
        <div className={`w-10 h-10 rounded-lg bg-surface flex items-center justify-center border border-border ${color}`}>
          <Icon className="w-5 h-5" />
        </div>
        <div>
          <p className="text-xs text-muted">{label}</p>
          <p className={`text-lg font-bold ${color}`}>{value}</p>
        </div>
      </CardContent>
    </Card>
  );
}

function formatNum(n) {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
  return String(n);
}
