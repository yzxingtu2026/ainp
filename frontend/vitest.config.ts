import process from 'node:process';
import { fileURLToPath, URL } from 'node:url';
import { defineConfig } from 'vitest/config';
import vue from '@vitejs/plugin-vue';

const applicationEntry = process.env.AINP_ADMIN_APPLICATION || './src/composition/application.ts';

export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@admin-application': fileURLToPath(new URL(applicationEntry, import.meta.url)),
      '~': fileURLToPath(new URL('./', import.meta.url)),
      '@': fileURLToPath(new URL('./src', import.meta.url))
    }
  },
  test: {
    environment: 'happy-dom'
  }
});
