import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { RootApp } from './app/RootApp';
import './styles/index.css';

const root = document.getElementById('root');

if (root === null) {
  throw new Error('InvoiceFlow could not find its application root.');
}

createRoot(root).render(
  <StrictMode>
    <RootApp />
  </StrictMode>,
);
