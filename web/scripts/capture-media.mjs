// Capture the landing-page demo video, its poster, and a review-screen still
// from a *running* InvoiceFlow demo, using only the bundled fictional fixtures.
//
// This is the ADR-005 media-regeneration tool. It does not build or seed the
// app; it drives whatever is already running and seeded. Full regeneration:
//
//   COMPOSE_PROJECT_NAME=invoiceflow-media API_HOST_PORT=18081 \
//     POSTGRES_HOST_PORT=15433 RECEIVER_HOST_PORT=18091 \
//     docker compose up --build --wait
//   API_HOST_PORT=18081 sh scripts/demo-seed.sh      # prints two document ids
//   BASE_URL=http://127.0.0.1:18081 REVIEW_DOC_ID=<needs_review id> \
//     npm --prefix web run capture:media
//
// Requires the Playwright Chromium browser once: `npm --prefix web exec -- \
// playwright install chromium`.
//
// Outputs (committed) land in web/public/media/ so Vite serves them from the
// bundle root:
//   demo.webm                 muted screen capture, 1280x720, target < 2.5 MB
//   demo-landing-poster.png   the video poster (landing hero, first frame)
//   demo-review.png           static product still + no-video / reduced-motion
//                             fallback (the seeded review screen)
//
// Every pixel comes from the real application running on invented data.

import { chromium } from '@playwright/test';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { mkdir, rename, readdir, rm } from 'node:fs/promises';

const baseURL = process.env.BASE_URL ?? 'http://127.0.0.1:18081';
const reviewDocID = process.env.REVIEW_DOC_ID;
if (!reviewDocID) {
  console.error('REVIEW_DOC_ID is required (the needs_review id printed by scripts/demo-seed.sh)');
  process.exit(1);
}

const here = dirname(fileURLToPath(import.meta.url));
const outDir = join(here, '..', 'public', 'media');
const videoTmp = join(outDir, '.video-tmp');

const viewport = { width: 1280, height: 720 };

async function smoothScrollToBottom(page, durationMs) {
  await page.evaluate(async (duration) => {
    const distance = document.body.scrollHeight - window.innerHeight;
    const start = performance.now();
    await new Promise((resolve) => {
      function step(now) {
        const t = Math.min(1, (now - start) / duration);
        // easeInOutQuad for a calm, non-jerky pan.
        const eased = t < 0.5 ? 2 * t * t : 1 - Math.pow(-2 * t + 2, 2) / 2;
        window.scrollTo(0, Math.round(distance * eased));
        if (t < 1) requestAnimationFrame(step);
        else resolve();
      }
      requestAnimationFrame(step);
    });
  }, durationMs);
}

async function main() {
  await mkdir(outDir, { recursive: true });
  await rm(videoTmp, { recursive: true, force: true });

  const browser = await chromium.launch();

  // 1) Video + poster: the landing page, panned top to bottom.
  const videoContext = await browser.newContext({
    viewport,
    recordVideo: { dir: videoTmp, size: viewport },
    reducedMotion: 'no-preference',
  });
  const landing = await videoContext.newPage();
  await landing.goto(baseURL, { waitUntil: 'networkidle' });
  await landing.locator('section.hero').waitFor({ state: 'visible' });
  await landing.waitForTimeout(1200); // hold on the hero as the poster frame
  await landing.screenshot({ path: join(outDir, 'demo-landing-poster.png') });
  await smoothScrollToBottom(landing, 7000);
  await landing.waitForTimeout(1200); // rest at the bottom
  const video = landing.video();
  await videoContext.close(); // finalizes the webm
  if (video) {
    await rename(await video.path(), join(outDir, 'demo.webm'));
  }
  await rm(videoTmp, { recursive: true, force: true });

  // 2) Review-screen still at 2x for a crisp static fallback.
  const shotContext = await browser.newContext({
    viewport,
    deviceScaleFactor: 2,
  });
  const review = await shotContext.newPage();
  await review.goto(`${baseURL}/app/documents/${reviewDocID}`, { waitUntil: 'networkidle' });
  await review.locator('section.review-workspace').waitFor({ state: 'visible' });
  // The source panel renders an <img> for image documents. Wait until it has
  // actually decoded, or the shot catches an empty panel. (PDF sources render in
  // an <object> the headless shell cannot paint, so this script expects an
  // image document for a legible still.)
  await review.locator('img.source-image').waitFor({ state: 'visible' });
  await review.waitForFunction(() => {
    const img = document.querySelector('img.source-image');
    return img instanceof HTMLImageElement && img.complete && img.naturalWidth > 0;
  }, null, { timeout: 15000 });
  // object-fit: contain centres the tall invoice inside a panel stretched to the
  // full height of the (taller) proposal column, so the drawn image sits well
  // below the fold. Scroll it into frame before the shot.
  await review.evaluate(() => {
    const img = document.querySelector('img.source-image');
    const rect = img.getBoundingClientRect();
    const boxTop = rect.top + window.scrollY;
    const scale = Math.min(rect.width / img.naturalWidth, rect.height / img.naturalHeight);
    const drawnTop = boxTop + (rect.height - img.naturalHeight * scale) / 2;
    window.scrollTo(0, Math.max(0, Math.round(drawnTop - 96)));
  });
  await review.waitForTimeout(500);
  await review.screenshot({ path: join(outDir, 'demo-review.png') });
  await shotContext.close();

  await browser.close();

  const files = await readdir(outDir);
  console.log('wrote', files.filter((f) => !f.startsWith('.')).sort().join(', '), 'to web/public/media/');
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
