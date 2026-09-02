import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Link, Outlet, useLocation, useNavigate } from 'react-router-dom';
import {
  ChevronDown, Command, LogOut, Menu, Monitor, Moon, PanelLeftClose,
  PanelLeftOpen, Search, Sun, X,
} from 'lucide-react';
import { useAuthStore } from '../../store/auth';
import { useBillingStore, bannerFor } from '../shared/billingStore';
import { canAccess, flatRoutes, isOperator, navFor, titleFor } from '../routes';
import { Banner, Meter, Money, SegmentedControl } from '../shared/ui';

const VERSION = 'Switchblade v1.0.0';

/* Theme resolution must match the pre-paint script in index.html or the page
 * flashes. Stored preference wins; otherwise the realm decides — the customer
 * portal opens light, the operator console opens dark. */
function readStoredTheme() {
  if (typeof window === 'undefined') return null;
  const v = localStorage.getItem('sb_theme');
  return v === 'system' || v === 'light' || v === 'dark' ? v : null;
}

function resolveTheme(pref, realm) {
  const t = pref || (realm === 'admin' ? 'dark' : 'light');
  if (t === 'system') {
    return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
  }
  return t;
}

function Mark({ className = '' }) {
  return (
    <svg viewBox="0 0 20 20" className={className} aria-hidden="true" fill="none">
      <rect
        x="1.25" y="1.25" width="17.5" height="17.5" rx="5.5"
        stroke="currentColor" strokeWidth="1.5" opacity="0.4"
      />
      <circle cx="10" cy="10" r="3.25" fill="currentColor" />
      <path
        d="M10 1.5v3.25M10 15.25v3.25" stroke="currentColor"
        strokeWidth="1.5" strokeLinecap="round" opacity="0.4"
      />
    </svg>
  );
}

