# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.0.0] - 2026-10-07

### Added

- Tray icon showing the current UTC time: hour (24h) on top, minute below.
- Right-click menu with a single **Exit** item.
- Tooltip with the UTC time and date.
- Automatic start at logon via the per-user `Run` registry key; `-remove-autostart` removes it.
- Digits follow the taskbar theme (white on dark, black on light) and are redrawn at the taskbar's DPI.
- Single-instance guard; the icon is restored automatically after Explorer restarts.
- `-version` flag and version information in the executable's file properties.

[Unreleased]: https://github.com/TomLadek/utc-time-widget/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/TomLadek/utc-time-widget/releases/tag/v1.0.0
