// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

// Site URL: deployed to GitHub Pages at the default project URL,
// `https://reswaraa.github.io/envdoctor/`. The trailing /envdoctor/
// path comes from the `base` config below. The URL is load-bearing
// for the Probe.DocURL contract: every probe emits
// `https://reswaraa.github.io/envdoctor/probes/<id>` and the
// doc_url lint test (internal/docslint) verifies each one resolves
// to a real page here.
//
// Custom domain (e.g. envdoctor.dev) is deferred — see the
// 2026-06-11 entry in implementation.md's decisions log.
export default defineConfig({
  site: 'https://reswaraa.github.io',
  base: '/envdoctor',
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
