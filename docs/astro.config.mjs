// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

// Pinned site URL — envdoctor.dev is served via GitHub Pages with
// a CNAME (docs/public/CNAME). The URL is load-bearing for the
// Probe.DocURL contract: every probe emits
// https://envdoctor.dev/probes/<id> and the doc_url lint test
// (commit 45) verifies each one resolves to a real page here.
export default defineConfig({
  site: 'https://envdoctor.dev',
  integrations: [
    starlight({
      title: 'envdoctor',
      description:
        'EnvDoctor diagnoses why a cloned repo will not run on your machine and emits copy-pasteable repair commands.',
      social: {
        github: 'https://github.com/reswaraa/envdoctor',
      },
      // Sidebar mirrors the URL contract from
      // implementation.md Phase 8: /probes/, /recipes/,
      // /checks/, /schema/. Auto-generated within each section
      // so adding a new probe page is just dropping a file.
      sidebar: [
        {
          label: 'Getting started',
          autogenerate: { directory: 'getting-started' },
        },
        {
          label: 'Probes',
          autogenerate: { directory: 'probes' },
        },
        {
          label: 'Recipes',
          autogenerate: { directory: 'recipes' },
        },
        {
          label: 'Config (.envdoctor.yaml)',
          autogenerate: { directory: 'checks' },
        },
      ],
    }),
  ],
});
