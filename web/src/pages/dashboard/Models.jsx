import { useEffect, useState } from 'react';
import { client } from '../../api/client';
import { useAuthStore } from '../../store/auth';
import { useToast } from '../../components/ui/Toast';
import Card from '../../components/ui/Card';
import Button from '../../components/ui/Button';
import Input from '../../components/ui/Input';
import Modal from '../../components/ui/Modal';
import Skeleton from '../../components/ui/Skeleton';
import Select from '../../components/ui/Select';
import Pagination from '../../components/ui/Pagination';
import { Badge } from '../../components/ui/Badge';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '../../components/ui/Table';
import {
  Boxes,
  Pencil,
  Plus,
  Search,
  Trash2,
  GitCommit,
  Route,
  Cpu,
  Layers,
  Check,
  X,
  ArrowRight,
  ListCollapse,
} from 'lucide-react';

const emptyProviderForm = { provider: '', base_url: '', enabled: true, model_ids: '' };
const emptyComboForm = { name: '', label: '', models_list: '', enabled: true };
const emptyMappingForm = { source_pattern: '', match_type: 'contains', target_model: '', priority: 0, label: '', enabled: true };

function modelGroups(payload) {
  const source = payload?.data ?? payload?.providers ?? payload;
  if (Array.isArray(source)) {
    return source.reduce((groups, item) => {
      const provider = item?.provider || item?.owned_by || 'unknown';
      const models = Array.isArray(item?.models)
        ? item.models
        : [item?.id || item?.model].filter(Boolean);
      groups[provider] = [...(groups[provider] || []), ...models.map((model) => (
        typeof model === 'string' ? model : model?.id
      )).filter(Boolean)];
      return groups;
    }, {});
  }
  if (!source || typeof source !== 'object') return {};
  return Object.fromEntries(Object.entries(source).map(([provider, value]) => {
    const models = Array.isArray(value) ? value : value?.models;
    return [provider, Array.isArray(models)
      ? models.map((model) => typeof model === 'string' ? model : model?.id).filter(Boolean)
      : []];
  }));
}

function parseModelIDs(value) {
  return [...new Set(value.split(/[\n,]+/).map((id) => id.trim()).filter(Boolean))];
}

