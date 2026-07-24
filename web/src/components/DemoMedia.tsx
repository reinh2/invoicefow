import { useReducedMotion } from '../motion/useReducedMotion';
import type { ReactElement } from 'react';

// DemoMedia shows a short muted screen capture of the running application on the
// bundled fictional invoices (see scripts/capture-media in web/scripts and
// ADR-005). When the visitor prefers reduced motion it renders the static
// review-screen still instead of the video, so the same product moment is
// conveyed without any autoplaying motion. The still also stands in as the
// video's poster and its no-video fallback content.
export function DemoMedia(): ReactElement {
  const reducedMotion = useReducedMotion();
  const still = '/media/demo-review.png';
  const stillAlt =
    'The InvoiceFlow review screen: a fictional Meridian Office Supplies invoice on the left and its editable extracted line items on the right.';

  return (
    <section id="demo" className="demo-media" aria-labelledby="demo-title">
      <div className="section-intro">
        <p className="eyebrow">See it running</p>
        <h2 id="demo-title">A capture from the real application.</h2>
        <p className="hero-copy">
          This is the running workspace processing the fictional invoices bundled in the repository — no
          real customer data, and no paid model. Regenerate it with <code>web/scripts/capture-media.mjs</code>.
        </p>
      </div>
      <figure className="demo-media-figure">
        {reducedMotion ? (
          <img className="demo-media-frame" src={still} alt={stillAlt} width={1280} height={720} />
        ) : (
          <video
            className="demo-media-frame"
            poster="/media/demo-landing-poster.png"
            width={1280}
            height={720}
            controls
            autoPlay
            muted
            loop
            playsInline
            preload="metadata"
            aria-label="Muted screen capture of the InvoiceFlow landing page and review workspace running on fictional data"
          >
            <source src="/media/demo.webm" type="video/webm" />
            {/* Browsers without video support fall back to the static still. */}
            <img className="demo-media-frame" src={still} alt={stillAlt} width={1280} height={720} />
          </video>
        )}
        <figcaption>
          The landing page and the split review screen, captured from a Docker Compose demo seeded only with
          fictional documents.
        </figcaption>
      </figure>
    </section>
  );
}
