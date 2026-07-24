import { useEffect, useState, type ReactElement } from 'react';
import { fetchClientConfig } from '../api/documents';

/* A publicly reachable instance has no authentication, so every visitor shares
   one workspace and can see what others uploaded. Saying so plainly is the
   honest thing to do, and it is the only difference a public deployment makes
   to the interface — the banner grants and withholds nothing.

   The notice renders only after the server confirms the flag. It is never
   assumed from the hostname, so a local demo shows nothing at all. */
export function PublicDemoNotice(): ReactElement | null {
  const [publicDemo, setPublicDemo] = useState(false);

  useEffect(() => {
    const controller = new AbortController();
    void fetchClientConfig(controller.signal)
      .then((config) => setPublicDemo(config.public_demo))
      .catch(() => {
        // A missing or failed config lookup must not invent a banner, and must
        // not break the page it sits above.
      });
    return () => controller.abort();
  }, []);

  if (!publicDemo) return null;

  return (
    <aside className="public-demo-notice" role="note">
      <strong>Shared public demo.</strong> Everyone sees the same workspace, there is no sign-in,
      and uploaded documents are erased periodically. Upload only fictional invoices — never real
      business or personal data.
    </aside>
  );
}
