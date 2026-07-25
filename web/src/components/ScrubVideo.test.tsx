import { act, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ScrubVideo } from './ScrubVideo';

const originalMatchMedia = window.matchMedia;
const originalObserver = globalThis.IntersectionObserver;

afterEach(() => {
  window.matchMedia = originalMatchMedia;
  globalThis.IntersectionObserver = originalObserver;
  vi.restoreAllMocks();
});

function preferReducedMotion(reduced: boolean): void {
  window.matchMedia = (query: string) =>
    ({
      matches: reduced && query.includes('reduced'),
      media: query,
      onchange: null,
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
      addListener: () => undefined,
      removeListener: () => undefined,
      dispatchEvent: () => false,
    }) as MediaQueryList;
}

/* Captures the observer callback so a test can put the section on screen
   without a layout engine. */
function stubIntersectionObserver(): { fire: (intersecting: boolean) => void } {
  const callbacks: IntersectionObserverCallback[] = [];
  class Stub {
    constructor(given: IntersectionObserverCallback) {
      callbacks.push(given);
    }
    observe(): void {}
    disconnect(): void {}
    unobserve(): void {}
    takeRecords(): IntersectionObserverEntry[] {
      return [];
    }
  }
  globalThis.IntersectionObserver = Stub as unknown as typeof IntersectionObserver;
  return {
    fire: (intersecting: boolean): void => {
      act(() => {
        for (const callback of callbacks) {
          callback([{ isIntersecting: intersecting } as IntersectionObserverEntry], {
            disconnect: () => undefined,
          } as unknown as IntersectionObserver);
        }
      });
    },
  };
}

/* jsdom has no media pipeline and no layout, so both are supplied: a fixed
   duration and a wrapper rectangle whose top edge is `scrolled` pixels above
   the viewport. One rAF callback then runs per pump() call. */
function stubMedia(duration: number): void {
  Object.defineProperty(HTMLMediaElement.prototype, 'duration', {
    configurable: true,
    get: () => duration,
  });
  let time = 0;
  Object.defineProperty(HTMLMediaElement.prototype, 'currentTime', {
    configurable: true,
    get: () => time,
    set: (next: number) => {
      time = next;
    },
  });
  vi.spyOn(HTMLMediaElement.prototype, 'pause').mockImplementation(() => undefined);
}

function stubGeometry(wrapperHeight: number, viewportHeight: number, scrolled: number): void {
  window.innerHeight = viewportHeight;
  vi.spyOn(HTMLDivElement.prototype, 'getBoundingClientRect').mockImplementation(
    () => ({ top: -scrolled, height: wrapperHeight }) as DOMRect,
  );
}

function pump(times: number, getProgress?: () => number): HTMLVideoElement {
  const frames: FrameRequestCallback[] = [];
  vi.spyOn(window, 'requestAnimationFrame').mockImplementation((callback) => {
    frames.push(callback);
    return frames.length;
  });
  vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => undefined);
  const observer = stubIntersectionObserver();
  const { container } = render(
    <ScrubVideo
      src="/media/clip.mp4"
      poster="/media/clip.jpg"
      getProgress={getProgress}
      decorative
    />,
  );
  observer.fire(true);
  for (let index = 0; index < times; index += 1) {
    const next = frames.shift();
    if (next === undefined) break;
    act(() => next(index));
  }
  const video = container.querySelector('video');
  if (video === null) throw new Error('the scroll path must render a video element');
  return video;
}

