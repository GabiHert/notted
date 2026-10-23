import Foundation
import ScreenCaptureKit

/// Runs both tracks on one serial queue so chunk n of each covers the same time range.
final class Recorder: @unchecked Sendable {
    private let dir: URL
    private let chunkSeconds: Int
    private let writerQueue = DispatchQueue(label: "audiocap.writer")
    private let mic = MicCapture()
    private let system = SystemCapture()
    private var micWriter: TrackWriter?
    private var othersWriter: TrackWriter?
    private var timer: DispatchSourceTimer?
    private var t0: DispatchTime?
    private var ticks = 0
    private var stopping = false

    init(dir: URL, chunkSeconds: Int) {
        self.dir = dir
        self.chunkSeconds = chunkSeconds
    }

    func run() async {
        let display = await recordPreflight(mic: mic)
        do {
            try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        } catch {
            fail("capture_failed", "Cannot create \(dir.path): \(error.localizedDescription)", exitCode: ExitCode.failure)
        }
        let perChunk = chunkSeconds * 16000
        let onChunk: TrackWriter.OnChunk = { track, index, path, startMs, endMs in
            emit(ChunkEvent(track: track, index: index, path: path, startMs: startMs, endMs: endMs))
        }
        micWriter = TrackWriter(track: "mic", dir: dir, samplesPerChunk: perChunk, onChunk: onChunk)
        othersWriter = TrackWriter(track: "others", dir: dir, samplesPerChunk: perChunk, onChunk: onChunk)

        do {
            try mic.start { [weak self] samples in
                guard let self = self else { return }
                self.writerQueue.async { self.micWriter?.append(samples) }
            }
        } catch {
            fail("capture_failed", "Microphone failed to start: \(error.localizedDescription)", exitCode: ExitCode.failure)
        }
        do {
            try await system.start(display: display, onSamples: { [weak self] samples in
                guard let self = self else { return }
                self.writerQueue.async { self.othersWriter?.append(samples) }
            }, onFailure: { [weak self] error in
                self?.shutdown(errorMessage: "System audio stopped: \(error.localizedDescription)")
            })
        } catch {
            mic.stop()
            if (error as NSError).code == SCStreamError.userDeclined.rawValue {
                fail("screen_denied", "Screen recording permission is denied.", exitCode: ExitCode.screenDenied)
            }
            fail("capture_failed", "System audio failed to start: \(error.localizedDescription)", exitCode: ExitCode.failure)
        }

        writerQueue.async { [self] in
            t0 = DispatchTime.now()
            // Both captures are running: audio buffered so far predates t0 and would shift a track.
            micWriter?.discardBuffered()
            othersWriter?.discardBuffered()
            emit(StartedEvent(t0Ms: Int64(Date().timeIntervalSince1970 * 1000)))
            let timer = DispatchSource.makeTimerSource(queue: writerQueue)
            timer.schedule(deadline: .now() + .seconds(chunkSeconds), repeating: .seconds(chunkSeconds))
            timer.setEventHandler { [self] in
                ticks += 1
                let end = ticks * chunkSeconds * 16000
                micWriter?.closeChunk(endSample: end)
                othersWriter?.closeChunk(endSample: end)
            }
            timer.resume()
            self.timer = timer
        }
    }

    /// Stops both captures, closes the open chunks, prints `stopped`, exits 0.
    func stop() { shutdown(errorMessage: nil) }

    private func shutdown(errorMessage: String?) {
        writerQueue.async { [self] in
            if stopping { return }
            stopping = true
            timer?.cancel()
            guard t0 != nil else {
                // Stopped before both captures were running: nothing was recorded.
                if let message = errorMessage {
                    emit(ErrorEvent(code: "capture_failed", message: message))
                    exit(ExitCode.failure)
                }
                emit(StoppedEvent())
                exit(0)
            }
            Task {
                mic.stop()
                await system.stop()
                writerQueue.async { [self] in
                    let started = t0 ?? DispatchTime.now()
                    let seconds = Double(DispatchTime.now().uptimeNanoseconds - started.uptimeNanoseconds) / 1e9
                    let elapsed = Int(seconds * 16000)
                    micWriter?.finish(endSample: elapsed)
                    othersWriter?.finish(endSample: elapsed)
                    if let message = errorMessage {
                        emit(ErrorEvent(code: "capture_failed", message: message))
                        FileHandle.standardError.write(Data("audiocap: \(message)\n".utf8))
                        exit(ExitCode.failure)
                    }
                    emit(StoppedEvent())
                    exit(0)
                }
            }
        }
    }
}
