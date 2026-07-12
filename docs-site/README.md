# herdr-web-tui docs site

Starlight site publishing this repo's design docs (`src/content/docs/design/`)
and reference/guides (`src/content/docs/reference/`, `src/content/docs/guides/`).
Build output serves `llms.txt` / `llms-full.txt` / `llms-small.txt` for RAG.

```bash
npm install
npm run dev     # localhost:4321
npm run build   # ./dist
```

CI/hosting not yet wired — follow-up (GitHub Pages, matching `astro.config.mjs`'s
`site`/`base`).