describe('ScrubVideo', () => {
  it('never plays on its own: the element is paused, unautoplayed, and uncontrolled', () => {
    preferReducedMotion(false);
    stubIntersectionObserver();
    // jsdom implements no media pipeline, so the pause the component performs on
    // mount has to be stubbed even where the playhead is not under test.
    vi.spyOn(HTMLMediaElement.prototype, 'pause').mockImplementation(() => undefined);
    render(
      <ScrubVideo
        src="/media/clip.mp4"
        poster="/media/clip.jpg"
        label="A scroll-driven clip"
        posterAlt="The clip's final frame"
      />,
    );
    const video = screen.getByLabelText('A scroll-driven clip');
    expect(video.tagName).toBe('VIDEO');
    expect(video).toHaveAttribute('src', '/media/clip.mp4');
    expect(video).toHaveAttribute('poster', '/media/clip.jpg');
    expect(video).not.toHaveAttribute('autoplay');
    expect(video).not.toHaveAttribute('controls');
    expect(video).not.toHaveAttribute('loop');
  });

  it('shows the final frame as a still instead of the video when motion is reduced', () => {
    preferReducedMotion(true);
    render(
      <ScrubVideo
        src="/media/clip.mp4"
        poster="/media/clip.jpg"
        label="A scroll-driven clip"
        posterAlt="The clip's final frame"
      />,
    );
    expect(screen.queryByLabelText('A scroll-driven clip')).not.toBeInTheDocument();
    const still = screen.getByAltText("The clip's final frame");
    expect(still.tagName).toBe('IMG');
    expect(still).toHaveAttribute('src', '/media/clip.jpg');
  });

  it('uses a paused final video frame when a supplied clip has no separate poster', () => {
    preferReducedMotion(true);
    stubMedia(5);
    const { container } = render(<ScrubVideo src="/media/clip.mp4" decorative />);
    const still = container.querySelector('video');
    if (still === null) throw new Error('a supplied clip without a poster needs a static video');
    act(() => still.dispatchEvent(new Event('loadedmetadata')));
    expect(still).toHaveAttribute('src', '/media/clip.mp4');
    expect(still).not.toHaveAttribute('autoplay');
    expect(still.currentTime).toBe(5);
  });

  it('hides decorative media from assistive technology entirely', () => {
    preferReducedMotion(true);
    render(<ScrubVideo src="/media/clip.mp4" poster="/media/clip.jpg" decorative />);
    const still = screen.getByRole('presentation', { hidden: true });
    expect(still).toHaveAttribute('alt', '');
  });

  it('drops the scroll machinery when motion is reduced', () => {
    preferReducedMotion(true);
    const observer = vi.fn();
    globalThis.IntersectionObserver = observer;
    render(<ScrubVideo src="/media/clip.mp4" poster="/media/clip.jpg" decorative />);
    expect(observer).not.toHaveBeenCalled();
  });

  it('maps scroll position to the playhead and eases toward it', () => {
    preferReducedMotion(false);
    stubMedia(5);
    // Half of the scrubbable distance covered: 400 of (1200 - 400) * ... -> 0.5.
    stubGeometry(1200, 400, 400);
    const video = pump(3);
    // The first frame snaps to the target rather than easing up from zero.
    expect(video.currentTime).toBeCloseTo(2.5, 5);
  });

  it('accepts a scene-owned progress mapping for semantic scroll landmarks', () => {
    preferReducedMotion(false);
    stubMedia(5);
    stubGeometry(1200, 400, 0);
    const video = pump(1, () => 0.6);
    expect(video.currentTime).toBeCloseTo(3, 5);
  });

  it('clamps the playhead inside the clip past both ends of the scroll range', () => {
    preferReducedMotion(false);
    stubMedia(5);
    // Scrolled far beyond the end of the wrapper.
    stubGeometry(1200, 400, 5000);
    const video = pump(1);
    expect(video.currentTime).toBe(5);
  });

  it('still scrubs when the browser refuses the priming play', () => {
    preferReducedMotion(false);
    stubMedia(5);
    vi.spyOn(HTMLMediaElement.prototype, 'play').mockRejectedValue(new Error('blocked'));
    stubGeometry(1200, 400, 400);
    const video = pump(1);
    expect(video.currentTime).toBeCloseTo(2.5, 5);
  });

  it('rests on the final frame where there is nothing to scrub across', () => {
    preferReducedMotion(false);
    stubMedia(5);
    // The stacked small-screen layout: the wrapper is shorter than the viewport.
    stubGeometry(200, 800, 0);
    const video = pump(1);
    expect(video.currentTime).toBe(5);
  });

  it('stays at the start before layout has given the wrapper a height', () => {
    preferReducedMotion(false);
    stubMedia(5);
    stubGeometry(0, 800, 0);
    const video = pump(1);
    expect(video.currentTime).toBe(0);
  });

  it('leaves the playhead alone while the duration is unknown', () => {
    preferReducedMotion(false);
    stubMedia(Number.NaN);
    stubGeometry(1200, 400, 400);
    const video = pump(2);
    expect(video.currentTime).toBe(0);
  });
});
