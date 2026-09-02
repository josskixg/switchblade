import { useEffect, useState } from 'react';
import { client } from '../../api/client';
import { useToast } from '../../components/ui/Toast';
import Card from '../../components/ui/Card';
import Button from '../../components/ui/Button';
import Skeleton from '../../components/ui/Skeleton';
import CertDownload from '../../components/ui/CertDownload';
import StatusIndicator from '../../components/ui/StatusIndicator';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '../../components/ui/Table';
import { Play, Square, Wifi, WifiOff, Globe, Clock } from 'lucide-react';

export default function MITM() {
  const [status, setStatus] = useState(null);
  const [sessions, setSessions] = useState([]);
  const [loading, setLoading] = useState(true);
  const [actionLoading, setActionLoading] = useState('');
  const toast = useToast();

  useEffect(() => {
    fetchStatus();
    fetchSessions();
  }, []);

  async function fetchStatus() {
    try {
      const res = await client.get('/api/v1/mitm/status');
      setStatus(res.data);
    } catch {
      toast.error('Failed to load MITM status');
    } finally {
      setLoading(false);
    }
  }

  async function fetchSessions() {
    try {
      const res = await client.get('/api/v1/mitm/sessions');
      setSessions(res.data || []);
    } catch {
      // Sessions may not be available if proxy not running
      setSessions([]);
    }
  }

  async function startProxy() {
    setActionLoading('start');
    try {
      await client.post('/api/v1/mitm/start');
      toast('MITM proxy started');
      await fetchStatus();
      await fetchSessions();
    } catch {
      toast.error('Failed to start proxy');
    } finally {
      setActionLoading('');
    }
  }

  async function stopProxy() {
    setActionLoading('stop');
    try {
      await client.post('/api/v1/mitm/stop');
      toast('MITM proxy stopped');
      await fetchStatus();
      setSessions([]);
    } catch {
      toast.error('Failed to stop proxy');
    } finally {
      setActionLoading('');
    }
  }

  async function installDns() {
    setActionLoading('dns-install');
    try {
      await client.post('/api/v1/mitm/dns/install');
      toast('DNS installed');
      await fetchStatus();
    } catch {
      toast.error('Failed to install DNS');
    } finally {
      setActionLoading('');
    }
  }

  async function uninstallDns() {
    setActionLoading('dns-uninstall');
    try {
      await client.post('/api/v1/mitm/dns/uninstall');
      toast('DNS uninstalled');
      await fetchStatus();
    } catch {
      toast.error('Failed to uninstall DNS');
    } finally {
      setActionLoading('');
    }
  }

  if (loading) {
    return (
      <div className="space-y-6">
        <div>
          <Skeleton className="h-8 w-48 mb-2" />
          <Skeleton className="h-4 w-72" />
        </div>
        <div className="grid gap-4 md:grid-cols-2">
          <Skeleton className="h-32" />
          <Skeleton className="h-32" />
        </div>
      </div>
    );
  }

  const isRunning = status?.running === true;
  const dnsInstalled = status?.dns_installed === true;

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-text">MITM Proxy</h1>
        <p className="text-sm text-muted mt-1">Intercept and inspect API traffic</p>
      </div>

      {/* Status & Controls */}
      <div className="grid gap-4 md:grid-cols-2">
        <Card>
          <div className="flex items-center justify-between mb-4">
            <h3 className="text-base font-semibold text-text">Proxy Status</h3>
            <StatusIndicator state={isRunning ? 'running' : 'stopped'} label={isRunning ? 'Running' : 'Stopped'} />
          </div>
          <div className="flex gap-2">
            <Button
              variant="primary"
              size="sm"
              onClick={startProxy}
              disabled={isRunning}
              loading={actionLoading === 'start'}
            >
              <Play className="w-3.5 h-3.5" />
              Start
            </Button>
            <Button
              variant="danger"
              size="sm"
              onClick={stopProxy}
              disabled={!isRunning}
              loading={actionLoading === 'stop'}
            >
              <Square className="w-3.5 h-3.5" />
              Stop
            </Button>
          </div>
        </Card>

        <Card>
          <div className="flex items-center justify-between mb-4">
            <h3 className="text-base font-semibold text-text">DNS Control</h3>
            <StatusIndicator state={dnsInstalled ? 'running' : 'stopped'} label={dnsInstalled ? 'Installed' : 'Not installed'} />
          </div>
          <div className="flex gap-2">
            <Button
              variant="secondary"
              size="sm"
              onClick={installDns}
              disabled={!isRunning || dnsInstalled}
              loading={actionLoading === 'dns-install'}
            >
              <Wifi className="w-3.5 h-3.5" />
              Install
            </Button>
            <Button
              variant="ghost"
              size="sm"
              onClick={uninstallDns}
              disabled={!dnsInstalled}
              loading={actionLoading === 'dns-uninstall'}
            >
              <WifiOff className="w-3.5 h-3.5" />
              Uninstall
            </Button>
          </div>
        </Card>
      </div>

      {/* Certificate */}
      <Card>
        <div className="flex items-center justify-between">
          <div>
            <h3 className="text-base font-semibold text-text">CA Certificate</h3>
            <p className="text-sm text-muted mt-1">Download and install to trust intercepted HTTPS traffic</p>
          </div>
          <CertDownload />
        </div>
      </Card>

      {/* Sessions */}
      <Card>
        <div className="flex items-center justify-between mb-4">
          <div>
            <h3 className="text-base font-semibold text-text">Active Sessions</h3>
            <p className="text-sm text-muted mt-1">{sessions.length} active connection{sessions.length !== 1 ? 's' : ''}</p>
          </div>
          <Button variant="ghost" size="sm" onClick={fetchSessions} disabled={!isRunning}>
            <Globe className="w-3.5 h-3.5" />
            Refresh
          </Button>
        </div>

        {!isRunning ? (
          <div className="text-center py-8 text-muted text-sm">
            Start the proxy to see active sessions
          </div>
        ) : sessions.length === 0 ? (
          <div className="text-center py-8 text-muted text-sm">
            No active sessions
          </div>
        ) : (
          <Table>
            <TableHead>
              <TableHeader>Client</TableHeader>
              <TableHeader>Target</TableHeader>
              <TableHeader>Started</TableHeader>
              <TableHeader>Status</TableHeader>
            </TableHead>
            <TableBody>
              {(sessions || []).map((session, i) => (
                <TableRow key={i}>
                  <TableCell className="font-mono text-xs truncate max-w-[150px]">
                    {session.client_ip || session.client || '—'}
                  </TableCell>
                  <TableCell className="font-mono text-xs truncate max-w-[200px]">
                    {session.target || session.host || '—'}
                  </TableCell>
                  <TableCell>
                    <span className="inline-flex items-center gap-1.5 text-xs text-muted">
                      <Clock className="w-3 h-3" />
                      {session.started_at ? new Date(session.started_at).toLocaleTimeString() : '—'}
                    </span>
                  </TableCell>
                  <TableCell>
                    <StatusIndicator state={session.status || 'running'} />
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
