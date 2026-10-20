import { defineHandler, defineHead, type InferProps } from 'void';
import mygo from '../../mygo.json';

// The version mygo.json builds, whose release the download links point at.
export const loader = defineHandler(() => ({ version: mygo.version }));

export type Props = InferProps<typeof loader>;

export const head = defineHead(() => ({
  title: 'Download',
  meta: [
    {
      name: 'description',
      content: 'Download Kero, a terminal for coding agents, for macOS, Windows and Linux.',
    },
  ],
}));
