import type { Heading } from '../../plugins/markdown';
import sidebarJson from '../../../docs/sidebar.json';

// The docs live in ../docs, a workspace package of their own. plugins/markdown
// compiles each file into its HTML, frontmatter and headings.
type DocModule = {
  html: string;
  frontmatter: { title?: string; description?: string };
  headings: Heading[];
};

const modules = import.meta.glob<DocModule>('../../../docs/**/*.md', { eager: true, import: 'default' });

export type Doc = {
  slug: string; // "" for the docs' index
  href: string;
  title: string;
  description: string;
  headings: Heading[];
  html: string;
};

export const docs = new Map<string, Doc>();
for (const [file, mod] of Object.entries(modules)) {
  const name = file.replace(/^.*\/docs\//, '').replace(/\.md$/, '');
  const slug = name === 'index' ? '' : name.replace(/\/index$/, '');
  docs.set(slug, {
    slug,
    href: slug ? `/docs/${slug}` : '/docs',
    title: mod.frontmatter.title ?? mod.headings.find((h) => h.depth === 1)?.text ?? name,
    description: mod.frontmatter.description ?? '',
    headings: mod.headings,
    html: mod.html,
  });
}

export type SidebarSection = { title: string; items: Doc[] };

export const sidebar: SidebarSection[] = (sidebarJson as { title: string; items: string[] }[]).map(
  (section) => ({
    title: section.title,
    items: section.items.map((name) => {
      const doc = docs.get(name === 'index' ? '' : name);
      if (!doc) throw new Error(`docs/sidebar.json lists "${name}", but docs/${name}.md is missing`);
      return doc;
    }),
  }),
);

// The docs in sidebar order, for previous and next links.
export const ordered = sidebar.flatMap((section) => section.items);