export default function AppShell({ realm = 'app' }) {
  const location = useLocation();
  const navigate = useNavigate();

  const user = useAuthStore((s) => s.user);
  const logout = useAuthStore((s) => s.logout);
  const username = user?.username || 'account';
  const role = user?.role || 'viewer';

  const account = useBillingStore((s) => s.account);
  const block = useBillingStore((s) => s.block);
  const dismissed = useBillingStore((s) => s.dismissed);
  const dismiss = useBillingStore((s) => s.dismiss);
  const loadBilling = useBillingStore((s) => s.load);

  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [menuOpen, setMenuOpen] = useState(false);
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [themePref, setThemePref] = useState(readStoredTheme);
  const [collapsed, setCollapsed] = useState(
    () => localStorage.getItem('sb_sidebar_collapsed') === 'true'
  );

  /* Realm + theme are the only things in the system that branch on realm. */
  useEffect(() => {
    document.documentElement.dataset.realm = realm;
  }, [realm]);

  useEffect(() => {
    const apply = () => {
      const resolved = resolveTheme(themePref, realm);
      document.documentElement.classList.toggle('dark', resolved === 'dark');
    };
    apply();
    if ((themePref || '') !== 'system') return undefined;
    const mq = window.matchMedia('(prefers-color-scheme: dark)');
    mq.addEventListener('change', apply);
    return () => mq.removeEventListener('change', apply);
  }, [themePref, realm]);

  useEffect(() => {
    localStorage.setItem('sb_sidebar_collapsed', String(collapsed));
  }, [collapsed]);

  useEffect(() => {
    loadBilling();
  }, [loadBilling]);

  useEffect(() => {
    setSidebarOpen(false);
    setMenuOpen(false);
    setPaletteOpen(false);
  }, [location.pathname]);

  useEffect(() => {
    const onKey = (e) => {
      if ((e.metaKey || e.ctrlKey) && String(e.key || '').toLowerCase() === 'k') {
        e.preventDefault();
        setPaletteOpen((v) => !v);
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, []);

  function chooseTheme(next) {
    setThemePref(next);
    localStorage.setItem('sb_theme', next);
  }

  const sections = useMemo(
    () =>
      navFor(realm)
        .map((s) => ({ ...s, items: s.items.filter((i) => canAccess(role, i.roles)) }))
        .filter((s) => s.items.length > 0),
    [realm, role]
  );

  const isActive = useCallback(
    (item) =>
      item.end
        ? location.pathname === item.path
        : location.pathname === item.path || location.pathname.startsWith(`${item.path}/`),
    [location.pathname]
  );

  const crumb = titleFor(location.pathname);
  const banner = realm === 'app' ? bannerFor(account, block) : null;
  const showBanner = banner && (banner.persistent || !dismissed);
  const sub = account?.subscription;

  return (
    <div className="flex min-h-screen bg-background">
      {sidebarOpen && (
        <div
          className="fixed inset-0 z-40 lg:hidden"
          style={{ backgroundColor: 'rgb(var(--color-overlay, 14 22 28) / 0.55)' }}
          onClick={() => setSidebarOpen(false)}
          aria-hidden="true"
        />
      )}

      {/* ── Sidebar ───────────────────────────────────────────────── */}
      <aside
        className={`fixed inset-y-0 left-0 z-50 flex w-60 flex-col border-r border-border transition-[width,transform] duration-200 lg:static ${
          realm === 'admin' ? 'bg-surface-sunken' : 'bg-surface'
        } ${collapsed ? 'lg:w-16' : 'lg:w-60'} ${
          sidebarOpen ? 'translate-x-0' : '-translate-x-full lg:translate-x-0'
        }`}
      >
        <div className="flex h-16 shrink-0 items-center justify-between border-b border-border px-4">
          <Link
            to={realm === 'admin' ? '/admin' : '/app'}
            className="flex min-w-0 items-center gap-2.5 text-text"
          >
            <Mark className="h-5 w-5 shrink-0 text-primary" />
            {!collapsed && (
              <span className="truncate text-card-title tracking-[-0.02em]">Switchblade</span>
            )}
          </Link>
          <button
            type="button"
            onClick={() => setSidebarOpen(false)}
            aria-label="Close navigation"
            className="text-muted transition-colors duration-150 hover:text-text lg:hidden"
          >
            <X className="h-5 w-5" aria-hidden="true" />
          </button>
        </div>

        <nav className="flex-1 space-y-6 overflow-y-auto py-4" aria-label="Main">
          {sections.map((section) => (
            <div key={section.label}>
              {!collapsed && (
                <p className="mb-1.5 px-[22px] text-caption font-medium text-muted">
                  {section.label}
                </p>
              )}
              <ul className="space-y-0.5">
                {section.items.map((item) => {
                  const Icon = item.icon;
                  const active = isActive(item);
                  return (
                    <li key={item.path}>
                      <Link
                        to={item.path}
                        title={collapsed ? item.label : undefined}
                        style={{ minHeight: 'var(--nav-item-h, 40px)' }}
                        className={`group relative flex items-center gap-3 text-body transition-[background-color,color] duration-150 ${
                          collapsed
                            ? 'mx-auto w-10 justify-center rounded-[var(--r-sm,6px)]'
                            : 'mx-2 rounded-[var(--r-sm,6px)] pl-[14px] pr-3'
                        } ${
                          active
                            ? 'bg-primary/[0.08] text-primary dark:bg-primary/[0.12]'
                            : 'text-text-secondary hover:bg-surface-hover hover:text-text'
                        }`}
                      >
                        {/* the jack rail */}
                        {active && (
                          <span
                            aria-hidden="true"
                            className="absolute inset-y-1 left-0 w-0.5 rounded-[var(--r-full,9999px)] bg-primary"
                          >
                            <span className="absolute left-1/2 top-1/2 h-1.5 w-1.5 -translate-x-1/2 -translate-y-1/2 rounded-[var(--r-full,9999px)] bg-primary" />
                          </span>
                        )}
                        <Icon className="h-5 w-5 shrink-0" aria-hidden="true" />
                        {!collapsed && <span className="truncate">{item.label}</span>}
                      </Link>
                    </li>
                  );
                })}
              </ul>
            </div>
          ))}
        </nav>

        {/* The one number that matters, always visible. */}
        {realm === 'app' && !collapsed && account && (
          <div className="shrink-0 border-t border-border px-4 py-3">
            <Link to="/app/billing" className="block">
              <div className="flex items-baseline justify-between gap-2">
                <span className="text-caption text-muted">Balance</span>
                <Money nano={account.balance_nano} className="text-label text-text" />
              </div>
              {sub && sub.included_nano > 0 && (
                <Meter
                  className="mt-2"
                  size="sm"
                  usedNano={sub.used_nano}
                  includedNano={sub.included_nano}
                />
              )}
            </Link>
          </div>
        )}
      </aside>

      {/* ── Main column ───────────────────────────────────────────── */}
      <div className="flex min-w-0 flex-1 flex-col">
        <header className="sticky top-0 z-30 flex h-16 shrink-0 items-center justify-between gap-4 border-b border-border bg-surface px-4 lg:px-6">
          <div className="flex min-w-0 items-center gap-2">
            <button
              type="button"
              onClick={() => setSidebarOpen(true)}
              aria-label="Open navigation"
              className="text-muted transition-colors duration-150 hover:text-text lg:hidden"
            >
              <Menu className="h-5 w-5" aria-hidden="true" />
            </button>
            <button
              type="button"
              onClick={() => setCollapsed((v) => !v)}
              aria-label={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
              className="hidden rounded-[var(--r-sm,6px)] p-2 text-muted transition-[background-color,color] duration-150 hover:bg-surface-hover hover:text-text lg:inline-flex"
            >
              {collapsed ? (
                <PanelLeftOpen className="h-5 w-5" aria-hidden="true" />
              ) : (
                <PanelLeftClose className="h-5 w-5" aria-hidden="true" />
              )}
            </button>

            <nav aria-label="Breadcrumb" className="ml-1 min-w-0 truncate">
              <span className="text-caption text-muted">
                {realm === 'admin' ? 'Operator' : 'Portal'}
                {crumb.section ? ` / ${crumb.section}` : ''}
              </span>
              {crumb.label && (
                <span className="ml-1.5 text-label text-text">{crumb.label}</span>
              )}
            </nav>
          </div>

          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => setPaletteOpen(true)}
              className="hidden items-center gap-2 rounded-[var(--r-sm,6px)] border border-border bg-surface-sunken px-3 py-1.5 text-label text-muted transition-[background-color,border-color,color] duration-150 hover:border-border-strong hover:text-text sm:inline-flex"
            >
              <Search className="h-4 w-4" aria-hidden="true" />
              Search
              <kbd className="ml-2 inline-flex items-center gap-0.5 font-mono text-caption text-muted">
                <Command className="h-3 w-3" aria-hidden="true" />K
              </kbd>
            </button>

            <div className="relative">
              <button
                type="button"
                onClick={() => setMenuOpen((v) => !v)}
                aria-haspopup="menu"
                aria-expanded={menuOpen}
                className="flex items-center gap-2 rounded-[var(--r-sm,6px)] px-2 py-1.5 text-label text-text-secondary transition-[background-color,color] duration-150 hover:bg-surface-hover hover:text-text"
              >
                <span className="flex h-7 w-7 items-center justify-center rounded-[var(--r-full,9999px)] bg-primary/[0.12] text-caption font-medium text-primary">
                  {username[0].toUpperCase()}
                </span>
                <span className="hidden sm:inline">{username}</span>
                <ChevronDown className="h-4 w-4" aria-hidden="true" />
              </button>

              {menuOpen && (
                <>
                  <div className="fixed inset-0 z-30" onClick={() => setMenuOpen(false)} />
                  <div
                    role="menu"
                    className="absolute right-0 z-40 mt-2 w-64 rounded-[var(--r-lg,12px)] border border-border bg-surface-elevated p-2 shadow-2"
                  >
                    <div className="border-b border-border-subtle px-2 pb-2">
                      <p className="text-label text-text">{username}</p>
                      <p className="text-caption capitalize text-muted">{role}</p>
                    </div>

                    <div className="px-2 py-3">
                      <p className="mb-1.5 text-caption text-muted">Theme</p>
                      <SegmentedControl
                        ariaLabel="Theme"
                        size="sm"
                        className="w-full"
                        value={themePref || (realm === 'admin' ? 'dark' : 'light')}
                        onChange={chooseTheme}
                        options={[
                          { value: 'system', label: 'System', icon: Monitor },
                          { value: 'light', label: 'Light', icon: Sun },
                          { value: 'dark', label: 'Dark', icon: Moon },
                        ]}
                      />
                    </div>

                    {isOperator(role) && (
                      <button
                        type="button"
                        role="menuitem"
                        onClick={() => navigate(realm === 'admin' ? '/app' : '/admin')}
                        className="flex w-full items-center gap-2 rounded-[var(--r-sm,6px)] px-2 py-2 text-label text-text-secondary transition-[background-color,color] duration-150 hover:bg-surface-hover hover:text-text"
                      >
                        {realm === 'admin' ? 'Customer portal' : 'Operator console'}
                      </button>
                    )}

                    <button
                      type="button"
                      role="menuitem"
                      onClick={() => {
                        setMenuOpen(false);
                        logout();
                        navigate('/login');
                      }}
                      className="flex w-full items-center gap-2 rounded-[var(--r-sm,6px)] px-2 py-2 text-label text-danger transition-[background-color] duration-150 hover:bg-danger/10"
                    >
                      <LogOut className="h-3.5 w-3.5" aria-hidden="true" />
                      Sign out
                    </button>

                    <p className="px-2 pt-2 text-caption text-muted">{VERSION}</p>
                  </div>
                </>
              )}
            </div>
          </div>
        </header>

        {showBanner && (
          <Banner
            key={`${banner.tone}:${banner.title}`}
            tone={banner.tone}
            title={banner.title}
            dismissible={!banner.persistent}
            onDismiss={dismiss}
            action={{ label: banner.cta, onClick: () => navigate(banner.to) }}
          >
            {banner.body}
          </Banner>
        )}

        <main className="flex-1 overflow-auto p-4 lg:p-6">
          <div className={realm === 'app' ? 'mx-auto w-full max-w-[1200px]' : 'w-full'}>
            <Outlet />
          </div>
        </main>
      </div>

      {paletteOpen && <CommandPalette role={role} onClose={() => setPaletteOpen(false)} />}
    </div>
  );
}

/* ── Command palette ─────────────────────────────────────────────── */

function CommandPalette({ role, onClose }) {
  const navigate = useNavigate();
  const [query, setQuery] = useState('');
  const [cursor, setCursor] = useState(0);
  const inputRef = useRef(null);

  const all = useMemo(() => flatRoutes(role), [role]);
  const results = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return all.slice(0, 12);
    return all
      .filter(
        (r) =>
          r.label.toLowerCase().includes(q) ||
          r.section.toLowerCase().includes(q) ||
          r.path.toLowerCase().includes(q)
      )
      .slice(0, 12);
  }, [all, query]);

  useEffect(() => {
    inputRef.current?.focus();
  }, []);

  useEffect(() => {
    setCursor(0);
  }, [query]);

  function onKeyDown(e) {
    if (e.key === 'Escape') {
      onClose();
    } else if (e.key === 'ArrowDown') {
      e.preventDefault();
      setCursor((c) => Math.min(c + 1, results.length - 1));
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      setCursor((c) => Math.max(c - 1, 0));
    } else if (e.key === 'Enter' && results[cursor]) {
      e.preventDefault();
      navigate(results[cursor].path);
      onClose();
    }
  }

  return (
    <div
      className="fixed inset-0 z-[100] flex items-start justify-center p-4 pt-[12vh] backdrop-blur-sm"
      style={{ backgroundColor: 'rgb(var(--color-overlay, 14 22 28) / 0.55)' }}
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-label="Command palette"
        className="w-full max-w-lg overflow-hidden rounded-[var(--r-xl,16px)] border border-border-strong bg-surface-elevated shadow-3"
      >
        <div className="flex items-center gap-3 border-b border-border-subtle px-4">
          <Search className="h-4 w-4 text-muted" aria-hidden="true" />
          <input
            ref={inputRef}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={onKeyDown}
            placeholder="Go to…"
            aria-label="Search pages"
            className="w-full border-0 bg-transparent py-3.5 text-body text-text placeholder:text-muted focus:outline-none focus:ring-0"
          />
        </div>
        {results.length === 0 ? (
          <p className="px-4 py-8 text-center text-body text-muted">
            Nothing matches “{query}”.
          </p>
        ) : (
          <ul className="max-h-80 overflow-y-auto p-2">
            {results.map((r, i) => {
              const Icon = r.icon;
              return (
                <li key={`${r.realm}${r.path}`}>
                  <button
                    type="button"
                    onMouseEnter={() => setCursor(i)}
                    onClick={() => {
                      navigate(r.path);
                      onClose();
                    }}
                    className={`flex w-full items-center gap-3 rounded-[var(--r-sm,6px)] px-3 py-2 text-left text-body transition-[background-color,color] duration-150 ${
                      i === cursor ? 'bg-surface-hover text-text' : 'text-text-secondary'
                    }`}
                  >
                    <Icon className="h-4 w-4 shrink-0 text-muted" aria-hidden="true" />
                    <span className="flex-1 truncate">{r.label}</span>
                    <span className="text-caption text-muted">
                      {r.realm === 'admin' ? 'Operator' : 'Portal'} · {r.section}
                    </span>
                  </button>
                </li>
              );
            })}
          </ul>
        )}
      </div>
    </div>
  );
}
