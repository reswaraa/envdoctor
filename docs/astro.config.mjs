// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

// Site URL: deployed to GitHub Pages at the default project URL,
// `https://reswaraa.github.io/envdoctor/`. The trailing /envdoctor/
// path comes from the `base` config below.
//
// Do not change this URL without also updating every probe's DocURL
// constant: each probe hard-codes a URL of the form
// `https://reswaraa.github.io/envdoctor/probes/<id>` that points to
// its documentation page. The doc_url lint test (internal/docslint)
// fails the build if any of those URLs 404 here.
//
// Custom domain (e.g. envdoctor.dev) is deferred.
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
      // Sidebar mirrors the URL structure: /probes/, /recipes/,
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
