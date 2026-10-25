# Notted

Notted records a meeting on your Mac (your microphone and the system audio), transcribes it locally, asks your own Claude Code for a summary and task suggestions, and saves the meeting to your meetings archive in git when you say yes.

## Install

```
git clone <your Notted repository address>
cd notted
make install
```

`make install` checks your Mac and the tools below, and tells you how to fix anything missing before it changes anything. If your keychain has no "Notted Local Signing" certificate, it creates one, and macOS asks for your login password once so it can trust the certificate for signing Notted. It then builds Notted, puts it in `/Applications`, and opens it. **Update Notted** then uses this clone as its source folder, until you choose another one in **Settings**.

### What you need

`make install` checks this list and stops with a fix for each missing item. A missing Claude Code only gives a warning, and the archive folder is not checked.

- macOS 14 or later on Apple silicon.
- Go 1.26 or later.
- The Wails CLI v2.12.0: `go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0`.
- Node 20.19 or later (Node 24 recommended).
- Xcode Command Line Tools (`xcode-select --install`), which provide `swiftc` 6 and the macOS SDK.
- Claude Code installed and logged in, with `claude` on your login shell's `PATH` (aliases are not followed).
- A folder for your meetings archive: an empty folder, or a clone of your team's archive. Notted sets it up the first time you choose it.

## Build

`make build` is for development: it builds `build/bin/Notted.app` and installs nothing. `make test` runs every check.

## First run

The app is not notarized, so macOS may block the first open. Open System Settings > Privacy & Security and click **Open Anyway**.

Notted asks for the microphone and for Screen & System Audio Recording when you first record. After you grant screen recording, quit and reopen Notted. If your keychain has no "Notted Local Signing" certificate, every build is signed ad hoc with a new identity, and macOS asks for both permissions again after each build. See the next section.

## Menu bar

Notted keeps an icon in the menu bar for as long as it runs. Its menu starts and stops a recording (with the default title and the language of your latest meeting), opens the window, and quits. Closing the window only hides it: the icon, and any recording, keep going. Click the Dock icon or choose **Open Notted** to bring the window back. Choose **Quit Notted** (or press Cmd+Q) to quit; a recording in progress is stopped and kept.

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

1. **Archive folder:** click **Choose…** and pick a folder for your meetings archive (an empty folder works), or
   click **Clone the archive from GitHub instead**, paste its address and choose where to put it.
   Notted runs your own git, with your own login.
2. **Profile:** choose yourself from the people in the archive, or choose **New profile…** to
   create your file from a short template.
3. **Projects:** every project in the archive gets a row. Click **Choose…** to give it a root
   folder that Claude Code may read during its meetings. **New project…** creates the project's
   file and your own file for it.

Every new file opens in the editor below, ready to fill in. You can also paste a full path into
any field and click **Save settings**.

Not sure how to fill in your profile and projects? The **Set up with Claude Code** card in
Settings has a **Copy setup prompt** button. Paste what it copies into a Claude Code session: the
prompt explains how Notted works, lists what your archive holds now, and tells Claude Code to ask
you one question at a time and write your profile and project files under `instructions/`.
**Show prompt** lets you read it first. If the archive is not chosen yet, the prompt tells Claude
Code to send you back to Settings to choose or clone it first.

To remove a teammate's profile, click **Delete a profile…** under **Profile**. Notted lists every
file it will delete and asks first. You cannot delete the profile you are using. A project row
shows **Create my file** when your own file for that project is missing.

Notted never commits these changes: deleting and creating files only changes the archive folder,
like any edit.

To delete a meeting, click **Delete meeting** on its page. Notted lists what it removes (the audio, the transcript, the summary and the suggestions) and asks first. It removes only Notted's copy and leaves the meeting's folder in the archive as it is. A recording must be stopped first.

## Update Notted

Notted can update itself from your own clone of its source. If you installed with `make install`, that clone is already in use. Otherwise, in **Settings**, under **Update
Notted**, click **Choose…** and pick the folder of your Notted clone. **Update Notted** then:

1. checks that the clone is on `main` with no uncommitted changes, and stops if it is not;
2. runs `git pull --ff-only` on `main`, with your own git and your own login;
3. runs `make build` without the network (if an update adds dependencies, run `make build` once in Terminal first);
4. quits Notted, puts `build/bin/Notted.app` in place of the app you are running, and reopens it.

Nothing runs until you click **Update Notted**. A failed pull or build leaves your app as it was
and shows the output. The installer copies the new app next to the old one, swaps the two with
renames, and keeps the old copy until the new one has started. Its log is `~/.notted/update.log`.

macOS asks for the microphone and screen recording permissions again only when the app's signature changes. `make install` and **Update Notted** compare the signature of the installed app with the new one, and reset the two permissions only when they differ.

## What Notted downloads

On first use per language, Notted downloads the Whisper model (about 640 MB for English `small.en`, about 640 MB for Portuguese `small`) and the Silero speech detector (0.6 MB) from the sherpa-onnx releases on GitHub (`https://github.com/k2-fsa/sherpa-onnx/releases`). It verifies each file by SHA-256 and stores it in `~/.notted/models`. This is the only network call Notted makes; the `claude` program talks to Anthropic on its own. **Update Notted** runs your own git pull, which talks to your clone's remote.

## Your data

Everything is under `~/.notted`: `meetings.sqlite`, `audio/<meeting-id>/`, `models/` and `update.log`. Audio never leaves the Mac and is never committed. The first time you choose a folder, Notted adds what the archive needs (bin/meeting, prompts/, instructions/, the meetings skill, a commit hook and the recording rules in .gitignore) and changes nothing already there. A folder that is not in a git repository gets git init; with no remote, meetings are committed on this Mac only. A meeting's files (`meeting.md`, `transcript.md`, `summary.md` and `suggestions.md`) go in only after you say yes.

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
