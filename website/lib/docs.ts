import type { Component } from 'solid-js';
import sidebarJson from '../../docs/sidebar.json';

// The docs live in ../docs, a workspace package of their own. @void/md
// compiles each file into a component, with its frontmatter and headings.
type DocModule = {
  default: Component;
  frontmatter: { title?: string; description?: string };
  headings: ReadonlyArray<{ depth: number; slug: string; text: string }>;
};

const modules = import.meta.glob<DocModule>('../../docs/**/*.md', { eager: true });

export type Doc = {
  slug: string; // "" for the docs' index
  href: string;
  title: string;
  description: string;
  headings: DocModule['headings'];
  Content: Component;
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
    Content: mod.default,
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
