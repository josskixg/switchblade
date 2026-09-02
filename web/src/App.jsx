import { useEffect } from 'react';
import { BrowserRouter, Routes, Route, Navigate, Outlet, useLocation } from 'react-router-dom';

import LoginPage from './pages/LoginPage';
import AppShell from './pages/dashboard/Layout';
import { useAuthStore } from './store/auth';
import { ToastProvider } from './components/ui/Toast';
import { LEGACY_REDIRECTS, OPERATOR_ROLES, landingFor } from './pages/routes';

/* Customer portal */
import Home from './pages/app/Home';
import Billing from './pages/app/Billing';
import Ledger from './pages/app/Ledger';
import TopUp from './pages/app/TopUp';
import CustomerModels from './pages/app/Models';

/* Operator console */
import Fleet from './pages/admin/Fleet';
import Pricing from './pages/admin/Pricing';
import TenantBilling from './pages/admin/TenantBilling';

/* Pages carried over from the flat dashboard. Each now belongs to exactly one
 * realm, except Usage and Requests which have a tenant view and a fleet view. */
import Keys from './pages/dashboard/Keys';
import Usage from './pages/dashboard/Usage';
import Requests from './pages/dashboard/Requests';
import References from './pages/dashboard/References';
import Accounts from './pages/dashboard/Accounts';
import AdminPanel from './pages/dashboard/AdminPanel';
import Models from './pages/dashboard/Models';
import ImageStudio from './pages/dashboard/ImageStudio';
import Integration from './pages/dashboard/Integration';
import ProxyPool from './pages/dashboard/ProxyPool';
import Relay from './pages/dashboard/Relay';
import MITM from './pages/dashboard/MITM';
import VCC from './pages/dashboard/VCC';
import Filters from './pages/dashboard/Filters';
import Jailbreaks from './pages/dashboard/Jailbreaks';
import Settings from './pages/dashboard/Settings';

/*
 * Realm + theme are resolved once at module load, before React's first render,
 * so a customer landing on /app does not flash the operator console's dark
 * default. index.html's pre-paint script forces `.dark` unconditionally; this
 * corrects it as early as the bundle can.
 */
(function bootstrapRealm() {
  if (typeof document === 'undefined') return;
  const realm = window.location.pathname.startsWith('/admin') ? 'admin' : 'app';
  document.documentElement.dataset.realm = realm;

  const stored = localStorage.getItem('sb_theme');
  const pref = stored === 'system' || stored === 'light' || stored === 'dark' ? stored : null;
  let theme = pref || (realm === 'admin' ? 'dark' : 'light');
  if (theme === 'system') {
    theme = window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
  }
  document.documentElement.classList.toggle('dark', theme === 'dark');
})();

function ProtectedRoute() {
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated);
  return isAuthenticated ? <Outlet /> : <Navigate to="/login" replace />;
}

function PublicRoute() {
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated);
  const role = useAuthStore((s) => s.user?.role) || 'viewer';
  return isAuthenticated ? <Navigate to={landingFor(role)} replace /> : <Outlet />;
}

/**
 * The client-side half of the realm split. The server enforces it with role
 * middleware on every /api group; this only decides what gets rendered, and it
 * reads the same `roles` lists the sidebar is generated from.
 */
function RequireRole({ roles, children }) {
  const role = useAuthStore((s) => s.user?.role) || 'viewer';
  if (!roles.includes(role)) return <Navigate to="/app" replace />;
  return children;
}

function Landing() {
  const role = useAuthStore((s) => s.user?.role) || 'viewer';
  return <Navigate to={landingFor(role)} replace />;
}

/** Existing /dashboard/* bookmarks keep working. */
function LegacyRedirect() {
  const { pathname, search } = useLocation();
  const role = useAuthStore((s) => s.user?.role) || 'viewer';
  const clean = pathname.replace(/\/+$/, '') || '/dashboard';
  if (clean === '/dashboard') return <Navigate to={landingFor(role)} replace />;
  const target = LEGACY_REDIRECTS[clean];
  return <Navigate to={target ? `${target}${search}` : landingFor(role)} replace />;
}

export default function App() {
  const checkAuth = useAuthStore((s) => s.checkAuth);

  useEffect(() => {
    checkAuth();
  }, [checkAuth]);

  return (
    <ToastProvider>
      <BrowserRouter>
        <Routes>
          <Route element={<PublicRoute />}>
            <Route path="/login" element={<LoginPage />} />
          </Route>

          <Route element={<ProtectedRoute />}>
            {/* ── Customer portal ─────────────────────────────── */}
            <Route path="/app" element={<AppShell realm="app" />}>
              <Route index element={<Home />} />
              <Route path="keys" element={<Keys />} />
              <Route path="models" element={<CustomerModels />} />
              <Route path="docs" element={<References />} />
              <Route path="usage" element={<Usage />} />
              <Route path="logs" element={<Requests />} />
              <Route path="billing" element={<Billing />} />
              <Route path="billing/ledger" element={<Ledger />} />
              <Route path="billing/topup" element={<TopUp />} />
              <Route path="*" element={<Navigate to="/app" replace />} />
            </Route>

            {/* ── Operator console ────────────────────────────── */}
            <Route
              path="/admin"
              element={
                <RequireRole roles={OPERATOR_ROLES}>
                  <AppShell realm="admin" />
                </RequireRole>
              }
            >
              <Route index element={<Fleet />} />
              <Route
                path="tenants"
                element={
                  <RequireRole roles={['owner']}>
                    <AdminPanel />
                  </RequireRole>
                }
              />
              <Route path="billing/pricing" element={<Pricing />} />
              <Route path="billing/tenants" element={<TenantBilling />} />
              <Route path="providers" element={<Integration />} />
              <Route path="providers/accounts" element={<Accounts />} />
              <Route path="routing" element={<Models />} />
              <Route path="traffic" element={<Requests />} />
              <Route path="usage" element={<Usage />} />
              <Route path="images" element={<ImageStudio />} />
              <Route path="infra/proxy" element={<ProxyPool />} />
              <Route path="infra/relay" element={<Relay />} />
              <Route
                path="infra/inspector"
                element={
                  <RequireRole roles={['owner']}>
                    <MITM />
                  </RequireRole>
                }
              />
              <Route path="policy/filters" element={<Filters />} />
              <Route
                path="policy/prompts"
                element={
                  <RequireRole roles={['owner']}>
                    <Jailbreaks />
                  </RequireRole>
                }
              />
              <Route
                path="resources"
                element={
                  <RequireRole roles={['owner']}>
                    <VCC />
                  </RequireRole>
                }
              />
              <Route path="settings" element={<Settings />} />
              <Route path="*" element={<Navigate to="/admin" replace />} />
            </Route>

            <Route path="/dashboard/*" element={<LegacyRedirect />} />
            <Route path="/dashboard" element={<LegacyRedirect />} />
            <Route path="/" element={<Landing />} />
          </Route>

          <Route path="*" element={<Navigate to="/login" replace />} />
        </Routes>
      </BrowserRouter>
    </ToastProvider>
  );
}
