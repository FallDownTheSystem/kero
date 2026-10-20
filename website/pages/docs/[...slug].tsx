import DocsView from '../../components/DocsView';
import type { Props } from './[...slug].server';

export default function DocPage(props: Props) {
  return <DocsView slug={props.slug} />;
}
