import AVFoundation
import CoreMedia

/// Converts capture buffers of any real format to 16 kHz mono Int16.
final class Resampler {
    private let outFormat = AVAudioFormat(commonFormat: .pcmFormatInt16, sampleRate: 16000, channels: 1, interleaved: true)!
    private var converter: AVAudioConverter?
    private var inFormat: AVAudioFormat?

    func convert(_ buffer: AVAudioPCMBuffer) -> [Int16] {
        guard buffer.frameLength > 0 else { return [] }
        if inFormat != buffer.format {
            inFormat = buffer.format
            converter = AVAudioConverter(from: buffer.format, to: outFormat)
            converter?.downmix = true
        }
        guard let converter = converter else { return [] }
        let ratio = outFormat.sampleRate / buffer.format.sampleRate
        let capacity = AVAudioFrameCount(Double(buffer.frameLength) * ratio) + 64
        guard let out = AVAudioPCMBuffer(pcmFormat: outFormat, frameCapacity: capacity) else { return [] }
        var supplied = false
        var error: NSError?
        let status = converter.convert(to: out, error: &error) { _, inputStatus in
            if supplied {
                inputStatus.pointee = .noDataNow
                return nil
            }
            supplied = true
            inputStatus.pointee = .haveData
            return buffer
        }
        if status == .error || error != nil {
            FileHandle.standardError.write(Data("audiocap: resample failed: \(error?.localizedDescription ?? "unknown")\n".utf8))
            return []
        }
        guard let ptr = out.int16ChannelData?[0] else { return [] }
        return Array(UnsafeBufferPointer(start: ptr, count: Int(out.frameLength)))
    }

    func convert(sampleBuffer: CMSampleBuffer) -> [Int16] {
        guard CMSampleBufferIsValid(sampleBuffer),
              let desc = CMSampleBufferGetFormatDescription(sampleBuffer) else { return [] }
        let frames = CMSampleBufferGetNumSamples(sampleBuffer)
        guard frames > 0 else { return [] }
        let format = AVAudioFormat(cmAudioFormatDescription: desc)
        guard let pcm = AVAudioPCMBuffer(pcmFormat: format, frameCapacity: AVAudioFrameCount(frames)) else { return [] }
        pcm.frameLength = AVAudioFrameCount(frames)
        let status = CMSampleBufferCopyPCMDataIntoAudioBufferList(
            sampleBuffer, at: 0, frameCount: Int32(frames), into: pcm.mutableAudioBufferList)
        guard status == noErr else { return [] }
        return convert(pcm)
    }
}
