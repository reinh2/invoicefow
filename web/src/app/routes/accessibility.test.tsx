import axe from 'axe-core';
import { render } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { LandingPage } from './LandingPage';

describe('landing accessibility', () => {
  it('has no detectable axe violations', async () => {
    render(<LandingPage />);
    // preload: false stops axe from trying to fetch the demo <video>/poster
    // media, which jsdom cannot load and which otherwise times out after ~10s.
    const result = await axe.run(document, { preload: false, rules: { 'color-contrast': { enabled: false } } });
    expect(result.violations).toEqual([]);
  });
});
