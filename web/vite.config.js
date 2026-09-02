import { defineConfig, loadEnv } from 'vite';
import react from '@vitejs/plugin-react';

/*
 * Two listeners, not one: the gateway serves /v1 on PORT and the dashboard
 * serves /api on DASHBOARD_PORT (internal/config defaults 2005 and 2006).
 * VITE_PROXY_TARGET / VITE_API_TARGET override them for a deployment that does
 * not use the defaults.
 */
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '');
  const apiTarget = env.VITE_API_TARGET || 'http://localhost:2006';
  const proxyTarget = env.VITE_PROXY_TARGET || 'http://localhost:2005';

  return {
    plugins: [react()],
    server: {
      port: 5173,
      proxy: {
        '/api': {
          target: apiTarget,
          changeOrigin: true,
        },
        '/v1': {
          target: proxyTarget,
          changeOrigin: true,
        },
      },
    },
    build: {
      outDir: 'dist',
      sourcemap: false,
    },
  };
});
