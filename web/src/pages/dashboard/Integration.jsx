import { useEffect, useState } from 'react';
import { client } from '../../api/client';
import { useToast } from '../../components/ui/Toast';
import Card from '../../components/ui/Card';
import Button from '../../components/ui/Button';
import Input from '../../components/ui/Input';
import Modal from '../../components/ui/Modal';
import Skeleton from '../../components/ui/Skeleton';
import Select from '../../components/ui/Select';
import { Badge } from '../../components/ui/Badge';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '../../components/ui/Table';
import {
  Plus,
  Trash2,
  RefreshCw,
  Check,
  X,
  Search,
  Grid,
  Settings,
  Layers,
  Terminal,
  Image as ImageIcon,
  Cpu,
  Volume2,
  Database,
  Sliders,
  Users,
  Boxes,
  Play,
  Flame,
  ShieldAlert,
} from 'lucide-react';

const CATEGORIES = [
  { id: 'all', name: 'All Services', icon: Grid },
  { id: 'llm', name: 'Language Models', icon: Layers },
  { id: 'infra', name: 'Hardware & Routers', icon: Cpu },
  { id: 'local', name: 'Local & Edge', icon: Terminal },
  { id: 'image', name: 'Creative & Video', icon: ImageIcon },
  { id: 'code', name: 'Developer Tools', icon: Sliders },
  { id: 'audio', name: 'Audio & Speech', icon: Volume2 },
  { id: 'db', name: 'Vector Databases', icon: Database },
];

const PROVIDER_CATEGORIES = {
  // LLMs
  openai: 'llm',
  'azure-openai': 'infra',
  anthropic: 'llm',
  cohere: 'llm',
  mistral: 'llm',
  groq: 'llm',
  deepseek: 'llm',
  perplexity: 'llm',
  xai: 'llm',
  google: 'llm',
  'google-vertex': 'llm',
  openrouter: 'llm',
  byok: 'llm',

  // LLM Hardware & Routers
  together: 'infra',
  fireworks: 'infra',
  cerebras: 'infra',
  sambanova: 'infra',
  novita: 'infra',
  lepton: 'infra',
  hyperbolic: 'infra',
  bedrock: 'infra',
  'aws-bedrock': 'infra',
  sagemaker: 'infra',
  'azure-ml': 'infra',
  cloudflare: 'infra',
  replicate: 'infra',
  huggingface: 'infra',
  anyscale: 'infra',
  octoai: 'infra',

  // Local
  ollama: 'local',
  lmstudio: 'local',
  localai: 'local',
  vllm: 'local',
  textgenwebui: 'local',
  koboldcpp: 'local',
  llamacpp: 'local',
  jan: 'local',
  oobabooga: 'local',

  // Image & Video
  stabilityai: 'image',
  fal: 'image',
  canva: 'image',
  ideogram: 'image',
  midjourney: 'image',
  leonardo: 'image',
  clipdrop: 'image',
  getimg: 'image',
  segmind: 'image',
  runway: 'image',
  sora: 'image',
  pika: 'image',
  kling: 'image',
  luma: 'image',

  // Code
  kiro: 'code',
  'kiro-pro': 'code',
  codebuddy: 'code',
  codex: 'code',
  qoder: 'code',
  mimo: 'code',
  'github-copilot': 'code',
  tabnine: 'code',
  codeium: 'code',
  sourcegraph: 'code',
  cursor: 'code',
  continue: 'code',

  // Audio
  elevenlabs: 'audio',
  'openai-tts': 'audio',
  playht: 'audio',
  assemblyai: 'audio',
  deepgram: 'audio',
  whisper: 'audio',

  // DB
  pinecone: 'db',
  weaviate: 'db',
  qdrant: 'db',
  milvus: 'db',
  chroma: 'db',
};

