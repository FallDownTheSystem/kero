import { createEffect, createMemo, createSignal, For, onCleanup, onMount, Show } from 'solid-js';
import { Dynamic } from 'solid-js/web';
import { Link } from '@void/solid';
import { docs, ordered, sidebar } from '../lib/docs';
import { ArrowLeftIcon, ArrowRightIcon, MenuIcon } from './Icons';

export default function DocsView(props: { slug: string }) {
  const doc = createMemo(() => docs.get(props.slug)!);
  const index = () => ordered.indexOf(doc());
  const prev = () => ordered[index() - 1];
  const next = () => ordered[index() + 1];
  const section = () => sidebar.find((s) => s.items.includes(doc()));
  const toc = () => doc().headings.filter((h) => h.depth === 2 || h.depth === 3);
  const active = useActiveHeading(() => toc().map((h) => h.slug));

  let menu: HTMLDetailsElement | undefined;
  createEffect(() => {
    doc();
    if (menu) menu.open = false; // close the phone menu on navigation
  });

  return (
    <div class="wrapper docs">
      <aside class="docs-sidebar">
        <details class="docs-sidebar__mobile" ref={menu}>
          <summary>
            <MenuIcon size={16} /> Menu
            <span class="muted">/ {doc().title}</span>
          </summary>
          <SidebarNav current={doc().href} />
        </details>
        <div class="docs-sidebar__desktop">
          <SidebarNav current={doc().href} />
        </div>
      </aside>

      <main class="docs-main">
        <article class="docs-article">
          <Show when={section()}>{(s) => <p class="eyebrow">{s().title}</p>}</Show>
          <div class="void-md">
            <Dynamic component={doc().Content} />
          </div>
        </article>
        <nav class="docs-pager" aria-label="Pagination">
          <Show when={prev()} fallback={<span />}>
            {(p) => (
              <Link href={p().href} class="docs-pager__link">
                <span class="docs-pager__label">
                  <ArrowLeftIcon size={14} /> Previous
                </span>
                <span class="docs-pager__title">{p().title}</span>
              </Link>
            )}
          </Show>
          <Show when={next()}>
            {(n) => (
              <Link href={n().href} class="docs-pager__link docs-pager__link--next">
                <span class="docs-pager__label">
                  Next <ArrowRightIcon size={14} />
                </span>
                <span class="docs-pager__title">{n().title}</span>
              </Link>
            )}
          </Show>
        </nav>
      </main>

      <aside class="docs-toc">
        <Show when={toc().length > 0}>
          <p class="docs-toc__title">On this page</p>
          <ul>
            <For each={toc()}>
              {(h) => (
                <li classList={{ 'is-nested': h.depth === 3 }}>
                  <a href={`#${h.slug}`} classList={{ 'is-active': active() === h.slug }}>
                    {h.text}
                  </a>
                </li>
              )}
            </For>
          </ul>
        </Show>
      </aside>
    </div>
  );
}

function SidebarNav(props: { current: string }) {
  return (
    <nav class="docs-nav">
      <For each={sidebar}>
        {(section) => (
          <div class="docs-nav__section">
            <p class="docs-nav__title">{section.title}</p>
            <ul>
              <For each={section.items}>
                {(item) => (
                  <li>
                    <Link href={item.href} classList={{ 'is-active': item.href === props.current }}>
                      {item.title}
                    </Link>
                  </li>
                )}
              </For>
            </ul>
          </div>
        )}
      </For>
    </nav>
  );
}

// The heading the reader is at: the last one above the top fifth of the
// viewport.
function useActiveHeading(slugs: () => string[]) {
  const [active, setActive] = createSignal<string | null>(null);
  const update = () => {
    let current: string | null = slugs()[0] ?? null;
    for (const slug of slugs()) {
      const el = document.getElementById(slug);
      if (el && el.getBoundingClientRect().top < window.innerHeight * 0.2) current = slug;
    }
    setActive(current);
  };
  onMount(() => {
    window.addEventListener('scroll', update, { passive: true });
    onCleanup(() => window.removeEventListener('scroll', update));
  });
  createEffect(() => {
    slugs();
    if (typeof window !== 'undefined') queueMicrotask(update);
  });
  return active;
}
