import { renderHook } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { useReducedMotion } from './useReducedMotion';

describe('useReducedMotion', () => {
  it('reads the user motion preference', () => {
    const original = window.matchMedia;
    window.matchMedia = (query: string) => ({
      matches: query.includes('reduced'),
      media: query,
      onchange: null,
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
      addListener: () => undefined,
      removeListener: () => undefined,
      dispatchEvent: () => false,
    });
    const { result } = renderHook(() => useReducedMotion());
    expect(result.current).toBe(true);
    window.matchMedia = original;
  });
});
