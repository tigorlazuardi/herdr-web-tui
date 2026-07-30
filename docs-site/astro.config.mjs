// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import starlightLlmsTxt from 'starlight-llms-txt';
import mermaid from 'astro-mermaid';
import { pluginLineNumbers } from '@expressive-code/plugin-line-numbers';

export default defineConfig({
  // GitHub Pages project site: tigorlazuardi/herdr-web-tui.
  site: 'https://tigorlazuardi.github.io',
  base: '/herdr-web-tui',
  integrations: [
    // astro-mermaid MUST come BEFORE starlight in the integrations array.
    mermaid({ theme: 'neutral', autoTheme: true }),
    starlight({
      title: 'Herdr Web TUI docs',
      customCss: ['./src/styles/print.css'],
      expressiveCode: {
        plugins: [pluginLineNumbers()],
        defaultProps: { showLineNumbers: false },
      },
      components: {
        PageTitle: './src/components/PageTitle.astro',
      },
      plugins: [
        starlightLlmsTxt({
          demote: ['reports/**'],
          customSets: [
            {
              label: 'Report: Herdr protocol snapshot (2026-07)',
              description: 'Web Push event subscription rejects Herdr 0.7.5 protocol 17 snapshots.',
              paths: ['reports/2026-07-herdr-protocol-snapshot'],
            },
          ],
        }),
      ],
      sidebar: [
        { label: 'Guides', items: [{ autogenerate: { directory: 'guides' } }] },
        { label: 'Reference', items: [{ autogenerate: { directory: 'reference' } }] },
        { label: 'Reports', collapsed: true, items: [{ autogenerate: { directory: 'reports' } }] },
      ],
    }),
  ],
});
