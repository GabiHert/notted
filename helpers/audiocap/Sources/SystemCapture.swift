import AVFoundation
import CoreMedia
import ScreenCaptureKit

/// Everything the Mac plays, through an audio-only ScreenCaptureKit stream that leaves out this process.
final class SystemCapture: NSObject, SCStreamOutput, SCStreamDelegate {
    private var stream: SCStream?
    private let resampler = Resampler()
    private let sampleQueue = DispatchQueue(label: "audiocap.system.audio")
    private let frameQueue = DispatchQueue(label: "audiocap.system.frames")
    private var onSamples: (([Int16]) -> Void)?
    private var onFailure: ((Error) -> Void)?

    func start(display: SCDisplay, onSamples: @escaping ([Int16]) -> Void, onFailure: @escaping (Error) -> Void) async throws {
        self.onSamples = onSamples
        self.onFailure = onFailure
        let filter = SCContentFilter(display: display, excludingApplications: [], exceptingWindows: [])
        let cfg = SCStreamConfiguration()
        cfg.capturesAudio = true
        cfg.excludesCurrentProcessAudio = true
        cfg.sampleRate = 16000
        cfg.channelCount = 1
        cfg.width = 2
        cfg.height = 2
        cfg.minimumFrameInterval = CMTime(value: 1, timescale: 1)
        let stream = SCStream(filter: filter, configuration: cfg, delegate: self)
        try stream.addStreamOutput(self, type: .audio, sampleHandlerQueue: sampleQueue)
        // Without a screen output the stream logs dropped-frame errors; the frames are ignored.
        try stream.addStreamOutput(self, type: .screen, sampleHandlerQueue: frameQueue)
        try await stream.startCapture()
        self.stream = stream
    }

    func stop() async {
        try? await stream?.stopCapture()
        stream = nil
    }

    func stream(_ stream: SCStream, didOutputSampleBuffer sampleBuffer: CMSampleBuffer, of type: SCStreamOutputType) {
        guard type == .audio else { return }
        let s = resampler.convert(sampleBuffer: sampleBuffer)
        if !s.isEmpty { onSamples?(s) }
    }

    func stream(_ stream: SCStream, didStopWithError error: Error) {
        onFailure?(error)
    }
}
