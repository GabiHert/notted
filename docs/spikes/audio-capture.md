# Audio-capture spike (T035)

- Date: 2026-10-23
- Mac: macOS 26.6.2 (build 25G83), Apple silicon
- Helper: helpers/audiocap at commit 3c76ce5, built with swiftc 6.3.2 (Apple Swift version 6.3.2, swiftlang-6.3.2.1.108)
- Run from: Terminal.app
- Verdict: PASS

## Permissions
- `check` output: **not captured.** The user ran the steps in Terminal.app but did not paste the two
  `check` lines.
- Prompts seen and clicked: **not captured** in detail. The user reported granting the microphone and
  Screen & System Audio Recording permissions to Terminal.
- Terminal restart after enabling screen recording: **not captured.**
- What the runs prove: `record` exits 2 (`mic_denied`) or 3 (`screen_denied`) before `started`
  when a permission is missing. Both runs emitted `started` and wrote both tracks, so both permissions
  were granted to Terminal at 20:14 and 20:22.

## Recording
Two runs, both from Terminal.app by the user.

Run 1 (20:14): `audiocap record --out /tmp/notted-spike`, Ctrl-C after about 23 s; nothing was playing.
Event log `/tmp/notted-spike-events.jsonl`, verbatim:

```
{"event":"started","t0_ms":1792797238545}
{"end_ms":23172,"event":"chunk","index":1,"path":"/tmp/notted-spike/mic-000001.wav","start_ms":0,"track":"mic"}
{"end_ms":23172,"event":"chunk","index":1,"path":"/tmp/notted-spike/others-000001.wav","start_ms":0,"track":"others"}
{"event":"stopped"}
```

Mic -42.5 dBFS (`silent=false`), and the user heard their voice. Others -120.0 dBFS, exact zeros: inconclusive,
because ScreenCaptureKit delivers zero buffers when nothing plays.

Run 2 (20:22): an `afplay /System/Library/Sounds/Glass.aiff` loop (×15) while recording into
`/tmp/notted-spike2`, then `pkill -INT -f 'audiocap record'`. Event log `/tmp/notted-spike2-events.jsonl`, verbatim:

```
{"event":"started","t0_ms":1792797712968}
{"end_ms":30000,"event":"chunk","index":1,"path":"/tmp/notted-spike2/mic-000001.wav","start_ms":0,"track":"mic"}
{"end_ms":30000,"event":"chunk","index":1,"path":"/tmp/notted-spike2/others-000001.wav","start_ms":0,"track":"others"}
{"end_ms":34006,"event":"chunk","index":2,"path":"/tmp/notted-spike2/mic-000002.wav","start_ms":30000,"track":"mic"}
{"end_ms":34006,"event":"chunk","index":2,"path":"/tmp/notted-spike2/others-000002.wav","start_ms":30000,"track":"others"}
{"event":"stopped"}
```

Both tracks rotated on the same 30 s boundary with identical byte sizes, and SIGINT stopped the helper cleanly.

## WAV check
Run 2, `go run ./scripts/wavcheck -expect-chunk 30 /tmp/notted-spike2/*.wav`, output verbatim (re-run by the conductor on the same files):

```
/tmp/notted-spike2/mic-000001.wav  rate=16000 channels=1 bits=16 duration=30.00s rms_dbfs=-36.1 silent=false
/tmp/notted-spike2/mic-000002.wav  rate=16000 channels=1 bits=16 duration=4.01s rms_dbfs=-47.1 silent=false
/tmp/notted-spike2/others-000001.wav  rate=16000 channels=1 bits=16 duration=30.00s rms_dbfs=-32.4 silent=false
/tmp/notted-spike2/others-000002.wav  rate=16000 channels=1 bits=16 duration=4.01s rms_dbfs=-34.8 silent=false
OK 4 files
```

Per-second RMS of `others-000001` (dBFS):

```
-50 -29 -49 -29 -50 -29 -56 -45 -29 -83 -29 -62 -32 -31 -85
-29 -67 -31 -33 -30 -36 -88 -29 -69 -29 -74 -29 -75 -29 -78
```

A chime peak about every 2 s with decay, so system audio is captured. `mic-000001` stayed
between -31 and -48 dBFS (room sound and the chime through the microphone).

## Listening
- mic: yes. The user heard their voice (run 1).
- others: not judged by ear in run 1 (nothing was playing). Run 2 confirmed the system audio by
  measurement: the chime loop shows in the per-second RMS above.

## Notes for the app
- When nothing plays, `others` is exact zeros (-120.0 dBFS, `silent=true`), because
  ScreenCaptureKit delivers no buffers. Transcription should skip silent chunks (VAD).
- The permissions belong to the app that runs the helper: Terminal in this spike, Notted.app in
  production. The user re-grants both permissions for Notted on first run.
- The audio files from both runs are in `/tmp` (`/tmp/notted-spike`, `/tmp/notted-spike2`) and
  are not committed.
