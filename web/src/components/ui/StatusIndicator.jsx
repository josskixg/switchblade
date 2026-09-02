const stateStyles = {
  running: { dot: 'bg-success', ring: 'bg-success/30', label: 'text-success', ping: true },
  active: { dot: 'bg-success', ring: 'bg-success/30', label: 'text-success', ping: false },
  stopped: { dot: 'bg-muted', ring: 'bg-muted/20', label: 'text-muted', ping: false },
  error: { dot: 'bg-danger', ring: 'bg-danger/30', label: 'text-danger', ping: false },
  loading: { dot: 'bg-warning', ring: 'bg-warning/30', label: 'text-warning', ping: false },
  degraded: { dot: 'bg-warning', ring: 'bg-warning/30', label: 'text-warning', ping: false },
  // Hollow ring: the account exists and is healthy, it is just resting.
  cooldown: { dot: 'bg-transparent border border-muted', ring: '', label: 'text-muted', ping: false },
};

export default function StatusIndicator({ state = 'stopped', label, className = '' }) {
  const s = stateStyles[state] || stateStyles.stopped;
  return (
    <span className={`inline-flex items-center gap-2 ${s.label} ${className}`}>
      <span className="relative flex h-2.5 w-2.5 shrink-0">
        {s.ping && (
          <span
            className={`absolute inline-flex h-full w-full rounded-full ${s.ring} opacity-75 motion-safe:animate-ping`}
            aria-hidden="true"
          />
        )}
        <span className={`relative inline-flex h-2.5 w-2.5 rounded-full ${s.dot}`} aria-hidden="true" />
      </span>
      {label && <span className="text-caption font-medium capitalize">{label}</span>}
    </span>
  );
}

export { StatusIndicator };
