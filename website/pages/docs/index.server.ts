import { defineHandler, defineHead, type InferProps } from 'void';
import { docs } from '../../lib/docs';

export const loader = defineHandler(() => {
  const doc = docs.get('')!;
  return { slug: '', title: doc.title, description: doc.description };
});

export type Props = InferProps<typeof loader>;

export const head = defineHead<Props>((_c, props) => ({
  title: props.title,
  meta: [{ name: 'description', content: props.description }],
}));
