import { create, insert, search as zbsearch } from 'zbsearch';
import { docs, ordered } from './docs';

// The search index: one entry per page (its title and intro) and one per
// ## or ### section. The search dialog imports this module lazily, so the
// docs' text is fetched the first time someone searches.

export type SearchEntry = {
  href: string; // the page, plus #slug for a section
  page: string;
  heading: string;
  text: string;
  section: boolean;
};

const sources = import.meta.glob<string>('../../../docs/**/*.md', {
  eager: true,
  query: '?raw',
  import: 'default',
});

// Markdown to the words a reader sees: no markers, links, tags or
// container fences. Code is kept, since commands are worth finding.
function plain(md: string) {
  return md
    .replace(/^:::.*$/gm, '')
    .replace(/^```.*$/gm, '')
    .replace(/!\[[^\]]*\]\([^)]*\)/g, '')
    .replace(/\[([^\]]*)\]\([^)]*\)/g, '$1')
    .replace(/<[^>]+>/g, '')
    .replace(/^\s*\|?[\s|:-]*-[\s|:-]*$/gm, '') // a table's rule row
    .replace(/^\s*(?:[-*+]|\d+\.|>)\s+/gm, '')
    .replace(/\|/g, ' ')
    .replace(/[`*]/g, '')
    .replace(/\s+/g, ' ')
    .trim();
}

function entries(): SearchEntry[] {
  const out: SearchEntry[] = [];
  for (const [file, source] of Object.entries(sources)) {
    const name = file.replace(/^.*\/docs\//, '').replace(/\.md$/, '');
    const doc = docs.get(name === 'index' ? '' : name.replace(/\/index$/, ''));
    if (!doc) continue;

    // The compiled headings carry the slugs the page renders; match them,
    // in order, to the source's ## and ### lines outside code blocks.
    const headings = doc.headings.filter((h) => h.depth === 2 || h.depth === 3);
    const body = source.replace(/^---\n[\s\S]*?\n---\n/, '');
    const parts: { heading?: (typeof headings)[number]; lines: string[] }[] = [{ lines: [] }];
    let fenced = false;
    for (const line of body.split('\n')) {
      if (/^(```|~~~)/.test(line)) fenced = !fenced;
      if (!fenced && /^#{2,3} /.test(line)) {
        parts.push({ heading: headings[parts.length - 1], lines: [] });
        continue;
      }
      if (!fenced && /^# /.test(line)) continue;
      parts.at(-1)!.lines.push(line);
    }

    for (const part of parts) {
      out.push({
        href: part.heading ? `${doc.href}#${part.heading.slug}` : doc.href,
        page: doc.title,
        heading: part.heading?.text ?? doc.title,
        text: plain(part.lines.join('\n')) || (part.heading ? '' : doc.description),
        section: !!part.heading,
      });
    }
  }
  return out;
}

const all = entries();
const db = create({ schema: { page: 'string', heading: 'string', text: 'string' } as const });
for (const entry of all) insert(db, entry);

// The pages, in sidebar order, for an empty query.
export const pages = all
  .filter((e) => !e.section)
  .sort((a, b) => order(a.href) - order(b.href));

function order(href: string) {
  const i = ordered.findIndex((d) => d.href === href);
  return i < 0 ? Infinity : i;
}

// Entries that match every word of the query, best first: a match in a
// heading counts most, then in the page's title, then in the text.
export async function search(query: string): Promise<SearchEntry[]> {
  const results = await zbsearch(db, {
    term: query,
    properties: ['heading', 'page', 'text'],
    boost: { heading: 4, page: 1.5 },
    threshold: 0,
    limit: 40,
  });
  return results.hits.map((hit) => hit.document as unknown as SearchEntry);
}
