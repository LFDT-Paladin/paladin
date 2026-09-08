import react from '@vitejs/plugin-react';
import { defineConfig } from 'vite';

const mockRpcPort = process.env.MOCK_RPC_PORT ?? '31999';

const getProxyTarget = (mode: string) => {
  switch (mode) {
    case 'test':
      return `http://localhost:${mockRpcPort}`;
    case 'node3':
      return 'http://localhost:31748';
    case 'node2':
      return 'http://localhost:31648';
    default:
      return 'http://localhost:31548';
  }
};

// https://vitejs.dev/config/
export default defineConfig(({ mode }) => ({
  base: './',
  plugins: [react()],
  server: {
    port: mode === 'node3' ? 3003 : mode === 'node2' ? 3002 : 3000,
    proxy: {
      '/': {
        target: getProxyTarget(mode),
        secure: false,
        bypass: (req, _resolveConfig, _options) =>
          req.method === 'POST' ? undefined : req.url,
      },
    },
  },
}));
