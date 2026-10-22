import { createFileRoute } from '@tanstack/solid-router';
import DocsView, { docHead } from '../../components/DocsView';

export const Route = createFileRoute('/docs/')({
  head: () => docHead(''),
  component: () => <DocsView slug="" />,
});
