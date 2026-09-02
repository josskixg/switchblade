import { useState, useEffect, useCallback } from 'react';
import { client } from '../../api/client';
import { useToast } from '../../components/ui/Toast';
import Card from '../../components/ui/Card';
import Button from '../../components/ui/Button';
import Input from '../../components/ui/Input';
import { UserRound, KeySquare, Fingerprint, Braces, Clock, Copy, RefreshCw, Check } from 'lucide-react';

function CopyButton({ text, size = 'sm' }) {
  const [copied, setCopied] = useState(false);
  const toast = useToast();

  function copy() {
    if (!text) return;
    navigator.clipboard.writeText(text).then(() => {
      setCopied(true);
      toast('Copied to clipboard');
      setTimeout(() => setCopied(false), 1500);
    });
  }

  return (
    <Button variant="ghost" size={size} onClick={copy} className="shrink-0">
      {copied ? <Check className="w-3.5 h-3.5" /> : <Copy className="w-3.5 h-3.5" />}
      {copied ? 'Copied' : 'Copy'}
    </Button>
  );
}

function FieldRow({ label, value }) {
  if (!value) return null;
  return (
    <div className="flex items-center justify-between gap-3 py-2.5 border-b border-border/40 last:border-0 hover:bg-surface-hover/40 -mx-2 px-2 rounded-md transition-colors duration-150">
      <span className="text-xs text-muted uppercase tracking-wider shrink-0">{label}</span>
      <div className="flex items-center gap-2 min-w-0">
        <span className="text-sm text-text font-mono truncate">{value}</span>
        <CopyButton text={value} />
      </div>
    </div>
  );
}

function IdentityGenerator() {
  const [identity, setIdentity] = useState(null);
  const [loading, setLoading] = useState(false);
  const [history, setHistory] = useState([]);
  const toast = useToast();

  const generate = useCallback(async () => {
    setLoading(true);
    try {
      const res = await client.get('/api/tools/random-identity');
      const data = res.data;
      setIdentity(data);
      setHistory((prev) => [data, ...prev].slice(0, 5));
    } catch {
      toast.error('Failed to generate identity');
    } finally {
      setLoading(false);
    }
  }, [toast]);

  function copyAll() {
    if (!identity) return;
    const json = JSON.stringify(identity, null, 2);
    navigator.clipboard.writeText(json);
    toast('Full identity copied as JSON');
  }

  return (
    <Card>
      <div className="flex items-center justify-between mb-4">
        <div className="flex items-center gap-2">
          <div className="w-8 h-8 rounded-lg flex items-center justify-center bg-primary/10">
            <UserRound className="w-4 h-4 text-primary" />
          </div>
          <h3 className="text-base font-semibold text-text">Random Identity</h3>
        </div>
        {identity && (
          <Button size="sm" variant="ghost" onClick={copyAll}>
            <Copy className="w-3.5 h-3.5" /> All
          </Button>
        )}
      </div>

      {identity ? (
        <div className="space-y-0">
          <FieldRow label="Name" value={identity.full_name || [identity.first_name, identity.last_name].filter(Boolean).join(' ')} />
          <FieldRow label="Email" value={identity.email} />
          <FieldRow label="Username" value={identity.username} />
          <FieldRow label="Phone" value={identity.phone} />
          <FieldRow label="Address" value={[identity.street, identity.city, identity.state, identity.zip].filter(Boolean).join(', ')} />
          <FieldRow label="Country" value={identity.country} />
        </div>
      ) : (
        <p className="text-sm text-muted py-8 text-center">Click generate to create a random identity</p>
      )}

      <div className="flex gap-2 mt-4">
        <Button size="sm" onClick={generate} loading={loading} className="flex-1">
          <RefreshCw className="w-3.5 h-3.5" />
          {identity ? 'Generate Another' : 'Generate'}
        </Button>
      </div>

      {history.length > 1 && (
        <div className="mt-4 pt-4 border-t border-border">
          <p className="text-xs text-muted uppercase tracking-wider mb-2">History</p>
          <div className="space-y-0">
            {history.slice(1).map((h, i) => (
              <button
                key={i}
                onClick={() => setIdentity(h)}
                className="block w-full text-left text-xs text-muted hover:text-text px-2 py-2 rounded-md hover:bg-surface-hover border-b border-border/30 last:border-0 transition-colors duration-150 truncate"
              >
                {h.full_name || h.email || 'Unknown'} — {h.email}
              </button>
            ))}
          </div>
        </div>
      )}
    </Card>
  );
}

