import { useEffect, useState } from 'react';
import { client } from '../../api/client';
import { useToast } from '../../components/ui/Toast';
import Card from '../../components/ui/Card';
import Skeleton from '../../components/ui/Skeleton';
import { Badge } from '../../components/ui/Badge';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '../../components/ui/Table';
import { MessageSquare, AudioLines, Type, Mic } from 'lucide-react';

const serviceKinds = [
  { key: 'chat', label: 'Chat', icon: MessageSquare, color: 'text-primary', bg: 'bg-primary/10' },
  { key: 'embeddings', label: 'Embeddings', icon: Type, color: 'text-blue-400', bg: 'bg-blue-400/10' },
  { key: 'tts', label: 'TTS', icon: AudioLines, color: 'text-purple-400', bg: 'bg-purple-400/10' },
  { key: 'stt', label: 'STT', icon: Mic, color: 'text-emerald-400', bg: 'bg-emerald-400/10' },
];

function formatNum(n) {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
  return String(n ?? 0);
}

function ServiceCard({ kind, stats, loading }) {
  const Icon = kind.icon;
  if (loading) {
    return (
      <Card>
        <div className="flex items-center gap-4 mb-4">
          <Skeleton className="w-11 h-11 rounded-lg shrink-0" />
          <div className="space-y-2 flex-1">
            <Skeleton className="h-3 w-20" />
            <Skeleton className="h-6 w-14" />
          </div>
        </div>
        <Skeleton className="h-8 w-full" />
      </Card>
    );
  }
  return (
    <Card>
      <div className="flex items-center gap-4 mb-4">
        <div className={`w-11 h-11 rounded-lg flex items-center justify-center shrink-0 ${kind.bg}`}>
          <Icon className={`w-5 h-5 ${kind.color}`} />
        </div>
        <div>
          <p className="text-xs text-muted uppercase tracking-wider font-medium">{kind.label}</p>
          <p className="text-2xl font-bold text-text mt-0.5 tabular-nums">{formatNum(stats?.requests)}</p>
        </div>
      </div>
      <div className="grid grid-cols-2 gap-3 pt-3 border-t border-border">
        <div>
          <p className="text-xs text-muted">Tokens</p>
          <p className="text-sm font-semibold text-text tabular-nums">{formatNum(stats?.tokens)}</p>
        </div>
        <div>
          <p className="text-xs text-muted">Errors</p>
          <p className="text-sm font-semibold text-text tabular-nums">{formatNum(stats?.errors)}</p>
        </div>
      </div>
    </Card>
  );
}

export default function Services() {
  const [stats, setStats] = useState(null);
  const [loading, setLoading] = useState(true);
  const toast = useToast();

  useEffect(() => {
    fetchStats();
  }, []);

  async function fetchStats() {
    try {
      const res = await client.get('/api/stats');
      setStats(res.data);
    } catch {
      toast.error('Failed to load service stats');
    } finally {
      setLoading(false);
    }
  }

  const serviceStats = stats?.services || {};
  const providers = stats?.providers || [];

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-text">Services</h1>
        <p className="text-sm text-muted mt-1">Usage stats across service kinds</p>
      </div>

      {/* Service kind cards */}
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {serviceKinds.map((kind) => (
          <ServiceCard
            key={kind.key}
            kind={kind}
            stats={serviceStats[kind.key]}
            loading={loading}
          />
        ))}
      </div>

      {/* Provider support matrix */}
      <Card>
        <h3 className="text-base font-semibold text-text mb-4">Provider Support</h3>

        {loading ? (
          <div className="space-y-2">
            <Skeleton className="h-8 w-full" />
            <Skeleton className="h-8 w-full" />
            <Skeleton className="h-8 w-full" />
          </div>
        ) : providers.length === 0 ? (
          <div className="text-center py-8 text-muted text-sm">
            No provider data available
          </div>
        ) : (
          <Table>
            <TableHead>
              <TableHeader>Provider</TableHeader>
              <TableHeader>Chat</TableHeader>
              <TableHeader>Embeddings</TableHeader>
              <TableHeader>TTS</TableHeader>
              <TableHeader>STT</TableHeader>
              <TableHeader>Status</TableHeader>
            </TableHead>
            <TableBody>
              {(providers || []).map((p, i) => (
                <TableRow key={i}>
                  <TableCell className="font-medium">{p.name || p.provider}</TableCell>
                  {['chat', 'embeddings', 'tts', 'stt'].map((kind) => (
                    <TableCell key={kind}>
                      {p[kind] ? (
                        <Badge variant="success">Yes</Badge>
                      ) : (
                        <span className="text-muted text-xs">—</span>
                      )}
                    </TableCell>
                  ))}
                  <TableCell>
                    <span className={`inline-block w-2 h-2 rounded-full mr-1.5 ${p.healthy !== false ? 'bg-success' : 'bg-danger'}`} />
                    <span className="text-sm">{p.healthy !== false ? 'Healthy' : 'Error'}</span>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </Card>
    </div>
  );
}
