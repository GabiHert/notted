# Changelog

All notable changes to Phonon are listed here, newest first. Every change a user can notice adds a
line under **Unreleased** in the same commit; `make release` moves the section under the new
version and uses it as the release notes.

## [Unreleased]

### Changed
- Notted is now Phonon. On first launch Phonon copies Notted's data (meetings, recordings, transcripts, speech models and settings) from `~/.notted` to `~/.phonon` and leaves the original and Notted.app untouched. macOS asks again for the microphone and for Screen & System Audio Recording.
- New look, Oscilloscope: graphite and phosphor amber, dark first, with a warm-grey light variant. Geist replaces the serif and Plex fonts, and Geist Mono sets timestamps, the recording timer and code.
- New app icon: five sound bars, the last one amber, on the orbit Phonon shares with Boson and Positron. The menu bar icon is the same five bars, with a dot while recording.

### Added
- Settings → Appearance: choose Oscilloscope (matching macOS), always dark or always light, or a theme of your own. The window opens in the chosen theme's colours.
- Import a VS Code colour theme (.json, comments allowed). Its editor, sidebar, button and error colours carry over, Phonon fills in the rest, and text stays readable. Export or delete your imported themes from the same place.
