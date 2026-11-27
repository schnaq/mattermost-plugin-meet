# Contributing

Thanks for helping out! Please read the [Code of Conduct](CODE_OF_CONDUCT.md) first. For bigger changes, open
an issue to discuss the idea before you start.

## Prerequisites

- Go (the version in [go.mod](go.mod))
- Node.js (the version in [.nvmrc](.nvmrc)) and npm
- make and git

## Build and test

```sh
make dist          # builds server + webapp and bundles dist/com.schnaq.meet-<version>.tar.gz
make test          # Go tests (race detector) and webapp tests
make check-style   # golangci-lint, eslint and tsc
cd webapp && npm test   # webapp tests only
```

Edit `plugin.json` for the manifest and run `make apply` (also part of the build) to regenerate
`server/manifest.go` and `webapp/src/manifest.ts`; both are git-ignored.

`webapp/package.json` overrides the `eslint` and `typescript` peer ranges: eslint-plugin-react and
eslint-plugin-jsx-a11y don't list ESLint 10 yet, and the `@mattermost/*` packages and mattermost-redux still ask for
TypeScript 5. Drop an override once those packages support the versions in use.

## Demo build

The demo build replaces Google with a fake, so you can try the UI without a Workspace domain (and take
screenshots):

```sh
make dist GO_BUILD_FLAGS='-tags demo'
```

Set any non-empty *Service account JSON key* in the plugin settings. Every huddle then has an active
conference. Set `MEET_DEMO_PARTICIPANTS` in the **Mattermost server's environment** to a comma-separated list
of emails (matching `GoogleAccounts` entries) to show them as joined. Never ship a demo build.

## Local Mattermost

```sh
docker run -d --name mattermost -p 8065:8065 \
  -e MM_SERVICESETTINGS_ENABLELOCALMODE=true \
  -e MM_PLUGINSETTINGS_ENABLEUPLOADS=true \
  -e MM_SERVICESETTINGS_SITEURL=http://localhost:8065 \
  mattermost/mattermost-team-edition:release-12.1
```

Open http://localhost:8065, create an admin user, then deploy the plugin. `make deploy` uses
`build/pluginctl` and needs either the local socket or admin credentials in the environment:

```sh
export MM_SERVICESETTINGS_SITEURL=http://localhost:8065
export MM_ADMIN_USERNAME=admin MM_ADMIN_PASSWORD=...   # or MM_ADMIN_TOKEN
make deploy
```

Alternatively upload `dist/*.tar.gz` in the System Console. `make watch` rebuilds and redeploys on changes.

## Project layout

| Path | Content |
| --- | --- |
| `plugin.json` | Plugin manifest and settings schema |
| `server/` | Go plugin: `huddle/` (lifecycle, polling, rings, status), `google/` (Meet client and fake), `api/` (HTTP endpoints), `command/` (`/meet`), `kv/` (storage helpers), `demo.go` (demo build) |
| `webapp/src/` | React/TypeScript UI: `components/`, Redux state, actions and plugin registration (`index.tsx`) |
| `webapp/i18n/` | Webapp translations (`en.json`, `de.json`) |
| `build/` | Build tooling from the Mattermost plugin starter template |
| `docs/` | Google setup guide and manual testing checklist |

## Conventions

- **Tests are required** for new behaviour and bug fixes. Server logic is tested against the fake Google client.
- **i18n:** every user-facing string needs a key in both `webapp/i18n/en.json` and `webapp/i18n/de.json`; server messages live in `server/huddle/i18n.go`. Public docs and code comments are in English.
- **Commits** follow [Conventional Commits](https://www.conventionalcommits.org/) (`feat:`, `fix:`, `docs:`, ...).
- Keep `make check-style` clean; follow [webapp/AGENTS.md](webapp/AGENTS.md) for UI code.
- Update [CHANGELOG.md](CHANGELOG.md) under *Unreleased* for user-visible changes.

## Pull requests

1. Fork and create a branch from `main`.
2. Make your change with tests; run `make test` and `make check-style`.
3. Open a pull request and fill in the template. CI must pass.
4. A maintainer reviews; please address comments with follow-up commits.

Releases: maintainers run `make patch`, `make minor` or `make major` on `main`, which tags and pushes; the
`release` workflow then builds the bundle and publishes the GitHub release.
