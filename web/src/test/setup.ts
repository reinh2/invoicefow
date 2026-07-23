import '@testing-library/jest-dom/vitest';

document.documentElement.lang = 'en';
document.title = 'InvoiceFlow';

if (!window.matchMedia) {
  window.matchMedia = (query: string): MediaQueryList => ({
    matches: false, media: query, onchange: null,
    addEventListener: () => undefined, removeEventListener: () => undefined,
    addListener: () => undefined, removeListener: () => undefined, dispatchEvent: () => false,
  });
}
