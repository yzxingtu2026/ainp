import process from 'node:process';
import { readFileSync } from 'node:fs';
import { URL, fileURLToPath } from 'node:url';
import { defineConfig, loadEnv } from 'vite';
import { setupVitePlugins } from './build/plugins';
import { createViteProxy, getBuildTime } from './build/config';

export default defineConfig(configEnv => {
  const viteEnv = loadEnv(configEnv.mode, process.cwd()) as unknown as Env.ImportMeta;

  const buildTime = getBuildTime();

  const enableProxy = configEnv.command === 'serve' && !configEnv.isPreview;
  const applicationEntry = process.env.AINP_ADMIN_APPLICATION || './src/composition/application.ts';

  return {
    base: viteEnv.VITE_BASE_URL,
    resolve: {
      alias: {
        '@admin-application': fileURLToPath(new URL(applicationEntry, import.meta.url)),
        '~': fileURLToPath(new URL('./', import.meta.url)),
        '@': fileURLToPath(new URL('./src', import.meta.url))
      }
    },
    css: {
      preprocessorOptions: {
        scss: {
          api: 'modern-compiler',
          additionalData: `@use "@/styles/scss/global.scss" as *;`
        }
      }
    },
    plugins: [
      ...setupVitePlugins(viteEnv, buildTime),
      {
        name: 'ainp-admin-composition-manifest',
        generateBundle() {
          this.emitFile({
            type: 'asset',
            fileName: 'admin-composition.json',
            source: JSON.stringify(
              {
                ...JSON.parse(readFileSync(fileURLToPath(new URL('./admin-baseline.json', import.meta.url)), 'utf8')),
                modules: (process.env.AINP_ADMIN_MODULES || 'core,system,operations,ai,demo').split(',').sort()
              },
              null,
              2
            )
          });
        }
      }
    ],
    define: {
      BUILD_TIME: JSON.stringify(buildTime)
    },
    server: {
      host: '0.0.0.0',
      port: 9527,
      open: true,
      proxy: createViteProxy(viteEnv, enableProxy)
    },
    preview: {
      port: 9725
    },
    build: {
      reportCompressedSize: false,
      sourcemap: viteEnv.VITE_SOURCE_MAP === 'Y',
      commonjsOptions: {
        ignoreTryCatch: false
      }
    }
  };
});
