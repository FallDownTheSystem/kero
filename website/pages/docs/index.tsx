import DocsView from '../../components/DocsView';
import type { Props } from './index.server';

export default function DocsIndexPage(props: Props) {
  return <DocsView slug={props.slug} />;
}
