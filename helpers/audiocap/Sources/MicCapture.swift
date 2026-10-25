import AVFoundation

/// The microphone through AVAudioEngine.
final class MicCapture {
    let engine = AVAudioEngine()
    private let resampler = Resampler()

    /// The input node's format, or nil when there is no usable input device.
    func inputFormat() -> AVAudioFormat? {
        let f = engine.inputNode.outputFormat(forBus: 0)
        return (f.sampleRate == 0 || f.channelCount == 0) ? nil : f
    }

    /// The name macOS gives the default input ("MacBook Air Microphone"); empty when it gives none.
    func deviceName() -> String {
        AVCaptureDevice.default(for: .audio)?.localizedName ?? ""
    }

    func start(onSamples: @escaping ([Int16]) -> Void) throws {
        let input = engine.inputNode
        input.installTap(onBus: 0, bufferSize: 4096, format: input.outputFormat(forBus: 0)) { [resampler] buffer, _ in
            let s = resampler.convert(buffer)
            if !s.isEmpty { onSamples(s) }
        }
        engine.prepare()
        try engine.start()
    }

    func stop() {
        engine.inputNode.removeTap(onBus: 0)
        engine.stop()
    }
}
