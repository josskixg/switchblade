import { useEffect, useState } from 'react';
import { client } from '../../api/client';
import { useToast } from '../../components/ui/Toast';
import Card from '../../components/ui/Card';
import Button from '../../components/ui/Button';
import Input from '../../components/ui/Input';
import Select from '../../components/ui/Select';
import Skeleton from '../../components/ui/Skeleton';
import { Copy, Check, BookOpen, Key, Terminal, Code } from 'lucide-react';

export default function References() {
  const [loading, setLoading] = useState(true);
  const [keys, setKeys] = useState([]);
  const [models, setModels] = useState([]);
  const [combos, setCombos] = useState([]);
  
  // Selection States
  const [selectedKey, setSelectedKey] = useState('YOUR_API_KEY');
  const [selectedModel, setSelectedModel] = useState('gpt-4o');
  const [gatewayUrl, setGatewayUrl] = useState('');
  
  const [copiedId, setCopiedId] = useState(null);
  const [activeGuideTab, setActiveGuideTab] = useState('cursor');
  const toast = useToast();

  useEffect(() => {
    // Determine local Gateway URL (default port 2005)
    const hostname = window.location.hostname || 'localhost';
    setGatewayUrl(`http://${hostname}:2005/v1`);

    loadData();
  }, []);

  async function loadData() {
    setLoading(true);
    try {
      // 1. Fetch available models & combos for selection
      const [modelsRes, combosRes] = await Promise.allSettled([
        client.get('/api/models'),
        client.get('/api/model-combos'),
      ]);

      let parsedModels = [];
      if (modelsRes.status === 'fulfilled') {
        const data = modelsRes.value.data?.data || modelsRes.value.data?.providers || modelsRes.value.data || [];
        if (Array.isArray(data)) {
          data.forEach(item => {
            const list = Array.isArray(item.models) ? item.models : [item.id || item.model].filter(Boolean);
            list.forEach(m => {
              const name = typeof m === 'string' ? m : m?.id;
              if (name) parsedModels.push(name);
            });
          });
        }
      }
      setModels([...new Set(parsedModels)]);

      if (combosRes.status === 'fulfilled') {
        setCombos(Array.isArray(combosRes.value.data) ? combosRes.value.data : []);
      }

      // 2. Fetch keys for current tenant if available
      const storedTenant = localStorage.getItem('sb_active_tenant_id');
      if (storedTenant) {
        const keysRes = await client.get('/api/keys/v2', { params: { tenant_id: storedTenant } });
        const list = keysRes.data?.keys || keysRes.data || [];
        if (Array.isArray(list) && list.length > 0) {
          setKeys(list);
          setSelectedKey(list[0].name); // Use the first key name as a placeholder/hint
        }
      }
    } catch {
      // Non-blocking fallback if endpoints fail
    } finally {
      setLoading(false);
    }
  }

  function handleCopy(text, id) {
    navigator.clipboard.writeText(text);
    setCopiedId(id);
    toast('Copied setup snippet');
    setTimeout(() => setCopiedId(null), 2000);
  }

  // --- Snippet Generators ---
  const guides = {
    cursor: {
      title: 'Cursor Editor',
      description: 'Configure Cursor to route completions and chat requests through your local proxy gateway.',
      steps: [
        'Open Cursor Settings > Models.',
        'Disable default OpenAI and Anthropic provider toggles.',
        `Under OpenAI API, toggle "Override OpenAI Base URL".`,
        `Set the Base URL override to: \`${gatewayUrl}\``,
        `Enter your Switchblade tenant API key: \`${selectedKey}\``,
        `Add \`${selectedModel}\` in the Models List and check/select it as your active model.`
      ],
      codeTitle: 'Workspace .cursorrules recommendation',
      code: `// Set this in your project root's .cursorrules to optimize assistant prompts:
{
  "instruction": "Route all requests to local Switchblade proxy gateway using model: ${selectedModel}",
  "preferred_model": "${selectedModel}"
}`
    },
    cline: {
      title: 'Cline / Roo Code (VSCode)',
      description: 'Run VSCode AI extensions using custom OpenAI-compatible settings.',
      steps: [
        'Open Cline Settings (Gear Icon).',
        'Set "API Provider" to "OpenAI Compatible".',
        `Set "Base URL" to: \`${gatewayUrl}\``,
        `Set "API Key" to: \`${selectedKey}\``,
        `Set "Model ID" to: \`${selectedModel}\``
      ],
      codeTitle: 'cline_custom_modes.json config snippet',
      code: `{
  "apiProvider": "open-ai-compatible",
  "openAiCompatibleBaseUrl": "${gatewayUrl}",
  "openAiCompatibleApiKey": "${selectedKey}",
  "openAiCompatibleModelId": "${selectedModel}"
}`
    },
    sdk_python: {
      title: 'Python SDK',
      description: 'Use the official openai python package pointing directly to your gateway.',
      steps: [
        'Install the package: `pip install openai`',
        'Initialize the client override variables as shown below.'
      ],
      codeTitle: 'main.py code example',
      code: `import openai

client = openai.OpenAI(
    base_url="${gatewayUrl}",
    api_key="${selectedKey}"
)

response = client.chat.completions.create(
    model="${selectedModel}",
    messages=[
        {"role": "user", "content": "Hello! How are you?"}
    ]
)

print(response.choices[0].message.content)`
    },
    sdk_node: {
      title: 'Node.js SDK',
      description: 'Use the official openai javascript package inside Node or Next.js projects.',
      steps: [
        'Install the package: `npm install openai`',
        'Initialize the OpenAI client overrides.'
      ],
      codeTitle: 'index.js code example',
      code: `const OpenAI = require('openai');

const openai = new OpenAI({
  baseURL: '${gatewayUrl}',
  apiKey: '${selectedKey}'
});

async function main() {
  const completion = await openai.chat.completions.create({
    model: '${selectedModel}',
    messages: [{ role: 'user', content: 'Hello gateway!' }],
  });
  console.log(completion.choices[0].message.content);
}
main();`
    },
    curl: {
      title: 'cURL / CLI Request',
      description: 'Test the proxy connectivity and prompt resolution directly from your terminal.',
      steps: [
        'Copy the command block below and paste it in bash, command prompt, or powershell.'
      ],
      codeTitle: 'terminal prompt test command',
      code: `curl -X POST "${gatewayUrl}/chat/completions" \\
  -H "Authorization: Bearer ${selectedKey}" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "${selectedModel}",
    "messages": [
      {
        "role": "user",
        "content": "Ping"
      }
    ]
  }'`
    }
  };

  const activeGuide = guides[activeGuideTab];

  return (
    <div className="space-y-6 max-w-5xl">
      <div>
        <h1 className="text-2xl font-bold text-text">Interactive Setup Guides</h1>
        <p className="text-sm text-muted mt-1">
          Dynamic reference instructions to connect Cursor, Cline, SDKs, or raw HTTP clients to your active gateway
        </p>
      </div>

      {/* Control Selector Panel */}
      <Card>
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          <div>
            <label className="block text-xs font-semibold text-muted uppercase tracking-wider mb-1.5 font-sans">
              Gateway Target Key / Token
            </label>
            <div className="relative">
              {keys.length > 0 ? (
                <Select
                  value={selectedKey}
                  onChange={(e) => setSelectedKey(e.target.value)}
                  className="w-full text-sm h-10"
                >
                  {keys.map((k) => (
                    <option key={k.id} value={k.name}>
                      {k.name} (tenant key)
                    </option>
                  ))}
                </Select>
              ) : (
                <Input
                  value={selectedKey}
                  onChange={(e) => setSelectedKey(e.target.value)}
                  placeholder="YOUR_API_KEY"
                  className="w-full text-sm h-10"
                />
              )}
            </div>
          </div>

          <div>
            <label className="block text-xs font-semibold text-muted uppercase tracking-wider mb-1.5 font-sans">
              Gateway Base URL override
            </label>
            <Input
              value={gatewayUrl}
              onChange={(e) => setGatewayUrl(e.target.value)}
              placeholder="http://localhost:2005/v1"
              className="w-full text-sm h-10 font-mono"
            />
          </div>

          <div>
            <label className="block text-xs font-semibold text-muted uppercase tracking-wider mb-1.5 font-sans">
              Target Model or Failover Combo
            </label>
            <Select
              value={selectedModel}
              onChange={(e) => setSelectedModel(e.target.value)}
              className="w-full text-sm h-10 font-mono"
            >
              {/* Show default options if loading failed */}
              {models.length === 0 && combos.length === 0 && (
                <>
                  <option value="gpt-4o">gpt-4o (default)</option>
                  <option value="claude-3-5-sonnet">claude-3-5-sonnet</option>
                  <option value="deepseek-coder">deepseek-coder</option>
                </>
              )}
              {combos.length > 0 && (
                <optgroup label="Model Combos / Fallback Chains">
                  {combos.map((c) => (
                    <option key={`combo-${c.id}`} value={c.name}>
                      {c.name}
                    </option>
                  ))}
                </optgroup>
              )}
              {models.length > 0 && (
                <optgroup label="Discovered Single Models">
                  {models.map((m) => (
                    <option key={`model-${m}`} value={m}>
                      {m}
                    </option>
                  ))}
                </optgroup>
              )}
            </Select>
          </div>
        </div>
      </Card>

      {/* Guide Tab Selection and Output */}
      <div className="grid grid-cols-1 md:grid-cols-4 gap-6">
        {/* Left tabs selector */}
        <div className="md:col-span-1 space-y-1.5">
          <button
            onClick={() => setActiveGuideTab('cursor')}
            className={`w-full text-left px-4 py-3 rounded-lg text-xs font-bold transition-all duration-150 border flex items-center gap-2.5 ${
              activeGuideTab === 'cursor'
                ? 'bg-primary text-primary-foreground border-primary'
                : 'bg-surface hover:bg-surface-hover text-muted hover:text-text border-border/40'
            }`}
          >
            <BookOpen className="w-4 h-4 shrink-0" />
            Cursor
          </button>
          <button
            onClick={() => setActiveGuideTab('cline')}
            className={`w-full text-left px-4 py-3 rounded-lg text-xs font-bold transition-all duration-150 border flex items-center gap-2.5 ${
              activeGuideTab === 'cline'
                ? 'bg-primary text-primary-foreground border-primary'
                : 'bg-surface hover:bg-surface-hover text-muted hover:text-text border-border/40'
            }`}
          >
            <Terminal className="w-4 h-4 shrink-0" />
            Cline / Roo Code
          </button>
          <button
            onClick={() => setActiveGuideTab('sdk_python')}
            className={`w-full text-left px-4 py-3 rounded-lg text-xs font-bold transition-all duration-150 border flex items-center gap-2.5 ${
              activeGuideTab === 'sdk_python'
                ? 'bg-primary text-primary-foreground border-primary'
                : 'bg-surface hover:bg-surface-hover text-muted hover:text-text border-border/40'
            }`}
          >
            <Code className="w-4 h-4 shrink-0" />
            Python SDK
          </button>
          <button
            onClick={() => setActiveGuideTab('sdk_node')}
            className={`w-full text-left px-4 py-3 rounded-lg text-xs font-bold transition-all duration-150 border flex items-center gap-2.5 ${
              activeGuideTab === 'sdk_node'
                ? 'bg-primary text-primary-foreground border-primary'
                : 'bg-surface hover:bg-surface-hover text-muted hover:text-text border-border/40'
            }`}
          >
            <Code className="w-4 h-4 shrink-0" />
            Node.js SDK
          </button>
          <button
            onClick={() => setActiveGuideTab('curl')}
            className={`w-full text-left px-4 py-3 rounded-lg text-xs font-bold transition-all duration-150 border flex items-center gap-2.5 ${
              activeGuideTab === 'curl'
                ? 'bg-primary text-primary-foreground border-primary'
                : 'bg-surface hover:bg-surface-hover text-muted hover:text-text border-border/40'
            }`}
          >
            <Terminal className="w-4 h-4 shrink-0" />
            cURL Request
          </button>
        </div>

        {/* Right guides contents */}
        <div className="md:col-span-3">
          <Card>
            <div className="space-y-4">
              <div>
                <h3 className="text-lg font-bold text-text-secondary capitalize">{activeGuide.title} Setup Guide</h3>
                <p className="text-xs text-muted mt-1 leading-relaxed">{activeGuide.description}</p>
              </div>

              {/* Step By Step Instructions */}
              <div className="bg-surface/30 p-4 rounded-lg border border-border/60">
                <h4 className="text-xs font-bold text-text uppercase tracking-wider mb-2.5 font-sans">
                  Steps to Configure
                </h4>
                <ol className="list-decimal list-inside space-y-2 text-xs text-muted leading-relaxed font-medium">
                  {activeGuide.steps.map((step, idx) => (
                    <li key={idx} className="marker:text-primary marker:font-bold">
                      <span className="text-text-secondary select-text pl-1">{step}</span>
                    </li>
                  ))}
                </ol>
              </div>

              {/* Editable/Copyable Code Block */}
              {activeGuide.code && (
                <div className="space-y-2">
                  <div className="flex items-center justify-between">
                    <span className="text-xs font-bold text-text font-sans">
                      {activeGuide.codeTitle}
                    </span>
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() => handleCopy(activeGuide.code, activeGuideTab)}
                      className="h-7 text-xs gap-1.5 px-2 hover:bg-surface-hover"
                    >
                      {copiedId === activeGuideTab ? (
                        <>
                          <Check className="w-3.5 h-3.5 text-success animate-scaleIn" />
                          <span className="text-success font-semibold">Copied</span>
                        </>
                      ) : (
                        <>
                          <Copy className="w-3.5 h-3.5" />
                          <span>Copy</span>
                        </>
                      )}
                    </Button>
                  </div>
                  <pre className="p-4 bg-background border border-border/80 rounded-lg text-xs font-mono text-text select-all overflow-x-auto leading-relaxed shadow-inner">
                    {activeGuide.code}
                  </pre>
                </div>
              )}
            </div>
          </Card>
        </div>
      </div>
    </div>
  );
}
