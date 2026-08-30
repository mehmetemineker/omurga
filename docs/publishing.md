---
layout: default
title: Publishing this site
description: Enable and maintain the Omurga documentation site on GitHub Pages.
---

# Publishing this site

The repository contains a Jekyll-based GitHub Pages site under `docs/`. The
workflow in `.github/workflows/pages.yml` builds and deploys it automatically
when documentation changes land on `main`.

## Enable GitHub Pages

1. Open the repository on GitHub.
2. Go to **Settings → Pages**.
3. Under **Build and deployment**, set **Source** to **GitHub Actions**.
4. Push a documentation change to `main`, or run the **Documentation** workflow manually from the **Actions** tab.
5. Open the URL shown in the workflow’s deployment environment.

The workflow uses the official Pages actions and has the minimum repository
permissions required to upload and deploy the Pages artifact.

## Local preview

If Ruby and Bundler are installed, run Jekyll from the repository root:

```bash
cd docs
bundle init
bundle add jekyll
bundle exec jekyll serve --livereload
```

Open `http://127.0.0.1:4000`. The site has no external theme dependency; its
layout and styles are in `_layouts/default.html` and `assets/site.css`.

## Add a page

Create a Markdown file under `docs/` with front matter:

```markdown
---
layout: default
title: New topic
description: A short description for search engines.
---

# New topic

Content goes here.
```

Add the page to the sidebar in `_layouts/default.html` when it is a primary
topic. Use relative Markdown links between documentation pages and keep all
source text, command output, and code comments in English.

## Update the command reference

When a CLI command changes, update these pages together:

- [Command reference](commands.md) for syntax, flags, and examples.
- [Projects and environments](projects.md) for project behavior.
- [Operations](operations.md) for production procedures.
- [Troubleshooting](troubleshooting.md) for failure modes.
- `README.md` for the short repository overview.

Before pushing, verify links and inspect the generated pages in the Actions
build log. The documentation workflow runs independently from the release
workflow, so documentation changes do not create a binary release.
