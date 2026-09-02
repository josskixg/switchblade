import { useCallback, useEffect, useRef, useState } from 'react';

/**
 * { data, loading, error, reload } — the shape every page in this tree uses.
 *
 * The point is that `error` is a first-class state. The pattern being replaced
 * (`client.get(...).catch(() => ({ data: {} }))`) renders a confident, false,
 * all-zero dashboard whenever the backend is down, which is a trust bug rather
 * than a loading state.
 */
export function useAsync(fn, deps = [], { skip = false } = {}) {
  const [state, setState] = useState({ data: null, loading: !skip, error: null });
  const alive = useRef(true);
  const [nonce, setNonce] = useState(0);

  // eslint-disable-next-line react-hooks/exhaustive-deps
  const run = useCallback(fn, deps);

  useEffect(() => {
    alive.current = true;
    if (skip) {
      setState({ data: null, loading: false, error: null });
      return () => {
        alive.current = false;
      };
    }
    setState((s) => ({ ...s, loading: true, error: null }));
    run()
      .then((data) => {
        if (alive.current) setState({ data, loading: false, error: null });
      })
      .catch((error) => {
        if (alive.current) setState({ data: null, loading: false, error });
      });
    return () => {
      alive.current = false;
    };
  }, [run, skip, nonce]);

  const reload = useCallback(() => setNonce((n) => n + 1), []);

  return { ...state, reload };
}

/** Client-side page slice — the ledger endpoint returns up to 200 rows at once. */
export function paginate(rows, page, perPage) {
  const list = Array.isArray(rows) ? rows : [];
  const totalPages = Math.max(1, Math.ceil(list.length / perPage));
  const safePage = Math.min(page, totalPages - 1);
  return {
    slice: list.slice(safePage * perPage, safePage * perPage + perPage),
    totalPages,
    page: safePage,
  };
}
