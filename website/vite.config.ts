import { defineConfig } from 'vite';
import { tanstackStart } from '@tanstack/solid-start/plugin/vite';
import solid from 'vite-plugin-solid';
import { cloudflare } from '@cloudflare/vite-plugin';
import { markdown } from './plugins/markdown';

export default defineConfig({
  plugins: [
    cloudflare({ viteEnvironment: { name: 'ssr' } }),
    // Compiles the Markdown in ../docs, which src/lib/docs.ts imports.
    markdown(),
    tanstackStart({
      // Every page but /download is prerendered at build time; that one is
      // rendered by the Worker, so it links to the latest release.
      prerender: {
        enabled: true,
        crawlLinks: true,
        autoSubfolderIndex: false,
        filter: (page) => page.path !== '/download' && !page.path.includes('#') && !page.path.endsWith('/docs/'),
      },
    }),
    solid({ ssr: true }),
  ],
});
