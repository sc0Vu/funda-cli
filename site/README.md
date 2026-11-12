# funda-cli project website

Static, responsive GitHub Pages site. No Node.js, bundler, database, or runtime required.

## Live URL

After enabling GitHub Pages with **Source: GitHub Actions** in repository Settings → Pages:

https://sc0vu.github.io/funda-cli/

The `.github/workflows/pages.yml` workflow publishes the `site/` directory on pushes to `main` that change the website. You can also deploy it manually from Actions → Deploy GitHub Pages → Run workflow.

## Local preview

```sh
make wasm   # builds site/funda.wasm and site/wasm_exec.js for the live demo
python3 -m http.server 8000 --directory site
# open http://localhost:8000
```

The live demo needs `make wasm` to be run first; without it the page cannot load `funda.wasm`. The generated files are git-ignored. The Pages workflow runs `make wasm` automatically before publishing.

## Customization

- `index.html` — copy, links, sections and example output
- `styles.css` — layout, colors and responsive design
- `script.js` — Docker / Go command tabs and copy button
- `favicon.svg` — brand icon

The terminal preview contains illustrative listing data; it is not a live feed. GitHub Pages hosts only the marketing site, **not** the monitoring worker. Keep the Oracle VM and SQLite database separate. Never commit `monitor.json`, `.env`, Telegram tokens, or SQLite files into the Pages site.

If your default branch isn't `main`, update the workflow's `on.push.branches` value.
