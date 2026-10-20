import { fileURLToPath } from "node:url";
import { defineConfig, type Plugin } from "vite";
import { voidPlugin } from "void";
import { voidSolid } from "@void/solid/plugin";
import { voidMarkdown } from "@void/md/plugin";

const docsDir = fileURLToPath(new URL("../docs/", import.meta.url));

// The docs link to each other as ./page.md, which reads well on GitHub.
// Served at /docs/page, and /docs for index.md, such links would resolve
// against the wrong directory on /docs, so make them absolute.
function docsLinks(): Plugin {
  return {
    name: "kero-docs-links",
    enforce: "pre",
    transform(code, id) {
      if (!id.startsWith(docsDir) || !id.endsWith(".md")) return;
      const out = code.replace(/\]\(\.\/([\w/-]+)\.md(#[\w-]+)?\)/g, (_, page: string, hash = "") => {
        const path = page === "index" ? "/docs" : `/docs/${page.replace(/\/index$/, "")}`;
        return `](${path}${hash})`;
      });
      return { code: out, map: null };
    },
  };
}

export default defineConfig({
  plugins: [
    voidPlugin(),
    // Prefetched page data is static until the next deploy, so keep it a
    // while rather than refetching after the 30s default.
    voidSolid({ prefetch: { cacheFor: "5m" } }),
    docsLinks(),
    // Compiles the Markdown in ../docs, which pages/docs imports.
    voidMarkdown({ shiki: { themes: { light: "github-light", dark: "github-dark-dimmed" } } }),
  ],
});
