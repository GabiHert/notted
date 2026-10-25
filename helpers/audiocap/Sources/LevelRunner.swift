import Foundation

/// `audiocap level`: listens to the default input and prints its level about every 100 ms.
/// It keeps no audio and writes no files.
final class LevelRunner: @unchecked Sendable {
    private let seconds: Int
    private let queue = DispatchQueue(label: "audiocap.level")
    private let mic = MicCapture()
    private var meter = LevelMeter()
    private var timer: DispatchSourceTimer?
    private var finished = false

    init(seconds: Int) { self.seconds = seconds }

    func run() async {
        guard await microphoneAllowed(asking: true) else {
            fail("mic_denied", "Microphone permission is denied.", exitCode: ExitCode.micDenied)
        }
        guard mic.inputFormat() != nil else {
            fail("no_device", "No microphone input device is available.", exitCode: ExitCode.noDevice)
        }
        emit(DeviceEvent(device: mic.deviceName()))
        do {
            try mic.start { [weak self] samples in
                guard let self = self else { return }
                self.queue.async {
                    if self.finished { return }
                    for r in self.meter.add(samples) { emit(LevelEvent(rmsDb: r.rmsDb, peakDb: r.peakDb)) }
                }
            }
        } catch {
            fail("capture_failed", "Microphone failed to start: \(error.localizedDescription)", exitCode: ExitCode.failure)
        }
        let timer = DispatchSource.makeTimerSource(queue: queue)
        timer.schedule(deadline: .now() + .seconds(seconds))
        timer.setEventHandler { [weak self] in self?.finish() }
        timer.resume()
        self.timer = timer
    }

    /// Stops listening, prints `stopped`, exits 0. Safe to call more than once.
    func stop() { queue.async { [weak self] in self?.finish() } }

    private func finish() {
        if finished { return }
        finished = true
        timer?.cancel()
        mic.stop()
        emit(StoppedEvent())
        exit(0)
    }
}
