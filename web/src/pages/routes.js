import {
  Activity, BarChart3, BookOpen, Boxes, Building2, CreditCard, Filter, Flame,
  Gauge, Globe, Image, KeyRound, Layers, ListOrdered, Network, Plug, Receipt,
  ScrollText, Server, Settings as SettingsIcon, Tags, Users, Wallet, Wrench,
} from 'lucide-react';

/**
 * One manifest. The sidebar is a projection of it and the route guards read the
 * same `roles` field, which is what prevents the previous failure where four
 * registered routes — including the whole admin console — had no link anywhere
 * and eighteen were reachable by URL regardless of role.
 *
 * Server-side RequireRole is still the authority; this only decides what the
 * client draws and lets through.
 */

export const OPERATOR_ROLES = ['owner', 'admin'];
export const ALL_ROLES = ['owner', 'admin', 'developer', 'viewer', 'customer'];

export const appNav = [
  {
    label: 'Overview',
    items: [{ path: '/app', label: 'Home', icon: Gauge, roles: ALL_ROLES, end: true }],
  },
  {
    label: 'Build',
    items: [
      { path: '/app/keys', label: 'API keys', icon: KeyRound, roles: ALL_ROLES },
      { path: '/app/models', label: 'Models & prices', icon: Layers, roles: ALL_ROLES },
      { path: '/app/docs', label: 'Docs', icon: BookOpen, roles: ALL_ROLES },
    ],
  },
  {
    label: 'Activity',
    items: [
      { path: '/app/usage', label: 'Usage', icon: BarChart3, roles: ALL_ROLES },
      { path: '/app/logs', label: 'Request log', icon: ListOrdered, roles: ALL_ROLES },
    ],
  },
  {
    label: 'Billing',
    items: [
      { path: '/app/billing', label: 'Plan & balance', icon: Wallet, roles: ALL_ROLES, end: true },
      { path: '/app/billing/ledger', label: 'Ledger', icon: ScrollText, roles: ALL_ROLES },
      { path: '/app/billing/topup', label: 'Add credit', icon: CreditCard, roles: ALL_ROLES },
    ],
  },
];

export const adminNav = [
  {
    label: 'Platform',
    items: [
      { path: '/admin', label: 'Fleet', icon: Activity, roles: OPERATOR_ROLES, end: true },
      { path: '/admin/tenants', label: 'Tenants', icon: Building2, roles: ['owner'] },
    ],
  },
  {
    label: 'Revenue',
    items: [
      { path: '/admin/billing/pricing', label: 'Rate card', icon: Tags, roles: OPERATOR_ROLES },
      { path: '/admin/billing/tenants', label: 'Tenant billing', icon: Receipt, roles: OPERATOR_ROLES },
    ],
  },
  {
    label: 'Providers',
    items: [
      { path: '/admin/providers', label: 'Catalog', icon: Plug, roles: OPERATOR_ROLES, end: true },
      { path: '/admin/providers/accounts', label: 'Accounts', icon: Users, roles: OPERATOR_ROLES },
      { path: '/admin/routing', label: 'Routing', icon: Boxes, roles: OPERATOR_ROLES },
    ],
  },
  {
    label: 'Traffic',
    items: [
      { path: '/admin/traffic', label: 'Requests', icon: ListOrdered, roles: OPERATOR_ROLES },
      { path: '/admin/usage', label: 'Usage', icon: BarChart3, roles: OPERATOR_ROLES },
      { path: '/admin/images', label: 'Image studio', icon: Image, roles: OPERATOR_ROLES },
    ],
  },
  {
    label: 'Infrastructure',
    items: [
      { path: '/admin/infra/proxy', label: 'Proxy pool', icon: Network, roles: OPERATOR_ROLES },
      { path: '/admin/infra/relay', label: 'Relay', icon: Globe, roles: OPERATOR_ROLES },
      { path: '/admin/infra/inspector', label: 'Traffic inspector', icon: Server, roles: ['owner'] },
    ],
  },
  {
    label: 'Policy',
    items: [
      { path: '/admin/policy/filters', label: 'Filter rules', icon: Filter, roles: OPERATOR_ROLES },
      { path: '/admin/policy/prompts', label: 'Prompt library', icon: Flame, roles: ['owner'] },
      { path: '/admin/resources', label: 'Payment instruments', icon: Wrench, roles: ['owner'] },
    ],
  },
  {
    label: 'System',
    items: [
      { path: '/admin/settings', label: 'Gateway settings', icon: SettingsIcon, roles: OPERATOR_ROLES },
    ],
  },
];

export function navFor(realm) {
  return realm === 'admin' ? adminNav : appNav;
}

export function canAccess(role, roles) {
  if (!roles) return true;
  return roles.includes(role);
}

export function isOperator(role) {
  return OPERATOR_ROLES.includes(role);
}

/** Where a user lands after login, driven by the same role the guards use. */
export function landingFor(role) {
  return isOperator(role) ? '/admin' : '/app';
}

/** Flat index for the command palette. */
export function flatRoutes(role) {
  const out = [];
  for (const realm of ['app', 'admin']) {
    for (const section of navFor(realm)) {
      for (const item of section.items) {
        if (!canAccess(role, item.roles)) continue;
        out.push({ ...item, realm, section: section.label });
      }
    }
  }
  return out;
}

/** Titles for breadcrumbs and the document title. */
export function titleFor(pathname) {
  for (const realm of ['app', 'admin']) {
    for (const section of navFor(realm)) {
      for (const item of section.items) {
        if (item.path === pathname) return { section: section.label, label: item.label, realm };
      }
    }
  }
  // Deepest matching prefix, so detail routes inherit their parent's label.
  let best = null;
  for (const realm of ['app', 'admin']) {
    for (const section of navFor(realm)) {
      for (const item of section.items) {
        if (pathname.startsWith(item.path) && (!best || item.path.length > best.item.path.length)) {
          best = { item, section, realm };
        }
      }
    }
  }
  return best
    ? { section: best.section.label, label: best.item.label, realm: best.realm }
    : { section: null, label: '', realm: pathname.startsWith('/admin') ? 'admin' : 'app' };
}

/** /dashboard/* bookmarks keep working. */
export const LEGACY_REDIRECTS = {
  '/dashboard': '/app',
  '/dashboard/accounts': '/admin/providers/accounts',
  '/dashboard/keys': '/app/keys',
  '/dashboard/usage': '/app/usage',
  '/dashboard/requests': '/app/logs',
  '/dashboard/references': '/app/docs',
  '/dashboard/models': '/admin/routing',
  '/dashboard/images': '/admin/images',
  '/dashboard/integration': '/admin/providers',
  '/dashboard/proxy-pool': '/admin/infra/proxy',
  '/dashboard/relay': '/admin/infra/relay',
  '/dashboard/mitm': '/admin/infra/inspector',
  '/dashboard/filters': '/admin/policy/filters',
  '/dashboard/jailbreaks': '/admin/policy/prompts',
  '/dashboard/vcc': '/admin/resources',
  '/dashboard/tools': '/admin/resources',
  '/dashboard/settings': '/admin/settings',
  '/dashboard/admin': '/admin/tenants',
  '/dashboard/tiers': '/admin/tenants',
  '/dashboard/services': '/admin',
};