function passwordStrength(pw) {
  if (!pw) return { label: '', color: '' };
  let score = 0;
  if (pw.length >= 12) score++;
  if (pw.length >= 20) score++;
  if (/[a-z]/.test(pw) && /[A-Z]/.test(pw)) score++;
  if (/\d/.test(pw)) score++;
  if (/[^a-zA-Z0-9]/.test(pw)) score++;
  if (score <= 1) return { label: 'Weak', color: 'text-danger', bar: 'bg-danger', w: '25%' };
  if (score <= 3) return { label: 'Medium', color: 'text-warning', bar: 'bg-warning', w: '60%' };
  return { label: 'Strong', color: 'text-success', bar: 'bg-success', w: '100%' };
}

function PasswordGenerator() {
  const [password, setPassword] = useState('');
  const [length, setLength] = useState(16);
  const [loading, setLoading] = useState(false);
  const toast = useToast();

  async function generate() {
    setLoading(true);
    try {
      const res = await client.get(`/api/tools/password?length=${length}`);
      setPassword(res.data?.password || '');
    } catch {
      toast.error('Failed to generate password');
    } finally {
      setLoading(false);
    }
  }

  const strength = passwordStrength(password);

  return (
    <Card>
      <div className="flex items-center gap-2 mb-4">
        <div className="w-8 h-8 rounded-lg flex items-center justify-center bg-primary/10">
          <KeySquare className="w-4 h-4 text-primary" />
        </div>
        <h3 className="text-base font-semibold text-text">Password Generator</h3>
      </div>

      <div className="mb-4">
        <div className="flex items-center justify-between mb-1.5">
          <label className="text-sm font-medium text-text">Length</label>
          <span className="text-sm font-mono text-primary tabular-nums">{length}</span>
        </div>
        <input
          type="range"
          min="8"
          max="64"
          value={length}
          onChange={(e) => setLength(Number(e.target.value))}
          className="w-full accent-primary cursor-pointer"
        />
      </div>

      {password ? (
        <div className="mb-4">
          <div className="flex items-center gap-2 p-3 rounded-lg bg-surface-elevated border border-border">
            <code className="flex-1 text-sm font-mono text-text break-all">{password}</code>
            <CopyButton text={password} />
          </div>
          <div className="flex items-center gap-2 mt-2">
            <div className="flex-1 h-1.5 rounded-full bg-border overflow-hidden">
              <div className={`h-full rounded-full transition-all duration-300 ${strength.bar}`} style={{ width: strength.w }} />
            </div>
            <span className={`text-xs font-medium ${strength.color}`}>{strength.label}</span>
          </div>
        </div>
      ) : (
        <p className="text-sm text-muted py-6 text-center mb-4">Click generate to create a password</p>
      )}

      <Button size="sm" onClick={generate} loading={loading} className="w-full">
        <RefreshCw className="w-3.5 h-3.5" /> Generate Password
      </Button>
    </Card>
  );
}

function UuidGenerator() {
  const [uuids, setUuids] = useState([]);
  const toast = useToast();

  function generate(count) {
    const list = Array.from({ length: count }, () => crypto.randomUUID());
    setUuids((prev) => [...list, ...prev].slice(0, 50));
  }

  function copyAll() {
    if (uuids.length === 0) return;
    navigator.clipboard.writeText(uuids.join('\n'));
    toast('All UUIDs copied');
  }

  return (
    <Card>
      <div className="flex items-center justify-between mb-4">
        <div className="flex items-center gap-2">
          <div className="w-8 h-8 rounded-lg flex items-center justify-center bg-primary/10">
            <Fingerprint className="w-4 h-4 text-primary" />
          </div>
          <h3 className="text-base font-semibold text-text">UUID Generator</h3>
        </div>
        {uuids.length > 0 && (
          <Button size="sm" variant="ghost" onClick={copyAll}>
            <Copy className="w-3.5 h-3.5" /> All
          </Button>
        )}
      </div>

      <div className="flex gap-2 mb-4">
        <Button size="sm" variant="secondary" onClick={() => generate(1)}>1</Button>
        <Button size="sm" variant="secondary" onClick={() => generate(5)}>5</Button>
        <Button size="sm" variant="secondary" onClick={() => generate(10)}>10</Button>
        <Button size="sm" variant="secondary" onClick={() => generate(25)}>25</Button>
      </div>

      {uuids.length === 0 ? (
        <p className="text-sm text-muted py-6 text-center">Click a button to generate UUIDs</p>
      ) : (
        <div className="max-h-64 overflow-y-auto -mx-1">
          {(uuids || []).map((uuid, i) => (
            <div key={i} className="flex items-center gap-2 py-2 px-2 border-b border-border/40 last:border-0 hover:bg-surface-hover rounded-md transition-colors duration-150">
              <span className="text-[10px] text-muted/50 font-mono w-5 shrink-0">{i + 1}</span>
              <code className="flex-1 text-xs font-mono text-text truncate">{uuid}</code>
              <CopyButton text={uuid} />
            </div>
          ))}
        </div>
      )}
    </Card>
  );
}

