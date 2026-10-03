import { defineConfig, loadEnv } from 'vite';
import vue from '@vitejs/plugin-vue';

const buildVersion = process.env.APP_BUILD_VERSION || Date.now().toString(36);

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '');
  const target = process.env.AGP_DEV_API_TARGET || env.AGP_DEV_API_TARGET || 'http://127.0.0.1:8080';

  return {
    define: {
      __APP_BUILD_VERSION__: JSON.stringify(buildVersion),
    },
    plugins: [
      vue(),
      {
        name: 'app-version-manifest',
        generateBundle() {
          this.emitFile({
            type: 'asset',
            fileName: 'version.json',
            source: `${JSON.stringify({ version: buildVersion })}\n`,
          });
        },
      },
    ],
    optimizeDeps: {
      entries: ['index.html', 'ui-review.html'],
    },
    server: {
      host: '0.0.0.0',
      port: 5173,
      proxy: {
        '/api': {
          target,
          changeOrigin: true,
          secure: false,
        },
      },
    },
    test: {
      fileParallelism: false,
    },
  };
});
