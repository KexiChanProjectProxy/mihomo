# Release Workflow

Mihomo uses a GitHub Actions workflow to automate releases. Every time a tag matching `v*` is pushed, the workflow builds release binaries and publishes them to GitHub Releases.

## Trigger

The workflow fires on any tag push that matches the `v*` pattern, for example `v1.0.0` or `v2.1.0-beta`.

## Build Targets

The build matrix produces two artifacts:

| OS     | Arch | Variant |
|--------|------|---------|
| linux  | amd64 | GOAMD64 v1 |
| linux  | arm64 | |

Both builds are compiled with `CGO_ENABLED=0` (static binary) and the `with_gvisor` build tag. The resulting binary is gzip-compressed and uploaded as an artifact.

## Artifacts

Each build produces a single gzipped binary:

```
mihomo-linux-amd64-<VERSION>.gz
mihomo-linux-arm64-<VERSION>.gz
```

The workflow downloads all artifacts into a `bin/` directory and then attaches every file in that directory to the GitHub Release.

## How to Cut a Release

1. Make sure all changes intended for the release are merged into the default branch.
2. Tag the commit with a `v` prefix:

   ```shell
   git tag v1.2.3
   git push origin v1.2.3
   ```

   Replacing `v1.2.3` with the appropriate version string.

3. The `Release` workflow runs automatically.
4. When it completes, GitHub publishes a release for the tag and attaches the generated binaries automatically.

## What the Workflow Does Not Do

The workflow does not create or manage the git tag itself. Tag creation and pushing is a manual step done by a maintainer. The workflow also does not publish to package registries or deploy the documentation site.
