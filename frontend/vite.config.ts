import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

const configDir = fileURLToPath(new URL('.', import.meta.url));
const adminOutDir = path.resolve(configDir, '../internal/openvpnweb/templates/static/admin');

function removeViteHtmlShell() {
  return {
    name: 'remove-vite-html-shell',
    closeBundle() {
      fs.rmSync(path.join(adminOutDir, 'index.html'), { force: true });
    },
  };
}

// 判断是否为浏览器地址栏发起的页面请求（Accept: text/html）
// api.ts 里所有请求都带 Accept: application/json，可与页面请求区分开
function isHtmlRequest(req: { headers?: Record<string, string | string[] | undefined> }) {
  const accept = req.headers?.accept;
  if (!accept) return false;
  return String(accept).toLowerCase().includes('text/html');
}

export default defineConfig(({ command }) => ({
  plugins: [react(), removeViteHtmlShell()],
  resolve: {
    alias: {
      '@': path.resolve(configDir, './src'),
    },
    // 强制去重 React 副本：避免某个依赖把 CJS 版 react 内联进 .vite/deps，
    // 导致与项目里 ESM 版 react 形成两份不同实例，从而触发
    // "Invalid hook call. Hooks can only be called inside of the body of a function component."
    dedupe: ['react', 'react-dom'],
  },
  // 开发环境用 '/'，让 SPA 路由（/login /overview /users 等）直接在根路径下生效
  // 生产构建用 '/static/admin/'，匹配后端 StaticFS("/static", ...) 的资源路径
  base: command === 'build' ? '/static/admin/' : '/',
  server: {
    host: '127.0.0.1',
    port: 5173,
    proxy: {
      // /login：GET 页面请求走 SPA（vite 返回 index.html），POST 走后端鉴权
      '/login': {
        target: 'http://127.0.0.1:8888',
        bypass: (req) => (req.method === 'GET' && isHtmlRequest(req) ? '/index.html' : undefined),
      },
      // /logout：GET 走后端清除 session
      '/logout': {
        target: 'http://127.0.0.1:8888',
      },
      // /download：公开落地页，GET HTML 走 SPA，API 调用 /ovpn/public/* 已被 /ovpn 代理覆盖
      '/download': {
        target: 'http://127.0.0.1:8888',
        bypass: (req) => (req.method === 'GET' && isHtmlRequest(req) ? '/index.html' : undefined),
      },
      // /mfa/* 全部走后端
      '/mfa': {
        target: 'http://127.0.0.1:8888',
      },
      '/ovpn': {
        target: 'http://127.0.0.1:8888',
        // 必须显式开启 ws：/ovpn/ws/notifications 是 WebSocket 接口，否则会被卡在 connecting
        ws: true,
        // WS 路径在容器/反代下可能要求透传 Cookie
        cookieDomainRewrite: '127.0.0.1',
      },
      '/client': {
        target: 'http://127.0.0.1:8888',
        ws: true,
        // /clients 是 SPA 路由，不能被 /client 前缀匹配代理到后端
        bypass: (req) => ((req.url || '').startsWith('/clients') ? '/index.html' : undefined),
      },
      // /user/template：精确前缀匹配，避免误拦截 /users（SPA 路由）
      '/user/template': 'http://127.0.0.1:8888',
      // /settings：GET HTML 走 SPA，GET JSON/POST 走后端
      '/settings': {
        target: 'http://127.0.0.1:8888',
        bypass: (req) => (req.method === 'GET' && isHtmlRequest(req) ? '/index.html' : undefined),
      },
      '/email': 'http://127.0.0.1:8888',
      '/static/server': 'http://127.0.0.1:8888',
    },
  },
  build: {
    outDir: adminOutDir,
    emptyOutDir: true,
    assetsDir: 'assets',
    rollupOptions: {
      output: {
        // Keep the application shell stable, but split lazy routes so the optional
        // operations dashboard (Three.js, globe textures and geographic maps) is
        // never downloaded unless the feature is enabled and /screen is opened.
        codeSplitting: true,
        entryFileNames: 'assets/app.js',
        chunkFileNames: 'assets/[name]-[hash].js',
        assetFileNames: (assetInfo) => {
          if (assetInfo.name?.endsWith('.css')) {
            return 'assets/app.css';
          }

          return 'assets/[name][extname]';
        },
      },
    },
  },
}));
