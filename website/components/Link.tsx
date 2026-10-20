import type { ComponentProps } from 'solid-js';
import { Link as VoidLink } from '@void/solid';

// Void's Link fetches the destination's page data only on click unless told
// to prefetch, so every navigation waited on a round trip. Start it on hover,
// focus or touch instead, which is usually done by the time the click lands.
export function Link(props: ComponentProps<typeof VoidLink>) {
  return <VoidLink prefetch="hover" {...props} />;
}
