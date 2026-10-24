# Notted

Notted records a meeting on your Mac (your microphone and the system audio), transcribes it locally, asks your own Claude Code for a summary and task suggestions, and saves the meeting to your meetings archive in git when you say yes.

## What you need

- macOS 14 or later on Apple silicon.
- Go 1.26 (the team uses gvm).
- The Wails CLI v2.12.0: `go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0`.
- Node 24 (nvm).
- Xcode Command Line Tools (`xcode-select --install`), which provide `swiftc` 6 and the macOS SDK.
- Claude Code installed and logged in, with `claude` on your login shell's `PATH` (aliases are not followed).
- A local clone of the meetings archive repository.

## Build

Run `make build`. The app is `build/bin/Notted.app`. `make test` runs every check.

## First run

The app is not notarized, so macOS may block the first open. Open System Settings > Privacy & Security and click **Open Anyway**.

Notted asks for the microphone and for Screen & System Audio Recording when you first record. After you grant screen recording, quit and reopen Notted. If your keychain has no "Notted Local Signing" certificate, every build is signed ad hoc with a new identity, and macOS asks for both permissions again after each build. See the next section.

## Sign with your own certificate (once)

Run `bash scripts/create-signing-identity.sh` once. It creates a self-signed code-signing
certificate named "Notted Local Signing" in your login keychain, and macOS asks for your login
password so it can trust that certificate for code signing. From then on, `make build` signs
with it and prints `sign-bundle: signing with "Notted Local Signing"`, so macOS keeps the
microphone and screen recording permissions across builds and updates. If macOS asks whether
codesign may use the key, click **Always Allow**. Without the certificate, `make build` signs ad
hoc and prints a note.

The certificate stays in your keychain and signs only the builds you make yourself. To remove
it, delete "Notted Local Signing" in Keychain Access.

## Set up

The first time you open Notted, it shows **Settings > Instructions** with a short checklist.

1. **Archive folder:** click **Choose…** and pick your local clone of the meetings archive, or
   click **Clone the archive from GitHub instead**, paste its address and choose where to put it.
   Notted runs your own git, with your own login.
2. **Profile:** choose yourself from the people in the archive, or choose **New profile…** to
   create your file from a short template.
3. **Projects:** every project in the archive gets a row. Click **Choose…** to give it a root
   folder that Claude Code may read during its meetings. **New project…** creates the project's
   file and your own file for it.

Every new file opens in the editor below, ready to fill in. You can also paste a full path into
any field and click **Save settings**.

To remove a teammate's profile, click **Delete a profile…** under **Profile**. Notted lists every
file it will delete and asks first. You cannot delete the profile you are using. A project row
shows **Create my file** when your own file for that project is missing.

Notted never commits these changes: deleting and creating files only changes the archive folder,
like any edit.

## What Notted downloads

On first use per language, Notted downloads the Whisper model (about 640 MB for English `small.en`, about 640 MB for Portuguese `small`) and the Silero speech detector (0.6 MB) from the sherpa-onnx releases on GitHub (`https://github.com/k2-fsa/sherpa-onnx/releases`). It verifies each file by SHA-256 and stores it in `~/.notted/models`. This is the only network call Notted makes; the `claude` program talks to Anthropic on its own.

## Your data

Everything is under `~/.notted`: `meetings.sqlite`, `audio/<meeting-id>/` and `models/`. Audio never leaves the Mac and is never committed. The archive folder gets only `meeting.md`, `transcript.md`, `summary.md` and `suggestions.md`, and only after you say yes.

## Claude Code

Claude Code is the only model Notted uses, and it needs no API key: `ANTHROPIC_API_KEY` and `ANTHROPIC_AUTH_TOKEN` are removed before `claude` runs. The summary runs read-only (`Read Grep Glob`). In the embedded session, Claude Code's permission prompts stay on and wait for you.

## Development

- `make dev` starts the app with live reload.
- `NOTTED_HOME` moves the data folder.
- `NOTTED_AUDIOCAP` points at a helper build during `wails dev`.
- `NOTTED_MODEL_MANIFEST` replaces the model manifest.
- The app icon's source is `build/appicon.svg`; `build/appicon.png` is its 1024 px render, which Wails turns into the app's icon file.

## Notices

Notted copies code from quant (MIT, Copyright (c) 2026 Luis Brienze). `NOTICE.md` lists the copied files.
