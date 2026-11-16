# Changelog

All notable changes to Phonon are listed here, newest first. Every change a user can notice adds a
line under **Unreleased** in the same commit; `make release` moves the section under the new
version and uses it as the release notes.

## [Unreleased]

## [1.1.0] - 2026-11-16

### Changed
- New speech engine: whisper.cpp with Whisper large-v3-turbo (q5_0) on the Mac's GPU, for English and Portuguese, with whisper.cpp's own speech detector. It transcribes about 6 times faster, needs about 0.9 GB of memory instead of 2.4 to 3.2 GB, and writes whole sentences instead of fragments. Each language is forced from the meeting, and each 30 s part is transcribed whole. The meeting page offers to download the new models once (about 575 MB, from Hugging Face). Earlier transcripts and the old models stay as they are.
- Phonon uses far less memory. The speech model now runs in a separate process that stops once a recording is transcribed, so Phonon drops back to about 50 MB instead of holding 1 to 5 GB for 10 minutes or more. A long transcription no longer grows without limit: the model process is replaced if it passes 3 GB.

### Added
- A live wave while recording: your microphone (amber) and the others' audio as two oscilloscope traces under the meeting's title, driven by the real input levels. It pauses, and the app stops sending levels, while the window is hidden or minimised; with Reduce Motion on it stands still and updates twice a second.
- Settings → Unused speech models lists the models earlier versions downloaded that nothing uses now, with their sizes, and removes one after you confirm. Phonon never removes them on its own.
- **Re-transcribe** on a finished meeting's page transcribes its audio again with the current model. The page shows part n of N; the old transcript stays until the new one is complete, then the new one replaces it. If it fails or Phonon quits first, the old transcript is kept. The summary and suggestions are not changed.
- For measuring: `PHONON_DEBUG_ADDR=127.0.0.1:6061` serves Go profiles and recording start and stop on that loopback address. It is off unless set.

## [1.0.0] - 2026-11-11

### Changed
- Phonon now updates from GitHub Releases instead of pulling and building its source clone, so Settings no longer asks for a Phonon source folder. The Updates card checks for a new release at launch and every 6 hours; **Update Phonon** downloads it, checks its SHA-256 and code signature, swaps the app and reopens it, and puts the previous version back if the new one does not start. It waits while a recording runs.
- Notted is now Phonon. On first launch Phonon copies Notted's data (meetings, recordings, transcripts, speech models and settings) from `~/.notted` to `~/.phonon` and leaves the original and Notted.app untouched. macOS asks again for the microphone and for Screen & System Audio Recording.
- Notted's **Update Notted** button now lands on Phonon. The update replaces Notted.app with Phonon.app in the same folder and opens it; Phonon then copies Notted's data as above. If Phonon cannot be installed, Notted.app stays and says so, and opening it again retries.
- New look, Oscilloscope: graphite and phosphor amber, dark first, with a warm-grey light variant. Geist replaces the serif and Plex fonts, and Geist Mono sets timestamps, the recording timer and code.
- New app icon: five sound bars, the last one amber, on the orbit Phonon shares with Boson and Positron. The menu bar icon is the same five bars, with a dot while recording.

### Added
- Settings shows the running version beside its title.
- Settings → Appearance: choose Oscilloscope (matching macOS), always dark or always light, or a theme of your own. The window opens in the chosen theme's colours.
- Import a VS Code colour theme (.json, comments allowed). Its editor, sidebar, button and error colours carry over, Phonon fills in the rest, and text stays readable. Export or delete your imported themes from the same place.
