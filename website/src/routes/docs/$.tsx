import { createFileRoute, notFound } from '@tanstack/solid-router';
import DocsView, { docHead } from '../../components/DocsView';
import { docs } from '../../lib/docs';

export const Route = createFileRoute('/docs/$')({
  loader: ({ params }) => {
    if (!docs.has(params._splat ?? '')) throw notFound();
  },
  head: ({ params }) => docHead(params._splat ?? ''),
  component: DocPage,
});

function DocPage() {
  const params = Route.useParams();
  return <DocsView slug={params()._splat ?? ''} />;
}
