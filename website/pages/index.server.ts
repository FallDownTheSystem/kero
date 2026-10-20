import { defineHead } from 'void';

export const head = defineHead(() => ({
  title: 'A terminal for coding agents',
  meta: [
    {
      name: 'description',
      content:
        'Kero is a macOS terminal for coding agents. Sessions keep running when you quit, and the sidebar shows which ones need you.',
    },
  ],
}));
