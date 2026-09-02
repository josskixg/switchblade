import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import { getChartTheme, currentMode } from './chartTheme';

/**
 * One theme source of truth, lifted out of Layout so charts can subscribe.
 *
 * Resolution order — this MUST match the pre-paint script in index.html or the
 * page flashes the wrong theme on load:
 *
 *   stored = localStorage.sb_theme            // 'system' | 'light' | 'dark' | null
 *   realm  = path.startsWith('/admin') ? 'admin' : 'app'
 *   theme  = stored ?? (realm === 'admin' ? 'dark' : 'light')
 *   if (theme === 'system') theme = matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
 *
 * Customer portal defaults light, operator console defaults dark. Both themes
 * are fully supported in both realms — the default is a first impression, not
 * a constraint.
 */

const STORAGE_KEY = 'sb_theme';

const ThemeCtx = createContext(null);

export function realmForPath(pathname) {
  return String(pathname || '').startsWith('/admin') ? 'admin' : 'app';
}

export function readStoredTheme() {
  if (typeof localStorage === 'undefined') return null;
  const v = localStorage.getItem(STORAGE_KEY);
  return v === 'system' || v === 'light' || v === 'dark' ? v : null;
}

export function resolveMode(theme, realm) {
  let t = theme;
  if (!t) t = realm === 'admin' ? 'dark' : 'light';
  if (t === 'system') {
    if (typeof matchMedia === 'undefined') return 'light';
    return matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
  }
  return t;
}

export function ThemeProvider({ children, realm = 'app' }) {
  const [theme, setThemeState] = useState(() => readStoredTheme() || (realm === 'admin' ? 'dark' : 'light'));
  const [mode, setMode] = useState(() => resolveMode(readStoredTheme(), realm));

  // Apply to the document. Realm is a data attribute so only the three chrome
  // variables branch on it — nothing else in the system does.
  useEffect(() => {
    const next = resolveMode(theme, realm);
    setMode(next);
    const root = document.documentElement;
    root.classList.toggle('dark', next === 'dark');
    root.dataset.realm = realm;
  }, [theme, realm]);

  // Follow the OS while the user is on 'system'.
  useEffect(() => {
    if (theme !== 'system' || typeof matchMedia === 'undefined') return undefined;
    const mq = matchMedia('(prefers-color-scheme: dark)');
    const onChange = () => {
      const next = mq.matches ? 'dark' : 'light';
      setMode(next);
      document.documentElement.classList.toggle('dark', next === 'dark');
    };
    mq.addEventListener('change', onChange);
    return () => mq.removeEventListener('change', onChange);
  }, [theme]);

  const setTheme = useCallback((next) => {
    setThemeState(next);
    try {
      localStorage.setItem(STORAGE_KEY, next);
    } catch {
      /* private mode — the in-memory value still applies for this session */
    }
  }, []);

  const value = useMemo(
    () => ({ theme, setTheme, mode, realm, chart: getChartTheme(mode) }),
    [theme, setTheme, mode, realm],
  );

  return <ThemeCtx.Provider value={value}>{children}</ThemeCtx.Provider>;
}

export function useTheme() {
  const ctx = useContext(ThemeCtx);
  if (ctx) return ctx;
  // Usable without the provider: read the class the pre-paint script set.
  return { theme: null, setTheme: () => {}, mode: currentMode(), realm: 'app', chart: getChartTheme(currentMode()) };
}

/**
 * Chart palette for the active theme. Works with or without ThemeProvider —
 * without it, it watches the `dark` class on <html> directly so charts still
 * re-render when the theme toggles.
 */
export function useChartTheme() {
  const ctx = useContext(ThemeCtx);
  const [mode, setMode] = useState(() => currentMode());

  useEffect(() => {
    if (ctx) return undefined;
    const obs = new MutationObserver(() => setMode(currentMode()));
    obs.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] });
    return () => obs.disconnect();
  }, [ctx]);

  return getChartTheme(ctx ? ctx.mode : mode);
}

export default ThemeProvider;
