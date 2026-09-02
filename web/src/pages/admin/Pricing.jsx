import { useMemo, useState } from 'react';
import { AlertTriangle, Pencil, Plus, Search, Tags } from 'lucide-react';
import { client } from '../../api/client';
import Card from '../../components/ui/Card';
import { Button } from '../../components/ui/Button';
import { Badge } from '../../components/ui/Badge';
import Input from '../../components/ui/Input';
import Select from '../../components/ui/Select';
import Modal from '../../components/ui/Modal';
import Skeleton from '../../components/ui/Skeleton';
import { useToast } from '../../components/ui/Toast';
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from '../../components/ui/Table';
import { useAsync } from '../shared/data';
import {
  EmptyState, ErrorState, Money, PageHeader, StatTile, apiErrorMessage, formatDate,
} from '../shared/ui';

const NANO = 1_000_000_000;

function effectiveNano(baseNano, marginBps) {
  return Math.round(((Number(baseNano) || 0) * (10000 + (Number(marginBps) || 0))) / 10000);
}

const BLANK = { model: '', input: '', output: '', margin: '2000', enabled: true };

export default function Pricing() {
  const toast = useToast();
  const [query, setQuery] = useState('');
  const [editing, setEditing] = useState(null);
  const [form, setForm] = useState(BLANK);
  const [saving, setSaving] = useState(false);

  const pricing = useAsync(
    () => client.get('/api/billing/pricing').then((r) => r.data),
    []
  );

  // Coverage: a routable model with no rate card row is served for $0.
  const catalog = useAsync(
    () =>
      client.get('/api/models').then((r) => {
        const d = r.data;
        return Array.isArray(d) ? d : Array.isArray(d?.data) ? d.data : [];
      }),
    []
  );

  const rows = useMemo(() => (Array.isArray(pricing.data) ? pricing.data : []), [pricing.data]);

  const unpriced = useMemo(() => {
    if (!Array.isArray(catalog.data) || rows.length === 0) return [];
    const priced = rows.filter((p) => p.enabled).map((p) => p.model);
    const seen = new Set();
    const out = [];
    for (const m of catalog.data) {
      const id = m.id || m;
      if (!id || seen.has(id)) continue;
      seen.add(id);
      const covered = priced.some((p) => id === p || id.startsWith(p));
      if (!covered) out.push({ id, owner: m.owned_by || '' });
    }
    return out;
  }, [catalog.data, rows]);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    return q ? rows.filter((p) => p.model.toLowerCase().includes(q)) : rows;
  }, [rows, query]);

  function openNew(model = '') {
    setForm({ ...BLANK, model });
    setEditing('new');
  }

  function openEdit(p) {
    setForm({
      model: p.model,
      input: String((Number(p.input_nano_per_mtok) || 0) / NANO),
      output: String((Number(p.output_nano_per_mtok) || 0) / NANO),
      margin: String(Number(p.margin_bps) || 0),
      enabled: Boolean(p.enabled),
    });
    setEditing(p.model);
  }

  async function save() {
    const model = form.model.trim();
    const input = Number(form.input);
    const output = Number(form.output);
    const margin = Number(form.margin);
    if (!model || !Number.isFinite(input) || !Number.isFinite(output) || input < 0 || output < 0) {
      toast.error('Enter a model name and non-negative input and output rates.');
      return;
    }
    setSaving(true);
    try {
      await client.put(`/api/billing/pricing/${encodeURIComponent(model)}`, {
        input_nano_per_mtok: Math.round(input * NANO),
        output_nano_per_mtok: Math.round(output * NANO),
        margin_bps: Number.isFinite(margin) ? Math.round(margin) : 0,
        enabled: form.enabled,
      });
      toast(`Saved rate for ${model}`);
      setEditing(null);
      pricing.reload();
    } catch (err) {
      toast.error(apiErrorMessage(err, 'Could not save the rate.'));
    } finally {
      setSaving(false);
    }
  }

  const previewIn = effectiveNano(Number(form.input) * NANO, Number(form.margin));
  const previewOut = effectiveNano(Number(form.output) * NANO, Number(form.margin));

  return (
    <div className="space-y-8">
      <PageHeader
        title="Rate card"
        description="What each model costs the customer. A model with no row here is served for nothing."
        actions={
          <Button type="button" variant="primary" size="md" onClick={() => openNew()}>
            <Plus className="h-4 w-4" aria-hidden="true" />
            Add model
          </Button>
        }
      />

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-3 lg:gap-5">
        <StatTile
          label="Priced models"
          value={rows.filter((p) => p.enabled).length}
          unit={`of ${rows.length} rows`}
          icon={Tags}
          loading={pricing.loading}
          empty={Boolean(pricing.error)}
          emptyLabel="Unavailable"
        />
        <StatTile
          label="Routable but unpriced"
          value={unpriced.length}
          unit="billed at $0"
          icon={AlertTriangle}
          loading={catalog.loading}
          empty={Boolean(catalog.error)}
          emptyLabel="Unavailable"
        />
        <StatTile
          label="Median margin"
          value={rows.length ? `${(median(rows.map((p) => p.margin_bps)) / 100).toFixed(1)}%` : 0}
          loading={pricing.loading}
          empty={Boolean(pricing.error) || rows.length === 0}
          emptyLabel={pricing.error ? 'Unavailable' : 'No rates yet'}
        />
      </div>

      {unpriced.length > 0 && (
        <Card
          title="Coverage gap"
          description="These models route today and produce no revenue. Give each a rate or disable it upstream."
        >
          <div className="flex flex-wrap gap-2">
            {unpriced.slice(0, 40).map((m) => (
              <button
                key={m.id}
                type="button"
                onClick={() => openNew(m.id)}
                className="inline-flex items-center gap-1.5 rounded-[var(--r-xs,4px)] border border-warning bg-warning/10 px-2 py-1 font-mono text-code text-warning transition-[background-color] duration-150 hover:bg-warning/20"
              >
                {m.id}
                <Plus className="h-3 w-3" aria-hidden="true" />
              </button>
            ))}
          </div>
          {unpriced.length > 40 && (
            <p className="mt-3 text-caption text-muted">
              …and {unpriced.length - 40} more.
            </p>
          )}
        </Card>
      )}

      <Card>
        <div className="mb-4 max-w-sm">
          <Input
            aria-label="Filter models"
            placeholder="Filter by model name"
            icon={<Search className="h-4 w-4" aria-hidden="true" />}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
        </div>

        {pricing.loading ? (
          <div className="space-y-2">
            {Array.from({ length: 6 }, (_, i) => (
              <Skeleton key={i} className="h-11 w-full" />
            ))}
          </div>
        ) : pricing.error ? (
          <ErrorState
            title="Could not load the rate card"
            detail={apiErrorMessage(pricing.error)}
            onRetry={pricing.reload}
          />
        ) : filtered.length === 0 ? (
          <EmptyState
            icon={Tags}
            title={query ? `No model matches “${query}”` : 'No rates configured'}
            body={
              query
                ? 'Clear the filter to see the whole card.'
                : 'Until a model has a rate, every request against it is billed at zero.'
            }
            action={{ label: 'Add a model rate', onClick: () => openNew(query) }}
          />
        ) : (
          <>
            {/* Eight columns need a desktop; below `md` each rate stacks. */}
            <ul className="divide-y divide-border-subtle md:hidden">
              {filtered.map((p) => (
                <li key={p.model} className="flex items-start justify-between gap-3 py-3">
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="truncate font-mono text-code text-text">{p.model}</span>
                      <Badge variant={p.enabled ? 'success' : 'default'}>
                        {p.enabled ? 'Live' : 'Off'}
                      </Badge>
                    </div>
                    <p className="mt-1 text-body text-text-secondary">
                      Billed{' '}
                      <Money nano={effectiveNano(p.input_nano_per_mtok, p.margin_bps)} sign="none" />
                      <span className="text-muted"> / </span>
                      <Money nano={effectiveNano(p.output_nano_per_mtok, p.margin_bps)} sign="none" />
                      <span className="text-muted"> per 1M</span>
                    </p>
                    <p className="mt-0.5 text-caption text-muted">
                      Margin {((Number(p.margin_bps) || 0) / 100).toFixed(1)}% · updated{' '}
                      {formatDate(p.updated_at)}
                    </p>
                  </div>
                  <Button type="button" variant="ghost" size="sm" onClick={() => openEdit(p)}>
                    <Pencil className="h-3.5 w-3.5" aria-hidden="true" />
                  </Button>
                </li>
              ))}
            </ul>

            <div className="hidden md:block">
              <Table>
                <TableHead>
                  <TableRow>
                    <TableHeader>Model</TableHeader>
                    <TableHeader align="numeric">List in / 1M</TableHeader>
                    <TableHeader align="numeric">List out / 1M</TableHeader>
                    <TableHeader align="numeric">Margin</TableHeader>
                    <TableHeader align="numeric">Billed in / out</TableHeader>
                    <TableHeader>Status</TableHeader>
                    <TableHeader align="numeric">Updated</TableHeader>
                    <TableHeader align="right">Edit</TableHeader>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {filtered.map((p) => (
                    <TableRow key={p.model}>
                      <TableCell className="font-mono text-code text-text">{p.model}</TableCell>
                      <TableCell align="numeric" className="text-text-secondary">
                        <Money nano={p.input_nano_per_mtok} sign="none" />
                      </TableCell>
                      <TableCell align="numeric" className="text-text-secondary">
                        <Money nano={p.output_nano_per_mtok} sign="none" />
                      </TableCell>
                      <TableCell align="numeric" className="text-text-secondary">
                        {((Number(p.margin_bps) || 0) / 100).toFixed(1)}%
                      </TableCell>
                      <TableCell align="numeric">
                        <Money nano={effectiveNano(p.input_nano_per_mtok, p.margin_bps)} sign="none" />
                        <span className="text-muted"> / </span>
                        <Money nano={effectiveNano(p.output_nano_per_mtok, p.margin_bps)} sign="none" />
                      </TableCell>
                      <TableCell>
                        <Badge variant={p.enabled ? 'success' : 'default'}>
                          {p.enabled ? 'Live' : 'Off'}
                        </Badge>
                      </TableCell>
                      <TableCell align="numeric" className="text-caption text-muted">
                        {formatDate(p.updated_at)}
                      </TableCell>
                      <TableCell align="right">
                        <Button type="button" variant="ghost" size="sm" onClick={() => openEdit(p)}>
                          <Pencil className="h-3.5 w-3.5" aria-hidden="true" />
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          </>
        )}
      </Card>

      <Modal
        open={Boolean(editing)}
        onClose={() => setEditing(null)}
        title={editing === 'new' ? 'Add model rate' : `Edit ${form.model}`}
      >
        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault();
            save();
          }}
        >
          <Input
            label="Model"
            placeholder="gpt-4o-mini"
            value={form.model}
            disabled={editing !== 'new'}
            onChange={(e) => setForm({ ...form, model: e.target.value })}
          />
          <div className="grid grid-cols-2 gap-4">
            <Input
              label="Input USD / 1M tokens"
              type="number"
              min="0"
              step="0.000001"
              value={form.input}
              onChange={(e) => setForm({ ...form, input: e.target.value })}
            />
            <Input
              label="Output USD / 1M tokens"
              type="number"
              min="0"
              step="0.000001"
              value={form.output}
              onChange={(e) => setForm({ ...form, output: e.target.value })}
            />
          </div>
          <div className="grid grid-cols-2 gap-4">
            <Input
              label="Margin (basis points)"
              type="number"
              step="1"
              value={form.margin}
              onChange={(e) => setForm({ ...form, margin: e.target.value })}
            />
            <Select
              label="Status"
              value={form.enabled ? 'on' : 'off'}
              onChange={(e) => setForm({ ...form, enabled: e.target.value === 'on' })}
            >
              <option value="on">Live</option>
              <option value="off">Off</option>
            </Select>
          </div>

          <div className="rounded-[var(--r-md,8px)] border border-border-subtle bg-surface-sunken p-4">
            <p className="text-label text-muted">Customer pays</p>
            <p className="mt-1 font-mono text-body text-text">
              <Money nano={previewIn} sign="none" /> in
              <span className="text-muted"> · </span>
              <Money nano={previewOut} sign="none" /> out
              <span className="text-muted"> per 1M tokens</span>
            </p>
            <p className="mt-1 text-caption text-muted">
              {((Number(form.margin) || 0) / 100).toFixed(1)}% on top of the list rate.
            </p>
          </div>

          <div className="flex justify-end gap-2 pt-2">
            <Button type="button" variant="ghost" size="md" onClick={() => setEditing(null)}>
              Cancel
            </Button>
            <Button type="submit" variant="primary" size="md" loading={saving}>
              Save rate
            </Button>
          </div>
        </form>
      </Modal>
    </div>
  );
}

function median(nums) {
  const list = nums.map((n) => Number(n) || 0).sort((a, b) => a - b);
  if (list.length === 0) return 0;
  const mid = Math.floor(list.length / 2);
  return list.length % 2 ? list[mid] : (list[mid - 1] + list[mid]) / 2;
}
