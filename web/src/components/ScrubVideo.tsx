import { useEffect, useRef, type CSSProperties, type ReactElement, type ReactNode } from 'react';
import { useReducedMotion } from '../motion/useReducedMotion';

/* A video whose playhead is driven by scroll position instead of by time.
   Nothing plays on its own: the element is paused for its whole life and the
   only thing that moves it is where the reader is on the page.

   Two constraints shape the implementation. The asset must be encoded
   all-intra (every frame a keyframe) or a seek costs a decode from the start of
   the file — see the ffmpeg invocation in docs/DECISIONS.md ADR-018. And the
   reduced-motion path must present the same moment rather than a degraded one,
   so it renders the video's final frame as a still image and drops the scroll
   machinery entirely.

   `scrollLength` is how much scrolling the whole clip is spread across. The
   viewport inside it is sticky, so the section holds still while the playhead
   advances, then releases. */
export type ScrubVideoProps = {
  src: string;
  /* A final-frame still for the reduced-motion path. Some supplied clips do
     not include a separate still; in that case the component seeks a paused
     video to its final frame instead. */
  poster?: string;
  /* The clip's own aspect ratio, as a CSS `aspect-ratio` value. It reserves the
     right box before metadata arrives, so the page does not reflow around the
     clip, and it keeps the media from being letterboxed against a background it
     cannot match. */
  aspect?: string;
  scrollLength?: string;
  /* Smaller values let a long visual scene settle rather than chase rapid
     wheel or trackpad movement. */
  smoothing?: number;
  /* An owning scene can map the playhead to meaningful scroll landmarks (for
     example, the centres of explanatory cards) instead of its raw height. */
  getProgress?: () => number;
  className?: string;
  children?: ReactNode;
} & ({ decorative: true } | { decorative?: false; label: string; posterAlt: string });

function clamp(value: number): number {
  if (value < 0) return 0;
  if (value > 1) return 1;
  return value;
}

/* Scroll progress of the wrapper through the viewport: 0 while its top edge is
   at or below the top of the screen, 1 once its bottom edge has arrived.

   A wrapper no taller than the viewport has nothing to scrub across — the
   stacked small-screen layout, where the clip is a plain block. It resolves to
   1, not 0: a frozen first frame would show the setup of the animation and none
   of its point, while the last frame is the same still the reduced-motion path
   presents. Height 0 means layout has not happened yet, which is not the same
   thing, so that case stays at the start. */
function scrollProgress(wrapper: HTMLDivElement): number {
  const rect = wrapper.getBoundingClientRect();
  if (rect.height === 0) return 0;
  const span = rect.height - window.innerHeight;
  if (span <= 0) return 1;
  return clamp(-rect.top / span);
}

export function ScrubVideo(props: ScrubVideoProps): ReactElement {
  const {
    src,
    poster,
    aspect = '16 / 9',
    scrollLength = '240vh',
    smoothing = 0.14,
    getProgress,
    className,
    children,
  } = props;
  const reducedMotion = useReducedMotion();
  const wrapper = useRef<HTMLDivElement | null>(null);
  const video = useRef<HTMLVideoElement | null>(null);

  useEffect(() => {
    if (reducedMotion) return undefined;
    const element = wrapper.current;
    const media = video.current;
    if (element === null || media === null) return undefined;

    let frame = 0;
    let position = 0;
    let started = false;

    const step = (): void => {
      frame = window.requestAnimationFrame(step);
      const duration = media.duration;
      // Metadata may not have arrived yet, and a stream of unknown length has
      // no position to seek to.
      if (!Number.isFinite(duration) || duration <= 0) return;

      const progress = getProgress === undefined ? scrollProgress(element) : getProgress();
      const target = clamp(progress) * duration;
      // Ease toward the target so a trackpad flick reads as motion rather than
      // as a jump between two distant frames. The first frame snaps, because
      // easing up from zero would animate content the reader never scrolled past.
      position = started ? position + (target - position) * smoothing : target;
      started = true;
      // Seeks are the expensive part, so skip sub-frame corrections.
      if (Math.abs(media.currentTime - position) > 0.01) media.currentTime = position;
    };

    // Scrubbing runs only while the section is on screen. Without this the
    // rAF loop would keep seeking a decoder for a video nobody can see.
    const observer =
      typeof IntersectionObserver === 'undefined'
        ? undefined
        : new IntersectionObserver(
            (entries) => {
              for (const entry of entries) {
                if (entry.isIntersecting && frame === 0) frame = window.requestAnimationFrame(step);
                if (!entry.isIntersecting && frame !== 0) {
                  window.cancelAnimationFrame(frame);
                  frame = 0;
                }
              }
            },
            { rootMargin: '10% 0px' },
          );

    // Priming: some browsers will not paint a seek on a video that has never
    // decoded a frame, so the clip appears frozen on its poster no matter where
    // the reader scrolls. One muted play, immediately paused, gets a frame
    // decoded. It is muted, so no autoplay policy is being circumvented, and a
    // rejected promise is simply the browser declining — the scrub still works
    // wherever seeking alone is enough.
    void media.play()?.then(
      () => media.pause(),
      () => undefined,
    );
    media.pause();
    if (observer === undefined) frame = window.requestAnimationFrame(step);
    else observer.observe(element);

    return () => {
      observer?.disconnect();
      if (frame !== 0) window.cancelAnimationFrame(frame);
    };
  }, [getProgress, reducedMotion, smoothing]);

  const pauseOnFinalFrame = (media: HTMLVideoElement): void => {
    const duration = media.duration;
    if (Number.isFinite(duration) && duration > 0) media.currentTime = duration;
    media.pause();
  };

  const media = reducedMotion ? (
    poster === undefined ? (
      <video
        ref={video}
        className="scrub-media"
        src={src}
        muted
        playsInline
        preload="metadata"
        disablePictureInPicture
        aria-label={props.decorative === true ? undefined : props.label}
        aria-hidden={props.decorative === true ? true : undefined}
        tabIndex={props.decorative === true ? -1 : undefined}
        onLoadedMetadata={(event) => pauseOnFinalFrame(event.currentTarget)}
      />
    ) : (
      <img
        className="scrub-media"
        src={poster}
        alt={props.decorative === true ? '' : props.posterAlt}
        aria-hidden={props.decorative === true ? true : undefined}
      />
    )
  ) : (
    <video
      ref={video}
      className="scrub-media"
      src={src}
      poster={poster}
      muted
      playsInline
      preload="auto"
      disablePictureInPicture
      aria-label={props.decorative === true ? undefined : props.label}
      aria-hidden={props.decorative === true ? true : undefined}
      tabIndex={props.decorative === true ? -1 : undefined}
    />
  );

  return (
    <div
      ref={wrapper}
      className={className === undefined ? 'scrub' : `scrub ${className}`}
      data-motion={reducedMotion ? 'static' : 'scroll'}
      style={{ '--scrub-length': scrollLength, '--scrub-aspect': aspect } as CSSProperties}
    >
      <div className="scrub-viewport">
        {media}
        {children === undefined ? null : <div className="scrub-overlay">{children}</div>}
      </div>
    </div>
  );
}