export default function Integration() {
  const [configs, setConfigs] = useState([]);
  const [defaults, setDefaults] = useState({});
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState('');
  const [activeCategory, setActiveCategory] = useState('all');
  const [statusFilter, setStatusFilter] = useState('all'); // all, active, available
  const [seeding, setSeeding] = useState(false);
  const toast = useToast();

  // Multi-tab Provider Details Panel States
  const [detailModalOpen, setDetailModalOpen] = useState(false);
  const [activeTab, setActiveTab] = useState('settings'); // settings, accounts, models, tester
  const [selectedProvider, setSelectedProvider] = useState(null);
  
  // Tab 1: Settings form
  const [settingsForm, setSettingsForm] = useState({ base_url: '', enabled: true, jb_enabled: false, jb_prompt: '' });
  const [savingSettings, setSavingSettings] = useState(false);

  // Tab 2: Accounts list
  const [accounts, setAccounts] = useState([]);
  const [loadingAccounts, setLoadingAccounts] = useState(false);
  const [accountForm, setAccountForm] = useState({ email: '', password: '', tokens: '', quota_limit: '' });
  const [savingAccount, setSavingAccount] = useState(false);

  // Tab 3: Models list
  const [modelsInput, setModelsInput] = useState('');
  const [savingModels, setSavingModels] = useState(false);

  // Tab 4: Tester Playground
  const [testerModel, setTesterModel] = useState('');
  const [testerPrompt, setTesterPrompt] = useState('Hello! Just a connectivity test request.');
  const [testing, setTesting] = useState(false);
  const [testResponse, setTestResponse] = useState('');
  const [userKeys, setUserKeys] = useState([]);
  const [selectedKey, setSelectedKey] = useState('');

  useEffect(() => { load(); }, []);

  async function load() {
    try {
      const [cfgRes, defRes] = await Promise.all([
        client.get('/api/provider-config').catch(() => ({ data: [] })),
        client.get('/api/provider-config/defaults').catch(() => ({ data: {} })),
      ]);
      setConfigs(cfgRes.data || []);
      setDefaults(defRes.data || {});
    } catch {
      toast.error('Failed to load provider configuration');
    } finally {
      setLoading(false);
    }
  }

  async function seedDefaults() {
    setSeeding(true);
    try {
      const res = await client.post('/api/provider-config/seed');
      toast(`Successfully seeded ${res.data?.seeded || 0} defaults`);
      load();
    } catch {
      toast.error('Seeding default providers failed');
    } finally {
      setSeeding(false);
    }
  }

  async function toggleEnabled(cfg) {
    try {
      await client.post('/api/provider-config', { ...cfg, enabled: !cfg.enabled });
      toast(cfg.enabled ? 'Provider disabled' : 'Provider enabled');
      load();
    } catch {
      toast.error('Toggle failed');
    }
  }

  async function remove(provider) {
    if (!window.confirm(`Delete configuration for ${provider}?`)) return;
    try {
      await client.delete(`/api/provider-config/${provider}`);
      toast('Provider configuration deleted');
      load();
    } catch {
      toast.error('Delete failed');
    }
  }

  // Open multi-tab Details Modal
  async function openDetails(provider, defaultUrl = '') {
    setSelectedProvider(provider);
    setActiveTab('settings');
    setTestResponse('');
    setTesterModel('');
    
    // Reset forms
    const existing = configs.find((c) => c.provider === provider);
    const extra = existing?.extra || {};
    
    setSettingsForm({
      base_url: existing ? existing.base_url : defaultUrl,
      enabled: existing ? existing.enabled : true,
      jb_enabled: extra.jailbreak_enabled === true,
      jb_prompt: extra.jailbreak_prompt || '',
    });
    
    setModelsInput(existing && Array.isArray(existing.models) ? existing.models.join('\n') : '');
    
    // Set loading and open
    setDetailModalOpen(true);
    
    // Fetch accounts and keys in background
    fetchProviderAccounts(provider);
    fetchUserKeys();
  }

  async function fetchProviderAccounts(provider) {
    setLoadingAccounts(true);
    try {
      const res = await client.get('/api/accounts', { params: { provider } });
      const list = Array.isArray(res.data) ? res.data : res.data?.accounts || [];
      setAccounts(list);
    } catch {
      setAccounts([]);
    } finally {
      setLoadingAccounts(false);
    }
  }

  async function fetchUserKeys() {
    try {
      const storedTenant = localStorage.getItem('sb_active_tenant_id');
      if (storedTenant) {
        const res = await client.get('/api/keys/v2', { params: { tenant_id: storedTenant } });
        const list = res.data?.keys || res.data || [];
        setUserKeys(list);
        if (list.length > 0) setSelectedKey(list[0].name);
      }
    } catch {
      setUserKeys([]);
    }
  }

  // Save Settings Form
  async function handleSaveSettings() {
    setSavingSettings(true);
    try {
      const existing = configs.find((c) => c.provider === selectedProvider) || {};
      const payload = {
        provider: selectedProvider,
        base_url: settingsForm.base_url.trim(),
        enabled: settingsForm.enabled,
        models: existing.models || [],
        extra: {
          jailbreak_enabled: settingsForm.jb_enabled,
          jailbreak_prompt: settingsForm.jb_prompt,
        },
      };
      
      await client.post('/api/provider-config', payload);
      toast('Provider settings saved');
      load();
    } catch {
      toast.error('Failed to save settings');
    } finally {
      setSavingSettings(false);
    }
  }

  // Create Multi-Account credential
  async function handleAddAccount() {
    if (!accountForm.email) return;
    setSavingAccount(true);
    try {
      await client.post('/api/accounts', {
        provider: selectedProvider,
        email: accountForm.email.trim(),
        password: accountForm.password,
        tokens: accountForm.tokens,
        quota_limit: Number(accountForm.quota_limit) || 0,
      });
      toast('Credential added to provider accounts list');
      setAccountForm({ email: '', password: '', tokens: '', quota_limit: '' });
      fetchProviderAccounts(selectedProvider);
    } catch {
      toast.error('Failed to add account credentials');
    } finally {
      setSavingAccount(false);
    }
  }

  async function handleDeleteAccount(id) {
    if (!window.confirm('Delete this account credentials?')) return;
    try {
      await client.delete(`/api/accounts/${id}`);
      toast('Account credentials deleted');
      fetchProviderAccounts(selectedProvider);
    } catch {
      toast.error('Failed to delete account');
    }
  }

  async function toggleAccountEnabled(acc) {
    try {
      await client.put(`/api/accounts/${acc.id}`, { enabled: !acc.enabled });
      toast(acc.enabled ? 'Account disabled' : 'Account enabled');
      fetchProviderAccounts(selectedProvider);
    } catch {
      toast.error('Failed to toggle account');
    }
  }

  // Save custom models
  async function handleSaveModels() {
    setSavingModels(true);
    try {
      const modelList = [...new Set(modelsInput.split(/[\n,]+/).map((id) => id.trim()).filter(Boolean))];
      const existing = configs.find((c) => c.provider === selectedProvider) || {};
      
      const payload = {
        provider: selectedProvider,
        base_url: settingsForm.base_url.trim(),
        enabled: settingsForm.enabled,
        models: modelList,
        extra: existing.extra || {},
      };
      
      await client.post('/api/provider-config', payload);
      toast('Custom models override saved');
      load();
    } catch {
      toast.error('Failed to save custom models');
    } finally {
      setSavingModels(false);
    }
  }

  // Test playground chat completions request
  async function handleRunTest() {
    const activeModel = testerModel || (configs.find(c => c.provider === selectedProvider)?.models?.[0]) || '';
    if (!activeModel) {
      toast.error('No active models to test. Add or configure models first.');
      return;
    }
    
    setTesting(true);
    setTestResponse('Connecting to Switchblade gateway /v1/chat/completions...\n');
    try {
      const headers = {};
      if (selectedKey) headers['Authorization'] = `Bearer ${selectedKey}`;
      
      // Request directly to gateway (port 2005) or fallback to internal test if needed
      const port = window.location.port === '3931' ? '2005' : window.location.port;
      const gatewayEndpoint = `${window.location.protocol}//${window.location.hostname}:${port}/v1/chat/completions`;
      
      const res = await fetch(gatewayEndpoint, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          ...headers
        },
        body: JSON.stringify({
          model: activeModel,
          messages: [{ role: 'user', content: testerPrompt }]
        })
      });
      
      const data = await res.json();
      if (res.ok) {
        setTestResponse(JSON.stringify(data, null, 2));
      } else {
        setTestResponse(`HTTP Error ${res.status}: ${JSON.stringify(data, null, 2)}`);
      }
    } catch (err) {
      setTestResponse(`Network Error: ${err.message}\nMake sure Switchblade proxy gateway is running on port 2005.`);
    } finally {
      setTesting(false);
    }
  }

  // Combine default providers & current configs to build marketplace catalog
  const catalog = Object.entries(defaults).map(([provider, defaultUrl]) => {
    const config = configs.find((c) => c.provider === provider);
    const category = PROVIDER_CATEGORIES[provider] || 'other';
    return {
      provider,
      defaultUrl,
      config,
      category,
      configured: !!config,
      enabled: config ? config.enabled : false,
      modelsCount: config && Array.isArray(config.models) ? config.models.length : 0,
    };
  });

  // Include custom configurations not present in the default catalog
  configs.forEach((cfg) => {
    if (!defaults[cfg.provider]) {
      const category = PROVIDER_CATEGORIES[cfg.provider] || 'other';
      catalog.push({
        provider: cfg.provider,
        defaultUrl: '',
        config: cfg,
        category,
        configured: true,
        enabled: cfg.enabled,
        modelsCount: Array.isArray(cfg.models) ? cfg.models.length : 0,
      });
    }
  });

  // Filter Catalog
  const filteredCatalog = catalog.filter((item) => {
    const q = search.trim().toLowerCase();
    if (q && !item.provider.toLowerCase().includes(q)) {
      return false;
    }
    if (activeCategory !== 'all' && item.category !== activeCategory) {
      return false;
    }
    if (statusFilter === 'active' && (!item.configured || !item.enabled)) {
      return false;
    }
    if (statusFilter === 'configured' && !item.configured) {
      return false;
    }
    if (statusFilter === 'available' && item.configured) {
      return false;
    }
    return true;
  });

  // Sort: Configured & Enabled first, then alphabetical
  filteredCatalog.sort((a, b) => {
    if (a.configured !== b.configured) {
      return a.configured ? -1 : 1;
    }
    if (a.enabled !== b.enabled) {
      return a.enabled ? -1 : 1;
    }
    return a.provider.localeCompare(b.provider);
  });

  const activeConfig = configs.find(c => c.provider === selectedProvider);
  const providerModels = activeConfig?.models || [];

  return (
    <div className="space-y-6">
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold text-text">Integration Marketplace</h1>
          <p className="text-sm text-muted mt-1">Connect, route, and configure API providers dynamically</p>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="secondary" size="sm" onClick={seedDefaults} loading={seeding}>
            <RefreshCw className="w-3.5 h-3.5 mr-1.5" /> Quick-Seed Defaults
          </Button>
          <Button size="sm" onClick={() => openDetails('')} variant="primary">
            <Plus className="w-3.5 h-3.5 mr-1.5" /> Custom Integration
          </Button>
        </div>
      </div>

      {/* Categories Horizontal Scroller */}
      <div className="flex items-center gap-2 overflow-x-auto pb-2 scrollbar-none border-b border-border">
        {CATEGORIES.map((cat) => {
          const Icon = cat.icon;
          const isActive = activeCategory === cat.id;
          return (
            <button
              key={cat.id}
              onClick={() => setActiveCategory(cat.id)}
              className={`flex items-center gap-2 px-3 py-1.5 rounded-lg text-xs font-semibold whitespace-nowrap transition-all duration-150 ${
                isActive
                  ? 'bg-primary text-primary-foreground shadow-sm shadow-primary/20'
                  : 'bg-surface hover:bg-surface-hover text-muted hover:text-text border border-border/40'
              }`}
            >
              <Icon className="w-3.5 h-3.5" />
              {cat.name}
            </button>
          );
        })}
      </div>

      {/* Filter and Search Bar */}
      <div className="flex flex-col md:flex-row md:items-center justify-between gap-4">
        <div className="max-w-md w-full">
          <Input
            placeholder="Search 100+ default providers..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            icon={<Search className="w-4 h-4 text-muted/50" />}
          />
        </div>

        <div className="flex items-center gap-1.5 bg-surface border border-border/60 p-0.5 rounded-lg text-xs font-medium self-end md:self-auto shrink-0">
          <button onClick={() => setStatusFilter('all')} className={`px-3 py-1.5 rounded-md transition-all duration-150 ${statusFilter === 'all' ? 'bg-surface-hover text-text font-semibold shadow-sm' : 'text-muted hover:text-text'}`}>All</button>
          <button onClick={() => setStatusFilter('configured')} className={`px-3 py-1.5 rounded-md transition-all duration-150 ${statusFilter === 'configured' ? 'bg-surface-hover text-text font-semibold shadow-sm' : 'text-muted hover:text-text'}`}>Configured</button>
          <button onClick={() => setStatusFilter('active')} className={`px-3 py-1.5 rounded-md transition-all duration-150 ${statusFilter === 'active' ? 'bg-surface-hover text-text font-semibold shadow-sm' : 'text-muted hover:text-text'}`}>Active</button>
          <button onClick={() => setStatusFilter('available')} className={`px-3 py-1.5 rounded-md transition-all duration-150 ${statusFilter === 'available' ? 'bg-surface-hover text-text font-semibold shadow-sm' : 'text-muted hover:text-text'}`}>Unconfigured</button>
        </div>
      </div>

      {/* Marketplace Catalog Grid */}
      {loading ? (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
          {Array.from({ length: 12 }).map((_, i) => (
            <Card key={i} className="h-44 flex flex-col justify-between">
              <div className="space-y-2">
                <Skeleton className="h-5 w-1/3" />
                <Skeleton className="h-4 w-2/3" />
              </div>
              <Skeleton className="h-8 w-full" />
            </Card>
          ))}
        </div>
      ) : filteredCatalog.length === 0 ? (
        <Card className="py-16 text-center">
          <Grid className="w-12 h-12 text-muted/20 mx-auto mb-3" />
          <p className="text-sm text-text-secondary font-medium">No providers match your search filters.</p>
          <button onClick={() => { setSearch(''); setActiveCategory('all'); setStatusFilter('all'); }} className="text-xs text-primary font-semibold mt-1 hover:underline">
            Reset filter presets
          </button>
        </Card>
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
          {filteredCatalog.map((item) => {
            const hasConfig = item.configured;
            const isEnabled = item.enabled;

            return (
              <Card
                key={item.provider}
                onClick={() => openDetails(item.provider, item.defaultUrl)}
                className={`relative flex flex-col justify-between h-44 p-5 transition-all duration-200 border cursor-pointer group ${
                  hasConfig
                    ? isEnabled
                      ? 'border-border bg-surface hover:shadow-lg hover:shadow-primary/5 hover:border-primary/40'
                      : 'border-border/60 bg-surface/50 opacity-80'
                    : 'border-dashed border-border/60 bg-transparent hover:bg-surface/20'
                }`}
              >
                <div>
                  <div className="flex items-start justify-between">
                    <h3 className="text-base font-bold text-text capitalize select-all">
                      {item.provider.replace('-', ' ')}
                    </h3>
                    <div className="flex items-center gap-1.5" onClick={(e) => e.stopPropagation()}>
                      {hasConfig ? (
                        <button
                          onClick={() => toggleEnabled(item.config)}
                          className="focus:outline-none"
                          title={isEnabled ? 'Disable Provider' : 'Enable Provider'}
                        >
                          <Badge variant={isEnabled ? 'success' : 'default'} className="cursor-pointer">
                            {isEnabled ? 'Active' : 'Disabled'}
                          </Badge>
                        </button>
                      ) : (
                        <Badge variant="default" className="bg-muted/10 text-muted border-none text-[10px]">
                          Available
                        </Badge>
                      )}
                    </div>
                  </div>

                  <span className="text-[10px] text-muted/60 font-semibold uppercase tracking-wider block mt-0.5">
                    {item.category}
                  </span>

                  <p className="text-xs text-muted mt-3 font-mono truncate select-all" title={item.config?.base_url || item.defaultUrl}>
                    {item.config?.base_url || item.defaultUrl || 'No default URL'}
                  </p>
                </div>

                <div className="flex items-center justify-between mt-4 pt-3 border-t border-border/40" onClick={(e) => e.stopPropagation()}>
                  <span className="text-xs text-muted font-semibold">
                    {hasConfig ? `${item.modelsCount} custom models` : 'Ready to connect'}
                  </span>

                  <div className="flex items-center gap-1.5">
                    {hasConfig && (
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => remove(item.provider)}
                        className="text-danger hover:text-danger hover:bg-danger/10 p-1.5 h-8 w-8"
                        title="Delete integration settings"
                      >
                        <Trash2 className="w-4 h-4" />
                      </Button>
                    )}
                    <Button
                      size="sm"
                      variant={hasConfig ? 'secondary' : 'primary'}
                      onClick={() => openDetails(item.provider, item.defaultUrl)}
                      className="h-8 text-xs px-3 font-semibold"
                    >
                      {hasConfig ? 'Manage' : 'Configure'}
                    </Button>
                  </div>
                </div>
              </Card>
            );
          })}
        </div>
      )}

      {/* MULTI-TAB PROVIDER DETAILS MODAL */}
      <Modal
        open={detailModalOpen}
        onClose={() => setDetailModalOpen(false)}
        title={`Manage ${selectedProvider ? selectedProvider.toUpperCase().replace('-', ' ') : 'Custom Integration'}`}
        width="max-w-4xl"
      >
        <div className="space-y-6">
          {/* Tabs header */}
          <div className="flex items-center gap-1 border-b border-border/60 pb-px">
            <button
              onClick={() => setActiveTab('settings')}
              className={`flex items-center gap-1.5 px-4 py-2.5 text-xs font-bold border-b-2 transition-all duration-150 ${
                activeTab === 'settings' ? 'border-primary text-primary' : 'border-transparent text-muted hover:text-text'
              }`}
            >
              <Settings className="w-3.5 h-3.5" />
              Settings
            </button>
            <button
              onClick={() => setActiveTab('accounts')}
              className={`flex items-center gap-1.5 px-4 py-2.5 text-xs font-bold border-b-2 transition-all duration-150 ${
                activeTab === 'accounts' ? 'border-primary text-primary' : 'border-transparent text-muted hover:text-text'
              }`}
            >
              <Users className="w-3.5 h-3.5" />
              Accounts ({accounts.length})
            </button>
            <button
              onClick={() => setActiveTab('models')}
              className={`flex items-center gap-1.5 px-4 py-2.5 text-xs font-bold border-b-2 transition-all duration-150 ${
                activeTab === 'models' ? 'border-primary text-primary' : 'border-transparent text-muted hover:text-text'
              }`}
            >
              <Boxes className="w-3.5 h-3.5" />
              Models Overrides
            </button>
            <button
              onClick={() => setActiveTab('tester')}
              className={`flex items-center gap-1.5 px-4 py-2.5 text-xs font-bold border-b-2 transition-all duration-150 ${
                activeTab === 'tester' ? 'border-primary text-primary' : 'border-transparent text-muted hover:text-text'
              }`}
            >
              <Play className="w-3.5 h-3.5" />
              Playground / Test
            </button>
          </div>

          {/* TAB 1: Settings Form */}
          {activeTab === 'settings' && (
            <div className="space-y-4">
              <Input
                label="Provider Identifier"
                value={selectedProvider || ''}
                disabled
                placeholder="openai"
              />
              <Input
                label="API Base URL"
                value={settingsForm.base_url}
                onChange={(e) => setSettingsForm({ ...settingsForm, base_url: e.target.value })}
                placeholder="https://api.provider.com/v1"
                required
              />
              <label className="flex items-center gap-2 text-sm text-text cursor-pointer select-none py-1">
                <input
                  type="checkbox"
                  checked={settingsForm.enabled}
                  onChange={(e) => setSettingsForm({ ...settingsForm, enabled: e.target.checked })}
                  className="rounded border-border bg-surface text-primary focus:ring-primary/50"
                />
                Enable route routing for this provider
              </label>

              {/* Provider-specific Jailbreak Override */}
              <div className="p-4 rounded-xl border border-border bg-surface/30 space-y-3 mt-4">
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <ShieldAlert className="w-4 h-4 text-primary" />
                    <span className="text-xs font-bold text-text-secondary uppercase tracking-wider">
                      Provider-Specific Jailbreak Override
                    </span>
                  </div>
                  <label className="flex items-center gap-2 text-xs text-text cursor-pointer select-none">
                    <input
                      type="checkbox"
                      checked={settingsForm.jb_enabled}
                      onChange={(e) => setSettingsForm({ ...settingsForm, jb_enabled: e.target.checked })}
                      className="rounded border-border bg-surface text-primary focus:ring-primary/50"
                    />
                    Enable override
                  </label>
                </div>
                {settingsForm.jb_enabled && (
                  <div className="space-y-1.5 animate-fade-in">
                    <label htmlFor="provider-jb" className="text-xs font-semibold text-muted block">Jailbreak Prompt</label>
                    <textarea
                      id="provider-jb"
                      rows={4}
                      value={settingsForm.jb_prompt}
                      onChange={(e) => setSettingsForm({ ...settingsForm, jb_prompt: e.target.value })}
                      placeholder="Enter provider-specific jailbreak prompt instructions..."
                      className="w-full rounded-lg border border-border bg-surface text-text px-3 py-2 text-sm font-mono placeholder:text-muted/40 focus:outline-none focus:ring-2 focus:ring-primary/50 focus:border-primary/50 transition-all duration-150"
                    />
                  </div>
                )}
              </div>

              <div className="flex gap-2 justify-end pt-3">
                <Button variant="ghost" onClick={() => setDetailModalOpen(false)}>Cancel</Button>
                <Button onClick={handleSaveSettings} loading={savingSettings} disabled={!settingsForm.base_url.trim()} variant="primary">
                  Save Settings
                </Button>
              </div>
            </div>
          )}

          {/* TAB 2: Multi-Accounts Credential Manager */}
          {activeTab === 'accounts' && (
            <div className="space-y-5">
              {/* Add Account Inline Form */}
              <div className="p-4 rounded-xl border border-border/80 bg-surface-elevated/40 space-y-3">
                <h4 className="text-xs font-bold text-text uppercase tracking-wider">Add Provider Account Credential</h4>
                <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
                  <Input
                    placeholder="Email / Identifier"
                    value={accountForm.email}
                    onChange={(e) => setAccountForm({ ...accountForm, email: e.target.value })}
                    className="h-9 py-1 text-xs"
                  />
                  <Input
                    placeholder="Password (Encrypted in DB)"
                    type="password"
                    value={accountForm.password}
                    onChange={(e) => setAccountForm({ ...accountForm, password: e.target.value })}
                    className="h-9 py-1 text-xs"
                  />
                  <Input
                    placeholder="API Keys / Auth Tokens"
                    value={accountForm.tokens}
                    onChange={(e) => setAccountForm({ ...accountForm, tokens: e.target.value })}
                    className="h-9 py-1 text-xs"
                  />
                </div>
                <div className="flex items-center justify-between gap-4 pt-1">
                  <div className="max-w-[180px] w-full">
                    <Input
                      placeholder="Quota Limit"
                      type="number"
                      value={accountForm.quota_limit}
                      onChange={(e) => setAccountForm({ ...accountForm, quota_limit: e.target.value })}
                      className="h-9 py-1 text-xs"
                    />
                  </div>
                  <Button
                    size="sm"
                    onClick={handleAddAccount}
                    disabled={!accountForm.email}
                    loading={savingAccount}
                    className="text-xs px-4"
                  >
                    Add Credential
                  </Button>
                </div>
              </div>

              {/* Accounts List Table */}
              <div>
                <h4 className="text-xs font-bold text-text uppercase tracking-wider mb-2">
                  Configured Provider Accounts ({accounts.length})
                </h4>
                {loadingAccounts ? (
                  <Skeleton className="h-32 w-full" />
                ) : accounts.length === 0 ? (
                  <div className="py-8 text-center text-muted text-xs border border-dashed border-border/60 rounded-xl">
                    No account credentials configured. Add a credential above to start load balancing.
                  </div>
                ) : (
                  <div className="max-h-60 overflow-y-auto border border-border/40 rounded-xl bg-surface/20">
                    <Table>
                      <TableHead>
                        <TableRow>
                          <TableHeader>Identifier/Email</TableHeader>
                          <TableHeader>Status</TableHeader>
                          <TableHeader>Quota Remaining</TableHeader>
                          <TableHeader>Active</TableHeader>
                          <TableHeader className="w-16 text-right"></TableHeader>
                        </TableRow>
                      </TableHead>
                      <TableBody>
                        {accounts.map((acc) => (
                          <TableRow key={acc.id}>
                            <td className="p-3 text-xs font-semibold text-text-secondary select-all">{acc.email}</td>
                            <td className="p-3 text-xs">
                              <Badge variant={acc.status === 'active' ? 'success' : 'default'} className="text-[10px]">
                                {acc.status || 'unknown'}
                              </Badge>
                            </td>
                            <td className="p-3 text-xs font-mono font-medium text-muted">
                              {acc.quota_remaining?.toFixed(2)} / {acc.quota_limit?.toFixed(2)}
                            </td>
                            <td className="p-3 text-xs">
                              <button onClick={() => toggleAccountEnabled(acc)} className="focus:outline-none">
                                <Badge variant={acc.enabled ? 'success' : 'default'} className="cursor-pointer text-[10px]">
                                  {acc.enabled ? 'On' : 'Off'}
                                </Badge>
                              </button>
                            </td>
                            <td className="p-3 text-right">
                              <button
                                onClick={() => handleDeleteAccount(acc.id)}
                                className="text-muted hover:text-danger p-1"
                              >
                                <Trash2 className="w-4 h-4" />
                              </button>
                            </td>
                          </TableRow>
                        ))}
                      </TableBody>
                    </Table>
                  </div>
                )}
              </div>
            </div>
          )}

          {/* TAB 3: Models Configuration */}
          {activeTab === 'models' && (
            <div className="space-y-4">
              <div>
                <label htmlFor="provider-models" className="block text-sm font-semibold text-text-secondary mb-1">
                  Custom Models Identifier List
                </label>
                <textarea
                  id="provider-models"
                  rows={8}
                  value={modelsInput}
                  onChange={(e) => setModelsInput(e.target.value)}
                  placeholder={'gpt-4o\ngpt-4o-mini'}
                  className="w-full rounded-lg border border-border bg-surface text-text px-3 py-2 text-sm font-mono placeholder:text-muted/60 focus:outline-none focus:ring-2 focus:ring-primary/50 focus:border-primary/50 transition-all duration-150"
                />
                <p className="mt-1.5 text-xs text-muted">
                  Specify which model identifiers this provider will load-balance. Enter one model ID per line, or separate with commas.
                </p>
              </div>

              <div className="flex gap-2 justify-end pt-3">
                <Button variant="ghost" onClick={() => setDetailModalOpen(false)}>Close</Button>
                <Button onClick={handleSaveModels} loading={savingModels} variant="primary" className="font-semibold">
                  Save Model List
                </Button>
              </div>
            </div>
          )}

          {/* TAB 4: Playground Tester */}
          {activeTab === 'tester' && (
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              {/* Form inputs */}
              <div className="space-y-4 pr-0 md:pr-4 border-r-0 md:border-r border-border/40">
                <div>
                  <label className="block text-xs font-semibold text-muted uppercase tracking-wider mb-1 font-sans">
                    Testing Model
                  </label>
                  {providerModels.length > 0 ? (
                    <Select
                      value={testerModel}
                      onChange={(e) => setTesterModel(e.target.value)}
                      className="w-full text-xs h-9 font-mono"
                    >
                      {/* Auto select first model */}
                      {(!testerModel && providerModels.length > 0) && setTesterModel(providerModels[0])}
                      {providerModels.map((m) => (
                        <option key={m} value={m}>
                          {m}
                        </option>
                      ))}
                    </Select>
                  ) : (
                    <div className="p-3 text-xs text-danger font-medium border border-danger/20 rounded-lg bg-danger/5">
                      No models configured for this provider. Configure models first.
                    </div>
                  )}
                </div>

                <div>
                  <label className="block text-xs font-semibold text-muted uppercase tracking-wider mb-1 font-sans">
                    Switchblade Gateway API Key
                  </label>
                  {userKeys.length > 0 ? (
                    <Select
                      value={selectedKey}
                      onChange={(e) => setSelectedKey(e.target.value)}
                      className="w-full text-xs h-9 font-mono"
                    >
                      {userKeys.map((k) => (
                        <option key={k.id} value={k.name}>
                          {k.name}
                        </option>
                      ))}
                    </Select>
                  ) : (
                    <Input
                      placeholder="No API key found. Enter manually..."
                      value={selectedKey}
                      onChange={(e) => setSelectedKey(e.target.value)}
                      className="h-9 py-1 text-xs font-mono"
                    />
                  )}
                </div>

                <div>
                  <label htmlFor="test-prompt" className="block text-xs font-semibold text-muted uppercase tracking-wider mb-1 font-sans">
                    Test Prompt
                  </label>
                  <textarea
                    id="test-prompt"
                    rows={4}
                    value={testerPrompt}
                    onChange={(e) => setTesterPrompt(e.target.value)}
                    className="w-full rounded-lg border border-border bg-surface text-text px-3 py-2 text-xs placeholder:text-muted/40 focus:outline-none focus:ring-2 focus:ring-primary/50 focus:border-primary/50 transition-all duration-150"
                  />
                </div>

                <Button
                  onClick={handleRunTest}
                  disabled={!testerModel && providerModels.length === 0}
                  loading={testing}
                  className="w-full font-semibold gap-1 text-xs"
                >
                  <Play className="w-3.5 h-3.5" />
                  Run Connectivity Test
                </Button>
              </div>

              {/* Logs / Console output */}
              <div className="space-y-2">
                <span className="text-xs font-bold text-text-secondary font-sans block">
                  Gateway API logs
                </span>
                <pre className="p-3 bg-background border border-border/80 rounded-lg text-xs font-mono text-text select-all overflow-x-auto h-72 overflow-y-auto leading-relaxed shadow-inner">
                  {testResponse || 'Logs will output here when test runs.'}
                </pre>
              </div>
            </div>
          )}
        </div>
      </Modal>
    </div>
  );
}
