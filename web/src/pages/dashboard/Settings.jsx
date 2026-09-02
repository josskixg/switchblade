import { useEffect, useState } from 'react';
import { client } from '../../api/client';
import { useToast } from '../../components/ui/Toast';
import Card from '../../components/ui/Card';
import Button from '../../components/ui/Button';
import Input from '../../components/ui/Input';
import Skeleton from '../../components/ui/Skeleton';
import { Save, Plus, Trash2, Settings as SettingsIcon } from 'lucide-react';

export default function Settings() {
  const [settings, setSettings] = useState({});
  const [loading, setLoading] = useState(true);
  const [newKey, setNewKey] = useState('');
  const [newValue, setNewValue] = useState('');
  const toast = useToast();

  useEffect(() => { load(); }, []);

  async function load() {
    try {
      const res = await client.get('/api/settings');
      setSettings(res.data || {});
    } catch {
      toast.error('Failed to load settings');
    } finally {
      setLoading(false);
    }
  }

  async function saveSetting(key, value) {
    try {
      await client.post('/api/settings', { key, value: String(value) });
      toast(`Setting "${key}" saved`);
      load();
    } catch {
      toast.error('Failed to save setting');
    }
  }

  async function updateValue(key, value) {
    setSettings({ ...settings, [key]: value });
  }

  async function addSetting() {
    if (!newKey) return;
    await saveSetting(newKey, newValue);
    setNewKey('');
    setNewValue('');
  }

  if (loading) {
    return (
      <div className="space-y-6">
        <div>
          <Skeleton className="h-8 w-48 mb-2" />
          <Skeleton className="h-4 w-72" />
        </div>
        <Card>
          <div className="space-y-3">
            {Array.from({ length: 5 }).map((_, i) => (
              <Skeleton key={i} className="h-12 w-full" />
            ))}
          </div>
        </Card>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-text">Proxy Settings</h1>
        <p className="text-sm text-muted mt-1">Configure proxy behavior and feature toggles</p>
      </div>

      <Card>
        <h3 className="text-base font-semibold text-text mb-4">Configuration</h3>
        <div className="space-y-3">
          {Object.keys(settings).length === 0 ? (
            <div className="flex flex-col items-center justify-center py-8 gap-2 text-muted">
              <SettingsIcon className="w-8 h-8 text-muted/30" />
              <p className="text-sm">No settings configured</p>
            </div>
          ) : (
            Object.entries(settings).map(([key, val]) => (
              <div key={key} className="flex items-center gap-3">
                <div className="w-48 shrink-0">
                  <code className="text-xs font-mono text-muted">{key}</code>
                </div>
                <Input
                  value={typeof val === 'boolean' ? String(val) : String(val ?? '')}
                  onChange={(e) => updateValue(key, e.target.value)}
                  className="flex-1"
                />
                <Button size="sm" variant="secondary" onClick={() => saveSetting(key, settings[key])}>
                  <Save className="w-3.5 h-3.5" />
                </Button>
              </div>
            ))
          )}
        </div>
      </Card>

      <Card>
        <h3 className="text-base font-semibold text-text mb-4">Add Setting</h3>
        <div className="flex items-end gap-3">
          <Input label="Key" value={newKey} onChange={(e) => setNewKey(e.target.value)} placeholder="rate_limit_enabled" className="flex-1" />
          <Input label="Value" value={newValue} onChange={(e) => setNewValue(e.target.value)} placeholder="true" className="flex-1" />
          <Button onClick={addSetting} disabled={!newKey}>
            <Plus className="w-3.5 h-3.5" /> Add
          </Button>
        </div>
      </Card>
    </div>
  );
}
