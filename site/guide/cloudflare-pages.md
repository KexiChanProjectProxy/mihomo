# Cloudflare Pages

The mihomo documentation site is built with VitePress and can be hosted on Cloudflare Pages. The setup below describes the required configuration for a Git-integrated Pages deployment connected to this repository.

## Cloudflare Dashboard Settings

When creating a new Pages project in the Cloudflare dashboard and connecting it to the `MetaCubeX/mihomo` repository, set the following:

| Field | Value |
|-------|-------|
| Production branch | Your chosen docs publishing branch |
| Root directory | `site` |
| Build command | `npm ci && npm run docs:build` |
| Build output directory | `.vitepress/dist` |

If you use the **Zero Config** option, Cloudflare Pages will attempt to detect the framework automatically. Verify that the settings above are applied, as auto-detection may not always select the correct root directory or build command.

## Build Command and Output

The docs site lives in the `site/` directory of the repository. VitePress builds to `.vitepress/dist/` relative to that root directory.

```
Repository root (project)
└── site/
    └── .vitepress/
        └── dist/   ← output directory
```

Set the build command to:

```shell
npm ci && npm run docs:build
```

And the output directory to:

```
.vitepress/dist
```

## Using wrangler.toml for Configuration

If you prefer to codify Pages settings in a `wrangler.toml` file rather than using the dashboard, the equivalent configuration is:

```toml
name = "mihomo-docs"
compatibility_date = "2024-01-01"
pages_build_output_dir = "site/.vitepress/dist"
```

Place `wrangler.toml` in the repository root. Cloudflare Pages will automatically pick it up when connected to the repository. The `pages_build_output_dir` key tells Pages where VitePress places its built files when building from the repository root.

Note that `wrangler.toml` does not replace the dashboard build command. You still need to configure the Pages project to run the same install and build steps shown above.

## Environment Variables

If the build requires environment variables (for example, to pull in a private dependency or a different VitePress theme), add them under **Settings > Environment variables** in the Pages dashboard. Mark them as **Encrypt** if they contain secrets.

## Preview Deployments

Cloudflare Pages automatically deploys a preview for every pull request opened against the connected branch. The preview URL is shown in the PR check output. Preview deployments use the same build settings as the production branch unless overridden in the dashboard.