export default function Models() {
  const role = useAuthStore((state) => state.user?.role || 'viewer');
  const canManage = role === 'owner' || role === 'admin';
  const toast = useToast();

  const [activeTab, setActiveTab] = useState('discovery'); // discovery, combos, mappings

  // Global search filters
  const [search, setSearch] = useState('');

  // Tab 1: Discovery & Config
  const [groups, setGroups] = useState({});
  const [configs, setConfigs] = useState([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState('');
  const [providerModalOpen, setProviderModalOpen] = useState(false);
  const [editingProvider, setEditingProvider] = useState(null);
  const [providerForm, setProviderForm] = useState(emptyProviderForm);
  const [savingProvider, setSavingProvider] = useState(false);

  // Tab 2: Combos
  const [combos, setCombos] = useState([]);
  const [loadingCombos, setLoadingCombos] = useState(false);
  const [comboModalOpen, setComboModalOpen] = useState(false);
  const [editingCombo, setEditingCombo] = useState(null);
  const [comboForm, setComboForm] = useState(emptyComboForm);
  const [savingCombo, setSavingCombo] = useState(false);
  const [comboPage, setComboPage] = useState(0);

  // Tab 3: Mappings
  const [mappings, setMappings] = useState([]);
  const [loadingMappings, setLoadingMappings] = useState(false);
  const [mappingModalOpen, setMappingModalOpen] = useState(false);
  const [editingMapping, setEditingMapping] = useState(null);
  const [mappingForm, setMappingForm] = useState(emptyMappingForm);
  const [savingMapping, setSavingMapping] = useState(false);
  const [mappingPage, setMappingPage] = useState(0);

  const PAGE_SIZE = 8;

  useEffect(() => {
    loadAll();
  }, []);

  async function loadAll() {
    setLoading(true);
    setLoadError('');
    await Promise.all([
      fetchDiscovery(),
      fetchCombos(),
      fetchMappings(),
    ]);
    setLoading(false);
  }

  async function fetchDiscovery() {
    const [modelsResult, configsResult] = await Promise.allSettled([
      client.get('/api/models'),
      client.get('/api/provider-config'),
    ]);

    if (modelsResult.status === 'fulfilled') {
      setGroups(modelGroups(modelsResult.value.data));
    } else {
      setGroups({});
      setLoadError('Models could not be loaded.');
    }

    if (configsResult.status === 'fulfilled') {
      setConfigs(Array.isArray(configsResult.value.data) ? configsResult.value.data : []);
    } else {
      setConfigs([]);
      if (canManage) setLoadError((error) => error || 'Provider configurations could not be loaded.');
    }
  }

  async function fetchCombos() {
    setLoadingCombos(true);
    try {
      const res = await client.get('/api/model-combos');
      setCombos(Array.isArray(res.data) ? res.data : []);
    } catch {
      setCombos([]);
    } finally {
      setLoadingCombos(false);
    }
  }

  async function fetchMappings() {
    setLoadingMappings(true);
    try {
      const res = await client.get('/api/model-mappings');
      setMappings(Array.isArray(res.data) ? res.data : []);
    } catch {
      setMappings([]);
    } finally {
      setLoadingMappings(false);
    }
  }

  // --- Provider Management Handlers ---
  function openCreateProvider() {
    setEditingProvider(null);
    setProviderForm(emptyProviderForm);
    setProviderModalOpen(true);
  }

  function openEditProvider(config) {
    setEditingProvider(config.provider);
    setProviderForm({
      provider: config.provider || '',
      base_url: config.base_url || '',
      enabled: config.enabled !== false,
      model_ids: Array.isArray(config.models) ? config.models.join('\n') : '',
    });
    setProviderModalOpen(true);
  }

  async function saveProvider() {
    const payload = {
      provider: providerForm.provider.trim(),
      base_url: providerForm.base_url.trim(),
      enabled: providerForm.enabled,
      models: parseModelIDs(providerForm.model_ids),
    };
    if (!payload.provider || !payload.base_url) return;

    setSavingProvider(true);
    try {
      await client.post('/api/provider-config', payload);
      toast(editingProvider ? 'Provider configuration updated' : 'Provider configuration added');
      setProviderModalOpen(false);
      await fetchDiscovery();
    } catch {
      toast.error('Failed to save provider configuration');
    } finally {
      setSavingProvider(false);
    }
  }

  async function removeProvider(provider) {
    if (!window.confirm(`Delete the ${provider} provider configuration?`)) return;
    try {
      await client.delete(`/api/provider-config/${encodeURIComponent(provider)}`);
      toast('Provider configuration deleted');
      await fetchDiscovery();
    } catch {
      toast.error('Failed to delete provider configuration');
    }
  }

  // --- Combo Management Handlers ---
  function openCreateCombo() {
    setEditingCombo(null);
    setComboForm(emptyComboForm);
    setComboModalOpen(true);
  }

  function openEditCombo(combo) {
    let parsedModels = '';
    try {
      const parsed = JSON.parse(combo.models_json);
      if (Array.isArray(parsed)) {
        parsedModels = parsed.join('\n');
      }
    } catch {
      parsedModels = combo.models_json || '';
    }

    setEditingCombo(combo);
    setComboForm({
      name: combo.name || '',
      label: combo.label || '',
      models_list: parsedModels,
      enabled: combo.enabled !== false,
    });
    setComboModalOpen(true);
  }

  async function saveCombo() {
    const modelsArr = parseModelIDs(comboForm.models_list);
    const payload = {
      name: comboForm.name.trim(),
      label: comboForm.label.trim(),
      models_json: JSON.stringify(modelsArr),
      enabled: comboForm.enabled,
    };
    if (!payload.name) return;

    setSavingCombo(true);
    try {
      if (editingCombo) {
        await client.put(`/api/model-combos/${editingCombo.id}`, payload);
        toast('Model combo updated');
      } else {
        await client.post('/api/model-combos', payload);
        toast('Model combo created');
      }
      setComboModalOpen(false);
      await fetchCombos();
    } catch {
      toast.error('Failed to save model combo');
    } finally {
      setSavingCombo(false);
    }
  }

  async function removeCombo(id) {
    if (!window.confirm('Delete this model combo?')) return;
    try {
      await client.delete(`/api/model-combos/${id}`);
      toast('Model combo deleted');
      await fetchCombos();
    } catch {
      toast.error('Failed to delete model combo');
    }
  }

  async function toggleComboActive(combo) {
    try {
      await client.put(`/api/model-combos/${combo.id}`, {
        name: combo.name,
        label: combo.label,
        models_json: combo.models_json,
        enabled: !combo.enabled,
      });
      toast(combo.enabled ? 'Combo disabled' : 'Combo enabled');
      await fetchCombos();
    } catch {
      toast.error('Toggle combo status failed');
    }
  }

  // --- Mapping Management Handlers ---
  function openCreateMapping() {
    setEditingMapping(null);
    setMappingForm(emptyMappingForm);
    setMappingModalOpen(true);
  }

  function openEditMapping(m) {
    setEditingMapping(m);
    setMappingForm({
      source_pattern: m.source_pattern || '',
      match_type: m.match_type || 'contains',
      target_model: m.target_model || '',
      priority: m.priority || 0,
      label: m.label || '',
      enabled: m.enabled !== false,
    });
    setMappingModalOpen(true);
  }

  async function saveMapping() {
    const payload = {
      source_pattern: mappingForm.source_pattern.trim(),
      match_type: mappingForm.match_type,
      target_model: mappingForm.target_model.trim(),
      priority: Number(mappingForm.priority) || 0,
      label: mappingForm.label.trim(),
      enabled: mappingForm.enabled,
    };
    if (!payload.source_pattern || !payload.target_model) return;

    setSavingMapping(true);
    try {
      if (editingMapping) {
        await client.put(`/api/model-mappings/${editingMapping.id}`, payload);
        toast('Model mapping updated');
      } else {
        await client.post('/api/model-mappings', payload);
        toast('Model mapping created');
      }
      setMappingModalOpen(false);
      await fetchMappings();
    } catch {
      toast.error('Failed to save model mapping');
    } finally {
      setSavingMapping(false);
    }
  }

  async function removeMapping(id) {
    if (!window.confirm('Delete this model mapping?')) return;
    try {
      await client.delete(`/api/model-mappings/${id}`);
      toast('Model mapping deleted');
      await fetchMappings();
    } catch {
      toast.error('Failed to delete mapping');
    }
  }

  async function toggleMappingActive(m) {
    try {
      await client.put(`/api/model-mappings/${m.id}`, {
        enabled: !m.enabled,
      });
      toast(m.enabled ? 'Mapping disabled' : 'Mapping enabled');
      await fetchMappings();
    } catch {
      toast.error('Toggle mapping status failed');
    }
  }

  // --- Filtering Data ---
  const query = search.trim().toLowerCase();

  // Tab 1 filtering
  const visibleGroups = Object.entries(groups)
    .map(([provider, models]) => [provider, Array.isArray(models) ? models : []])
    .map(([provider, models]) => [
      provider,
      models.filter((model) =>
        !query || provider.toLowerCase().includes(query) || model.toLowerCase().includes(query)
      ),
    ])
    .filter(([provider, models]) => models.length > 0 || (!query && provider));

  // Tab 2 filtering
  const filteredCombos = combos.filter((combo) => {
    if (!query) return true;
    return (
      (combo.name || '').toLowerCase().includes(query) ||
      (combo.label || '').toLowerCase().includes(query) ||
      (combo.models_json || '').toLowerCase().includes(query)
    );
  });
  const totalComboPages = Math.ceil(filteredCombos.length / PAGE_SIZE);
  const comboPageData = filteredCombos.slice(comboPage * PAGE_SIZE, (comboPage + 1) * PAGE_SIZE);

  // Tab 3 filtering
  const filteredMappings = mappings.filter((m) => {
    if (!query) return true;
    return (
      (m.source_pattern || '').toLowerCase().includes(query) ||
      (m.target_model || '').toLowerCase().includes(query) ||
      (m.label || '').toLowerCase().includes(query)
    );
  });
  const totalMappingPages = Math.ceil(filteredMappings.length / PAGE_SIZE);
  const mappingPageData = filteredMappings.slice(mappingPage * PAGE_SIZE, (mappingPage + 1) * PAGE_SIZE);

  return (
    <div className="space-y-6">
      {/* Header and Add Actions */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold text-text">Models & Routing</h1>
          <p className="text-sm text-muted mt-1">
            Manage provider connections, model fallback chains, and query mappings
          </p>
        </div>
        {canManage && (
          <div>
            {activeTab === 'discovery' && (
              <Button size="sm" onClick={openCreateProvider}>
                <Plus className="w-3.5 h-3.5 mr-1" /> Add Provider
              </Button>
            )}
            {activeTab === 'combos' && (
              <Button size="sm" onClick={openCreateCombo}>
                <Plus className="w-3.5 h-3.5 mr-1" /> New Combo
              </Button>
            )}
            {activeTab === 'mappings' && (
              <Button size="sm" onClick={openCreateMapping}>
                <Plus className="w-3.5 h-3.5 mr-1" /> New Mapping
              </Button>
            )}
          </div>
        )}
      </div>

      {/* Tabs Switcher */}
      <div className="flex items-center gap-2 border-b border-border">
        <button
          onClick={() => { setActiveTab('discovery'); setSearch(''); }}
          className={`flex items-center gap-2 px-4 py-2 text-sm font-semibold border-b-2 transition-all duration-150 ${
            activeTab === 'discovery'
              ? 'border-primary text-primary'
              : 'border-transparent text-muted hover:text-text'
          }`}
        >
          <Cpu className="w-4 h-4" />
          Model Discovery
        </button>
        <button
          onClick={() => { setActiveTab('combos'); setSearch(''); setComboPage(0); }}
          className={`flex items-center gap-2 px-4 py-2 text-sm font-semibold border-b-2 transition-all duration-150 ${
            activeTab === 'combos'
              ? 'border-primary text-primary'
              : 'border-transparent text-muted hover:text-text'
          }`}
        >
          <ListCollapse className="w-4 h-4" />
          Model Combos
        </button>
        <button
          onClick={() => { setActiveTab('mappings'); setSearch(''); setMappingPage(0); }}
          className={`flex items-center gap-2 px-4 py-2 text-sm font-semibold border-b-2 transition-all duration-150 ${
            activeTab === 'mappings'
              ? 'border-primary text-primary'
              : 'border-transparent text-muted hover:text-text'
          }`}
        >
          <Route className="w-4 h-4" />
          Model Mappings
        </button>
      </div>

      {/* Search Bar */}
      <div className="max-w-md">
        <Input
          placeholder={
            activeTab === 'discovery'
              ? 'Search providers or model IDs...'
              : activeTab === 'combos'
              ? 'Search combos...'
              : 'Search pattern or target model...'
          }
          value={search}
          onChange={(event) => {
            setSearch(event.target.value);
            setComboPage(0);
            setMappingPage(0);
          }}
          icon={<Search className="w-4 h-4 text-muted/50" />}
        />
      </div>

      {loadError && (
        <Card className="border-danger/30">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <p className="text-sm text-danger">{loadError}</p>
            <Button variant="secondary" size="sm" onClick={loadAll}>Retry</Button>
          </div>
        </Card>
      )}

      {/* TAB 1: Discovery & Providers */}
      {activeTab === 'discovery' && (
        <div className="space-y-6">
          {loading ? (
            <div className="grid gap-4 lg:grid-cols-2">
              {Array.from({ length: 4 }).map((_, index) => (
                <Card key={index}>
                  <Skeleton className="h-6 w-1/4 mb-3" />
                  <div className="flex flex-wrap gap-2">
                    {[1, 2, 3].map((i) => <Skeleton key={i} className="h-7 w-20" />)}
                  </div>
                </Card>
              ))}
            </div>
          ) : visibleGroups.length === 0 ? (
            <Card>
              <div className="flex flex-col items-center justify-center py-12 gap-2 text-muted">
                <Boxes className="w-8 h-8 text-muted/30" />
                <p className="text-sm">{query ? 'No models match your search' : 'No models discovered'}</p>
                <p className="text-xs text-muted/60">{query ? 'Try searching a different keyword.' : 'Add or enable a provider config to load models.'}</p>
              </div>
            </Card>
          ) : (
            <div className="grid gap-4 lg:grid-cols-2">
              {visibleGroups.map(([provider, models]) => (
                <Card key={provider} title={provider} description={`${models.length} effective model${models.length === 1 ? '' : 's'}`}>
                  <div className="flex flex-wrap gap-1.5 pt-2">
                    {models.map((model) => (
                      <Badge key={`${provider}-${model}`} variant="default" className="font-mono text-xs select-all">
                        {model}
                      </Badge>
                    ))}
                  </div>
                </Card>
              ))}
            </div>
          )}

          {canManage && (
            <Card title="Provider Routing Configurations" description="Base URL overrides used for model loading and forwarding">
              {loading ? (
                <div className="space-y-2">
                  {Array.from({ length: 3 }).map((_, index) => <Skeleton key={index} className="h-12 w-full" />)}
                </div>
              ) : configs.length === 0 ? (
                <div className="flex flex-col items-center justify-center py-8 gap-2 text-muted">
                  <Boxes className="w-8 h-8 text-muted/30" />
                  <p className="text-sm">No provider configurations</p>
                  <Button size="sm" variant="secondary" onClick={openCreateProvider}>Add the first provider</Button>
                </div>
              ) : (
                <Table>
                  <TableHead>
                    <TableRow>
                      <TableHeader>Provider</TableHeader>
                      <TableHeader>Base URL</TableHeader>
                      <TableHeader>Models Loaded</TableHeader>
                      <TableHeader>Status</TableHeader>
                      <TableHeader className="w-24 text-right">Actions</TableHeader>
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {configs.map((config) => (
                      <TableRow key={config.provider}>
                        <TableCell className="font-mono text-xs font-semibold text-text-secondary select-all">{config.provider}</TableCell>
                        <TableCell className="text-xs text-muted max-w-xs truncate select-all">{config.base_url || '—'}</TableCell>
                        <TableCell className="text-xs text-muted font-medium">
                          {Array.isArray(config.models) ? config.models.length : 0} models
                        </TableCell>
                        <TableCell>
                          <Badge variant={config.enabled === false ? 'default' : 'success'}>
                            {config.enabled === false ? 'Disabled' : 'Enabled'}
                          </Badge>
                        </TableCell>
                        <TableCell>
                          <div className="flex items-center justify-end gap-1">
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => openEditProvider(config)}
                              title={`Edit override for ${config.provider}`}
                            >
                              <Pencil className="w-3.5 h-3.5" />
                            </Button>
                            <Button
                              variant="ghost"
                              size="sm"
                              className="text-danger hover:text-danger hover:bg-danger/10"
                              onClick={() => removeProvider(config.provider)}
                              title={`Delete override for ${config.provider}`}
                            >
                              <Trash2 className="w-3.5 h-3.5" />
                            </Button>
                          </div>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              )}
            </Card>
          )}
        </div>
      )}

      {/* TAB 2: Model Combos */}
      {activeTab === 'combos' && (
        <Card>
          {loadingCombos && combos.length === 0 ? (
            <div className="space-y-3">
              {[1, 2, 3].map((i) => <Skeleton key={i} className="h-12 w-full" />)}
            </div>
          ) : filteredCombos.length === 0 ? (
            <div className="py-12 text-center">
              <p className="text-sm text-muted">
                {query ? 'No combos found matching search.' : 'No model combos (fallback chains) created yet.'}
              </p>
              {!query && canManage && (
                <button onClick={openCreateCombo} className="text-primary hover:underline font-semibold text-xs mt-1 block mx-auto">
                  Create a combo chain →
                </button>
              )}
            </div>
          ) : (
            <>
              <Table>
                <TableHead>
                  <TableRow>
                    <TableHeader>Name / Alias</TableHeader>
                    <TableHeader>Label</TableHeader>
                    <TableHeader>Fallback Chains</TableHeader>
                    <TableHeader>Status</TableHeader>
                    {canManage && <TableHeader className="w-24 text-right">Actions</TableHeader>}
                  </TableRow>
                </TableHead>
                <TableBody>
                  {comboPageData.map((combo) => {
                    let modelsArray = [];
                    try {
                      modelsArray = JSON.parse(combo.models_json);
                    } catch {
                      modelsArray = [];
                    }

                    return (
                      <TableRow key={combo.id}>
                        <TableCell className="font-semibold text-xs text-text-secondary select-all">{combo.name}</TableCell>
                        <TableCell className="text-xs font-medium text-muted">{combo.label || '—'}</TableCell>
                        <TableCell>
                          <div className="flex flex-wrap items-center gap-1">
                            {modelsArray.map((m, idx) => (
                              <span key={idx} className="inline-flex items-center text-xs font-mono">
                                <Badge variant="default" className="text-[10px] select-all">{m}</Badge>
                                {idx < modelsArray.length - 1 && (
                                  <ArrowRight className="w-3 h-3 text-muted mx-1 shrink-0" />
                                )}
                              </span>
                            ))}
                            {modelsArray.length === 0 && <span className="text-muted text-xs">—</span>}
                          </div>
                        </TableCell>
                        <TableCell>
                          {canManage ? (
                            <button onClick={() => toggleComboActive(combo)} className="focus:outline-none">
                              <Badge variant={combo.enabled ? 'success' : 'default'}>
                                {combo.enabled ? 'Enabled' : 'Disabled'}
                              </Badge>
                            </button>
                          ) : (
                            <Badge variant={combo.enabled ? 'success' : 'default'}>
                              {combo.enabled ? 'Enabled' : 'Disabled'}
                            </Badge>
                          )}
                        </TableCell>
                        {canManage && (
                          <TableCell>
                            <div className="flex items-center justify-end gap-1">
                              <Button variant="ghost" size="sm" onClick={() => openEditCombo(combo)}>
                                <Pencil className="w-3.5 h-3.5" />
                              </Button>
                              <Button variant="ghost" size="sm" className="text-danger hover:text-danger hover:bg-danger/10" onClick={() => removeCombo(combo.id)}>
                                <Trash2 className="w-3.5 h-3.5" />
                              </Button>
                            </div>
                          </TableCell>
                        )}
                      </TableRow>
                    );
                  })}
                </TableBody>
              </Table>

              <Pagination
                currentPage={comboPage}
                totalPages={totalComboPages}
                onPageChange={setComboPage}
              />
            </>
          )}
        </Card>
      )}

      {/* TAB 3: Model Mappings */}
      {activeTab === 'mappings' && (
        <Card>
          {loadingMappings && mappings.length === 0 ? (
            <div className="space-y-3">
              {[1, 2, 3].map((i) => <Skeleton key={i} className="h-12 w-full" />)}
            </div>
          ) : filteredMappings.length === 0 ? (
            <div className="py-12 text-center">
              <p className="text-sm text-muted">
                {query ? 'No model mappings match search.' : 'No model mappings (alias routing) created yet.'}
              </p>
              {!query && canManage && (
                <button onClick={openCreateMapping} className="text-primary hover:underline font-semibold text-xs mt-1 block mx-auto">
                  Create a model mapping rule →
                </button>
              )}
            </div>
          ) : (
            <>
              <Table>
                <TableHead>
                  <TableRow>
                    <TableHeader>Priority</TableHeader>
                    <TableHeader>Incoming Request Pattern</TableHeader>
                    <TableHeader>Match Mode</TableHeader>
                    <TableHeader>Routes / Rewrites To</TableHeader>
                    <TableHeader>Label / Notes</TableHeader>
                    <TableHeader>Status</TableHeader>
                    {canManage && <TableHeader className="w-24 text-right">Actions</TableHeader>}
                  </TableRow>
                </TableHead>
                <TableBody>
                  {mappingPageData.map((m) => (
                    <TableRow key={m.id}>
                      <TableCell className="tabular-nums text-xs text-muted font-bold">{m.priority}</TableCell>
                      <TableCell className="font-mono text-xs text-text-secondary select-all font-semibold">
                        {m.source_pattern}
                      </TableCell>
                      <TableCell>
                        <Badge variant="default" className="text-[10px] capitalize font-sans">{m.match_type || 'contains'}</Badge>
                      </TableCell>
                      <TableCell className="font-mono text-xs text-primary font-semibold select-all">
                        {m.target_model}
                      </TableCell>
                      <TableCell className="text-xs text-muted">{m.label || '—'}</TableCell>
                      <TableCell>
                        {canManage ? (
                          <button onClick={() => toggleMappingActive(m)} className="focus:outline-none">
                            <Badge variant={m.enabled ? 'success' : 'default'}>
                              {m.enabled ? 'Active' : 'Inactive'}
                            </Badge>
                          </button>
                        ) : (
                          <Badge variant={m.enabled ? 'success' : 'default'}>
                            {m.enabled ? 'Active' : 'Inactive'}
                          </Badge>
                        )}
                      </TableCell>
                      {canManage && (
                        <TableCell>
                          <div className="flex items-center justify-end gap-1">
                            <Button variant="ghost" size="sm" onClick={() => openEditMapping(m)}>
                              <Pencil className="w-3.5 h-3.5" />
                            </Button>
                            <Button variant="ghost" size="sm" className="text-danger hover:text-danger hover:bg-danger/10" onClick={() => removeMapping(m.id)}>
                              <Trash2 className="w-3.5 h-3.5" />
                            </Button>
                          </div>
                        </TableCell>
                      )}
                    </TableRow>
                  ))}
                </TableBody>
              </Table>

              <Pagination
                currentPage={mappingPage}
                totalPages={totalMappingPages}
                onPageChange={setMappingPage}
              />
            </>
          )}
        </Card>
      )}

      {/* MODAL 1: Provider Config Dialog */}
      <Modal open={providerModalOpen} onClose={() => setProviderModalOpen(false)} title={editingProvider ? 'Edit Provider Routing' : 'Add Provider Routing'}>
        <div className="space-y-4">
          <Input
            label="Provider ID"
            value={providerForm.provider}
            disabled={Boolean(editingProvider)}
            onChange={(event) => setProviderForm({ ...providerForm, provider: event.target.value.toLowerCase().trim() })}
            placeholder="openai"
            required
          />
          <Input
            label="Base URL"
            value={providerForm.base_url}
            onChange={(event) => setProviderForm({ ...providerForm, base_url: event.target.value.trim() })}
            placeholder="https://api.openai.com/v1"
            required
          />
          <div>
            <label htmlFor="model-ids" className="block text-sm font-medium text-text mb-1.5 font-sans">Discovered Models List</label>
            <textarea
              id="model-ids"
              rows={6}
              value={providerForm.model_ids}
              onChange={(event) => setProviderForm({ ...providerForm, model_ids: event.target.value })}
              placeholder={'gpt-4o\ngpt-4o-mini'}
              className="w-full rounded-lg border border-border bg-surface text-text px-3 py-2 text-sm font-mono placeholder:text-muted/60 focus:outline-none focus:ring-2 focus:ring-primary/50 focus:border-primary/50 transition-all duration-150"
            />
            <p className="mt-1 text-xs text-muted font-sans">Enter one model ID per line, or separate with commas.</p>
          </div>
          <label className="flex items-center gap-2 text-sm text-text cursor-pointer select-none">
            <input
              type="checkbox"
              checked={providerForm.enabled}
              onChange={(event) => setProviderForm({ ...providerForm, enabled: event.target.checked })}
              className="rounded border-border bg-surface text-primary focus:ring-primary/50"
            />
            Enable route forwarding for this provider
          </label>
          <div className="flex gap-2 justify-end pt-2">
            <Button variant="ghost" onClick={() => setProviderModalOpen(false)}>Cancel</Button>
            <Button onClick={saveProvider} loading={savingProvider} disabled={!providerForm.provider.trim() || !providerForm.base_url.trim()}>
              {editingProvider ? 'Save Changes' : 'Add Provider'}
            </Button>
          </div>
        </div>
      </Modal>

      {/* MODAL 2: Combo Config Dialog */}
      <Modal open={comboModalOpen} onClose={() => setComboModalOpen(false)} title={editingCombo ? 'Edit Model Combo' : 'New Model Combo'}>
        <div className="space-y-4">
          <Input
            label="Combo Name / Target Identifier"
            value={comboForm.name}
            onChange={(event) => setComboForm({ ...comboForm, name: event.target.value.toLowerCase().trim() })}
            placeholder="gpt-4o-failover"
            required
          />
          <Input
            label="Friendly Label"
            value={comboForm.label}
            onChange={(event) => setComboForm({ ...comboForm, label: event.target.value })}
            placeholder="GPT-4o with Sonnet Fallback"
          />
          <div>
            <label htmlFor="combo-models" className="block text-sm font-medium text-text mb-1.5 font-sans">Fallback Models Ordered List</label>
            <textarea
              id="combo-models"
              rows={5}
              value={comboForm.models_list}
              onChange={(event) => setComboForm({ ...comboForm, models_list: event.target.value })}
              placeholder={'openai/gpt-4o\nanthropic/claude-3-5-sonnet'}
              className="w-full rounded-lg border border-border bg-surface text-text px-3 py-2 text-sm font-mono placeholder:text-muted/60 focus:outline-none focus:ring-2 focus:ring-primary/50 focus:border-primary/50 transition-all duration-150"
            />
            <p className="mt-1 text-xs text-muted font-sans">Enter one model/combo name per line. If the first fails, it falls back to the next.</p>
          </div>
          <label className="flex items-center gap-2 text-sm text-text cursor-pointer select-none">
            <input
              type="checkbox"
              checked={comboForm.enabled}
              onChange={(event) => setComboForm({ ...comboForm, enabled: event.target.checked })}
              className="rounded border-border bg-surface text-primary focus:ring-primary/50"
            />
            Combo Enabled
          </label>
          <div className="flex gap-2 justify-end pt-2">
            <Button variant="ghost" onClick={() => setComboModalOpen(false)}>Cancel</Button>
            <Button onClick={saveCombo} loading={savingCombo} disabled={!comboForm.name.trim()}>
              {editingCombo ? 'Save Changes' : 'Create Combo'}
            </Button>
          </div>
        </div>
      </Modal>

      {/* MODAL 3: Mapping Config Dialog */}
      <Modal open={mappingModalOpen} onClose={() => setMappingModalOpen(false)} title={editingMapping ? 'Edit Model Mapping' : 'New Model Mapping'}>
        <div className="space-y-4">
          <Input
            label="Incoming Model Query Pattern"
            value={mappingForm.source_pattern}
            onChange={(event) => setMappingForm({ ...mappingForm, source_pattern: event.target.value.trim() })}
            placeholder="gpt-4"
            required
          />
          <div>
            <label className="block text-sm font-medium text-text mb-1 font-sans">Match Mode</label>
            <Select
              value={mappingForm.match_type}
              onChange={(event) => setMappingForm({ ...mappingForm, match_type: event.target.value })}
            >
              <option value="contains">Contains Pattern</option>
              <option value="exact">Exact Match</option>
              <option value="regex">Regex Match</option>
            </Select>
          </div>
          <Input
            label="Forward to Target Model"
            value={mappingForm.target_model}
            onChange={(event) => setMappingForm({ ...mappingForm, target_model: event.target.value.trim() })}
            placeholder="claude-3-5-sonnet"
            required
          />
          <Input
            label="Priority (Lower Runs First)"
            type="number"
            value={mappingForm.priority}
            onChange={(event) => setMappingForm({ ...mappingForm, priority: Number(event.target.value) })}
          />
          <Input
            label="Friendly Label / Note"
            value={mappingForm.label}
            onChange={(event) => setMappingForm({ ...mappingForm, label: event.target.value })}
            placeholder="Map generic GPT-4 queries to Anthropic"
          />
          <label className="flex items-center gap-2 text-sm text-text cursor-pointer select-none">
            <input
              type="checkbox"
              checked={mappingForm.enabled}
              onChange={(event) => setMappingForm({ ...mappingForm, enabled: event.target.checked })}
              className="rounded border-border bg-surface text-primary focus:ring-primary/50"
            />
            Mapping Active
          </label>
          <div className="flex gap-2 justify-end pt-2">
            <Button variant="ghost" onClick={() => setMappingModalOpen(false)}>Cancel</Button>
            <Button onClick={saveMapping} loading={savingMapping} disabled={!mappingForm.source_pattern.trim() || !mappingForm.target_model.trim()}>
              {editingMapping ? 'Save Changes' : 'Create Mapping'}
            </Button>
          </div>
        </div>
      </Modal>
    </div>
  );
}
