import React from 'react';
import ReactDOM from 'react-dom/client';
import './styles/index.css';

// Keep the server-template entry (assets/app.js) as a tiny bootstrap only.
// Route chunks must never import shared runtime code from that stable filename:
// a nested import cannot inherit its cache-busting query parameter after a
// release. Loading the application shell as a hashed chunk prevents browsers
// from combining a current entry with a cached older router runtime.
async function bootstrap() {
  const { default: App } = await import('./App');
  ReactDOM.createRoot(document.getElementById('root')!).render(
    <React.StrictMode>
      <App />
    </React.StrictMode>,
  );
}

void bootstrap();
