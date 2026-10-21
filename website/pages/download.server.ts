import { defineHandler, defineHead, type InferProps } from 'void';
import { GITHUB_URL } from '../components/SiteHeader';

// The latest release changes only when one is published, so the page can be
// a few minutes behind it.
export const revalidate = 300;

// The latest published release, whose assets the download links point at:
// mygo.json's version may be a draft still building, or not released at all.
// GitHub redirects releases/latest to its tag, which, unlike the API, has no
// rate limit for the Worker's shared IPs to run into.
export const loader = defineHandler(async () => {
  const res = await fetch(`${GITHUB_URL}/releases/latest`, { redirect: 'manual' });
  const tag = res.headers.get('location')?.match(/\/releases\/tag\/v([^/?#]+)$/)?.[1];
  if (!tag) throw new Error(`No latest release: ${res.status} ${res.headers.get('location') ?? ''}`);
  return { version: tag };
});

export type Props = InferProps<typeof loader>;

export const head = defineHead(() => ({
  title: 'Download',
  meta: [
    {
      name: 'description',
      content: 'Download Kero, a terminal for coding agents, for macOS, Windows and Linux.',
    },
  ],
}));
