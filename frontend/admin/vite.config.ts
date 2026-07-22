import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

const configDir = fileURLToPath(new URL('.', import.meta.url));
const adminOutDir = path.resolve(configDir, '../../internal/openvpnweb/templates/static/admin');

function removeViteHtmlShell() {
  return {
    name: 'remove-vite-html-shell',
    closeBundle() {
      fs.rmSync(path.join(adminOutDir, 'index.html'), { force: true });
    },
  };
}

export default defineConfig({
  plugins: [react(), removeViteHtmlShell()],
  base: '/static/admin/',
  build: {
    outDir: adminOutDir,
    emptyOutDir: true,
    assetsDir: 'assets',
    rollupOptions: {
      output: {
        entryFileNames: 'assets/app.js',
        chunkFileNames: 'assets/[name].js',
        assetFileNames: (assetInfo) => {
          if (assetInfo.name?.endsWith('.css')) {
            return 'assets/app.css';
          }

          return 'assets/[name][extname]';
        },
      },
    },
  },
});
