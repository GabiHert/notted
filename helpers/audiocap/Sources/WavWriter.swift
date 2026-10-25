import Foundation

enum WavError: Error { case closed, io(String) }

/// Writes a 16 kHz mono 16-bit PCM WAV to `<final>.part` and renames it on close.
final class WavWriter {
    let finalURL: URL
    private let partURL: URL
    private var handle: FileHandle?
    private(set) var sampleCount = 0

    init(finalURL: URL) {
        self.finalURL = finalURL
        self.partURL = URL(fileURLWithPath: finalURL.path + ".part")
        FileManager.default.createFile(atPath: partURL.path, contents: nil)
        handle = try? FileHandle(forWritingTo: partURL)
        handle?.write(WavWriter.header(dataBytes: 0))
    }

    func append(_ samples: [Int16]) {
        guard let handle = handle, !samples.isEmpty else { return }
        var data = Data(capacity: samples.count * 2)
        for s in samples {
            let u = UInt16(bitPattern: s)
            data.append(UInt8(u & 0xff))
            data.append(UInt8(u >> 8))
        }
        handle.write(data)
        sampleCount += samples.count
    }

    func close() throws -> URL {
        guard let handle = handle else { throw WavError.closed }
        let dataBytes = UInt32(sampleCount * 2)
        try handle.seek(toOffset: 0)
        handle.write(WavWriter.header(dataBytes: dataBytes))
        try handle.close()
        self.handle = nil
        try? FileManager.default.removeItem(at: finalURL)
        try FileManager.default.moveItem(at: partURL, to: finalURL)
        return finalURL
    }

    /// Removes the unfinished file without producing a final one.
    func discard() {
        try? handle?.close()
        handle = nil
        try? FileManager.default.removeItem(at: partURL)
    }

    static func header(dataBytes: UInt32) -> Data {
        var d = Data()
        func u32(_ v: UInt32) { for i in 0..<4 { d.append(UInt8((v >> (8 * UInt32(i))) & 0xff)) } }
        func u16(_ v: UInt16) { d.append(UInt8(v & 0xff)); d.append(UInt8(v >> 8)) }
        d.append(contentsOf: Array("RIFF".utf8)); u32(36 + dataBytes)
        d.append(contentsOf: Array("WAVEfmt ".utf8)); u32(16)
        u16(1); u16(1); u32(16000); u32(32000); u16(2); u16(16)
        d.append(contentsOf: Array("data".utf8)); u32(dataBytes)
        return d
    }
}