function JsonFormatter() {
  const [input, setInput] = useState('');
  const [output, setOutput] = useState('');
  const [error, setError] = useState('');
  const toast = useToast();

  function format() {
    setError('');
    setOutput('');
    if (!input.trim()) {
      setError('Input is empty');
      return;
    }
    try {
      const parsed = JSON.parse(input);
      setOutput(JSON.stringify(parsed, null, 2));
    } catch (e) {
      setError(e.message);
    }
  }

  function minify() {
    setError('');
    setOutput('');
    if (!input.trim()) {
      setError('Input is empty');
      return;
    }
    try {
      const parsed = JSON.parse(input);
      setOutput(JSON.stringify(parsed));
    } catch (e) {
      setError(e.message);
    }
  }

  return (
    <Card>
      <div className="flex items-center gap-2 mb-4">
        <div className="w-8 h-8 rounded-lg flex items-center justify-center bg-primary/10">
          <Braces className="w-4 h-4 text-primary" />
        </div>
        <h3 className="text-base font-semibold text-text">JSON Formatter</h3>
      </div>

      <textarea
        value={input}
        onChange={(e) => setInput(e.target.value)}
        placeholder='{"key": "value"}'
        rows={4}
        className="w-full rounded-lg border border-border bg-surface text-text px-3 py-2 text-sm font-mono placeholder:text-muted/60 focus:outline-none focus:ring-2 focus:ring-primary/50 focus:border-primary/50 transition-all duration-150 resize-y"
      />

      <div className="flex gap-2 my-3">
        <Button size="sm" onClick={format}>Format</Button>
        <Button size="sm" variant="secondary" onClick={minify}>Minify</Button>
        <Button size="sm" variant="ghost" onClick={() => { setInput(''); setOutput(''); setError(''); }}>Clear</Button>
      </div>

      {error ? (
        <div className="p-3 rounded-lg bg-danger/10 border border-danger/20 text-sm text-danger font-mono break-all">{error}</div>
      ) : output ? (
        <div className="relative">
          <pre className="p-3 rounded-lg bg-surface-elevated border border-border text-sm font-mono text-text overflow-x-auto max-h-48 overflow-y-auto">{output}</pre>
          <div className="absolute top-2 right-2">
            <CopyButton text={output} />
          </div>
        </div>
      ) : null}
    </Card>
  );
}

function TimestampConverter() {
  const [now, setNow] = useState(Math.floor(Date.now() / 1000));
  const [tsInput, setTsInput] = useState('');
  const [dateInput, setDateInput] = useState('');

  useEffect(() => {
    const interval = setInterval(() => setNow(Math.floor(Date.now() / 1000)), 1000);
    return () => clearInterval(interval);
  }, []);

  const tsResult = tsInput ? (() => {
    const n = Number(tsInput);
    if (isNaN(n)) return 'Invalid number';
    const d = new Date(n * 1000);
    if (isNaN(d.getTime())) return 'Invalid timestamp';
    return d.toUTCString();
  })() : '';

  const dateResult = dateInput ? (() => {
    const d = new Date(dateInput);
    if (isNaN(d.getTime())) return 'Invalid date';
    return String(Math.floor(d.getTime() / 1000));
  })() : '';

  return (
    <Card>
      <div className="flex items-center gap-2 mb-4">
        <div className="w-8 h-8 rounded-lg flex items-center justify-center bg-primary/10">
          <Clock className="w-4 h-4 text-primary" />
        </div>
        <h3 className="text-base font-semibold text-text">Timestamp Converter</h3>
      </div>

      <div className="mb-4 p-3 rounded-lg bg-surface-elevated border border-border text-center">
        <p className="text-xs text-muted uppercase tracking-wider mb-1">Current Unix Time</p>
        <p className="text-xl font-bold text-text font-mono tabular-nums">{now}</p>
      </div>

      <div className="space-y-4">
        <div>
          <Input
            label="Timestamp → Date"
            value={tsInput}
            onChange={(e) => setTsInput(e.target.value)}
            placeholder="1700000000"
            className="font-mono"
          />
          {tsResult && (
            <p className="mt-1.5 text-sm text-text font-mono break-all">{tsResult}</p>
          )}
        </div>

        <div>
          <Input
            label="Date → Timestamp"
            value={dateInput}
            onChange={(e) => setDateInput(e.target.value)}
            placeholder="2024-01-01T00:00:00Z"
            className="font-mono"
          />
          {dateResult && (
            <div className="flex items-center gap-2 mt-1.5">
              <p className="text-sm text-text font-mono">{dateResult}</p>
              <CopyButton text={dateResult} />
            </div>
          )}
        </div>
      </div>
    </Card>
  );
}

export default function Tools() {
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-text">Tools</h1>
        <p className="text-sm text-muted mt-1">Utility tools for everyday tasks</p>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <IdentityGenerator />
        <PasswordGenerator />
        <UuidGenerator />
        <JsonFormatter />
        <TimestampConverter />
      </div>
    </div>
  );
}
