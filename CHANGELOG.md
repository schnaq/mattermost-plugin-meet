# Changelog

All notable changes to this project are documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.2.0] - 2026-11-27

### Changed

- Requires the React 19 runtime that Mattermost 12.1 ships; JSX now uses the host's JSX runtime.
- Toolchain: Go 1.27, Node 26 LTS, Babel 8, ESLint 10 (flat config), TypeScript 6, webpack-cli 7.

### Fixed

- The plugin bundle no longer injects global polyfills into the Mattermost page.

## [0.1.0] - 2026-11-27

Initial public release.

### Added

- One-click huddles from the channel header, from DMs and group DMs, from the profile popover and with the `/meet` slash command.
- Ringing for DMs, group DMs and invitations: incoming call dialog with ringtone, Accept, Decline and quick replies; answering on one device stops the ring everywhere; missed calls are reported; Do Not Disturb suppresses ringing.
- Live participant avatars, a sidebar indicator, a channel banner and a floating widget while you are in a huddle.
- Custom status while in a huddle; the previous status is restored afterwards.
- Invite channel members to a running huddle; end a huddle for everyone (the person who started it).
- Summary card with duration and participants after the end, plus links to recordings, transcripts and notes once Google provides them.
- Automatic end of abandoned huddles, and the Meet space is restricted afterwards so the old link cannot reopen a call.
- Participant tracking via the Google Meet REST API (polling), mapped to Mattermost users through the `GoogleAccounts` setting.
- Per-user settings for ring sound and ring dialog.
- English and German translations.
- Mobile fallback: the huddle card is a post attachment with a Join button.
- Demo build (`-tags demo`) with a fake Google backend for local development.

[Unreleased]: https://github.com/schnaq/mattermost-plugin-meet/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/schnaq/mattermost-plugin-meet/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/schnaq/mattermost-plugin-meet/releases/tag/v0.1.0
