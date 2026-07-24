import { act, render, screen, within } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { LandingPage } from './LandingPage';

const originalMatchMedia = window.matchMedia;
const originalObserver = globalThis.IntersectionObserver;

afterEach(() => {
  window.matchMedia = originalMatchMedia;
  globalThis.IntersectionObserver = originalObserver;
});

function preferReducedMotion(reduced: boolean): void {
  window.matchMedia = ((query: string) => ({
    matches: reduced && query.includes('reduced'), media: query, onchange: null,
    addEventListener: () => undefined, removeEventListener: () => undefined,
    addListener: () => undefined, removeListener: () => undefined, dispatchEvent: () => false,
  })) as typeof window.matchMedia;
}

/* Captures the observer callback so a test can drive scroll position without a
   layout engine. */
function stubIntersectionObserver(): { fire: (target: Element) => void } {
  let callback: IntersectionObserverCallback | undefined;
  const observed: Element[] = [];
  class Stub {
    constructor(given: IntersectionObserverCallback) { callback = given; }
    observe(element: Element): void { observed.push(element); }
    disconnect(): void { observed.length = 0; }
    unobserve(): void { /* not used */ }
    takeRecords(): IntersectionObserverEntry[] { return []; }
  }
  globalThis.IntersectionObserver = Stub as unknown as typeof IntersectionObserver;
  return {
    fire: (target: Element): void => {
      act(() => callback?.([{ target, isIntersecting: true } as IntersectionObserverEntry], {} as IntersectionObserver));
    },
  };
}

describe('landing page', () => {
  it('describes shipped behavior instead of the retired foundation stage', () => {
    render(<LandingPage />);
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('a person still approves');
    expect(screen.queryByText(/not connected yet|foundation stage|contains no uploads/i)).not.toBeInTheDocument();
    expect(screen.getAllByRole('link', { name: 'Open the workspace' })[0]).toHaveAttribute('href', '/app');
  });

  it('makes no metric, customer, accuracy, or compliance claim', () => {
    render(<LandingPage />);
    expect(screen.queryByText(/\d+%|documents processed|customers|trusted by|accuracy|GDPR|SOC ?2|compliant/i)).not.toBeInTheDocument();
  });

  it('states the boundaries the system does not cross', () => {
    render(<LandingPage />);
    const limits = within(screen.getByRole('region', { name: /deliberately does not do/i }));
    expect(limits.getByText(/never pays an invoice/i)).toBeVisible();
    expect(limits.getByText(/no authentication/i)).toBeVisible();
    expect(limits.getByText(/at-least-once/i)).toBeVisible();
  });

  it('renders every provenance state as readable content, not scroll-gated content', () => {
    render(<LandingPage />);
    const story = within(screen.getByRole('region', { name: /Each state is visible/i }));
    expect(story.getAllByRole('listitem')).toHaveLength(4);
    for (const state of ['Stored', 'Extracted', 'Corrected', 'Approved']) {
      expect(story.getByText(state)).toBeVisible();
    }
    expect(story.getByText('Orchard Office Supplies')).toBeVisible();
    expect(story.getByText(/Human correction/)).toBeVisible();
  });

  it('presents the story statically and emphasizes nothing when motion is reduced', () => {
    preferReducedMotion(true);
    stubIntersectionObserver();
    render(<LandingPage />);
    const steps = within(screen.getByRole('region', { name: /Each state is visible/i })).getAllByRole('listitem');
    expect(steps[0].parentElement).toHaveAttribute('data-motion', 'static');
    for (const step of steps) expect(step).toHaveAttribute('data-active', 'false');
  });

  it('moves scroll emphasis without hiding any step', () => {
    preferReducedMotion(false);
    const observer = stubIntersectionObserver();
    render(<LandingPage />);
    const steps = within(screen.getByRole('region', { name: /Each state is visible/i })).getAllByRole('listitem');
    expect(steps[0]).toHaveAttribute('data-active', 'true');

    observer.fire(steps[2]);
    expect(steps[2]).toHaveAttribute('data-active', 'true');
    expect(steps[0]).toHaveAttribute('data-active', 'false');
    for (const step of steps) expect(step).toBeVisible();
  });

  it('renders the full story when IntersectionObserver is unavailable', () => {
    preferReducedMotion(false);
    // @ts-expect-error deliberately removing the API to exercise the fallback path
    delete globalThis.IntersectionObserver;
    render(<LandingPage />);
    const steps = within(screen.getByRole('region', { name: /Each state is visible/i })).getAllByRole('listitem');
    expect(steps).toHaveLength(4);
    for (const step of steps) expect(step).toBeVisible();
  });

  it('embeds a captured demo video from the real application with a static fallback', () => {
    preferReducedMotion(false);
    render(<LandingPage />);
    const demo = within(screen.getByRole('region', { name: /capture from the real application/i }));
    const video = demo.getByLabelText(/muted screen capture/i);
    expect(video.tagName).toBe('VIDEO');
    expect(video).toHaveAttribute('poster', '/media/demo-landing-poster.png');
    expect(video.querySelector('source')).toHaveAttribute('src', '/media/demo.webm');
    // The still doubles as the no-video fallback inside the <video> element.
    const fallback = demo.getByAltText(/Meridian Office Supplies invoice/i);
    expect(fallback).toHaveAttribute('src', '/media/demo-review.png');
  });

  it('shows the static review still instead of the video when motion is reduced', () => {
    preferReducedMotion(true);
    render(<LandingPage />);
    const demo = within(screen.getByRole('region', { name: /capture from the real application/i }));
    expect(demo.queryByLabelText(/muted screen capture/i)).not.toBeInTheDocument();
    const still = demo.getByAltText(/Meridian Office Supplies invoice/i);
    expect(still.tagName).toBe('IMG');
    expect(still).toHaveAttribute('src', '/media/demo-review.png');
  });

  it('keeps one top-level heading and labels every section', () => {
    render(<LandingPage />);
    expect(screen.getAllByRole('heading', { level: 1 })).toHaveLength(1);
    for (const region of screen.getAllByRole('region')) {
      expect(region).toHaveAccessibleName();
    }
  });
});
