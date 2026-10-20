import { defineHandler, defineHead, type InferProps } from 'void';
import { HTTPException } from 'hono/http-exception';
import { docs } from '../../lib/docs';

export const loader = defineHandler((c) => {
  const slug = c.req.param('slug') ?? '';
  const doc = docs.get(slug);
  if (!doc) throw new HTTPException(404, { message: 'No such page' });
  return { slug, title: doc.title, description: doc.description };
});

export type Props = InferProps<typeof loader>;

export const head = defineHead<Props>((_c, props) => ({
  title: props.title,
  meta: [
    { name: 'description', content: props.description },
    { property: 'og:title', content: `${props.title} | Kero` },
    { property: 'og:description', content: props.description },
  ],
}));

export function getPrerenderPaths() {
  return [...docs.keys()].filter(Boolean).map((slug) => ({ slug }));
}
