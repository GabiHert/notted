import Foundation

/// Cuts one track's samples into chunks of exact lengths decided by the caller.
/// Not thread-safe: the owner calls it from one serial queue.
final class TrackWriter {
    typealias OnChunk = (_ track: String, _ index: Int, _ path: String, _ startMs: Int, _ endMs: Int) -> Void

    private let track: String
    private let dir: URL
    private let samplesPerChunk: Int
    private let onChunk: OnChunk
    private var buffer: [Int16] = []
    private var chunkStartSample = 0
    private var nextIndex = 1

    init(track: String, dir: URL, samplesPerChunk: Int, onChunk: @escaping OnChunk) {
        self.track = track
        self.dir = dir
        self.samplesPerChunk = samplesPerChunk
        self.onChunk = onChunk
        buffer.reserveCapacity(samplesPerChunk + 8192)
    }

    func append(_ samples: [Int16]) {
        buffer.append(contentsOf: samples)
    }

    /// Drops the samples held for the current chunk. Called once, when both tracks are running,
    /// so audio that arrived before the shared start time does not shift this track.
    func discardBuffered() {
        buffer.removeAll(keepingCapacity: true)
    }

    /// Closes the current chunk so it holds `endSample - chunkStartSample` samples:
    /// padded with zeros when short, with the surplus moved into the next chunk when long.
    func closeChunk(endSample: Int) {
        close(endSample: endSample, keepSurplus: true)
    }

    /// Closes the last chunk the same way, but drops any surplus.
    func finish(endSample: Int) {
        close(endSample: endSample, keepSurplus: false)
    }

    private func close(endSample: Int, keepSurplus: Bool) {
        let target = endSample - chunkStartSample
        guard target > 0 else {
            if !keepSurplus { buffer.removeAll() }
            return
        }
        var chunk: [Int16]
        if buffer.count > target {
            chunk = Array(buffer[0..<target])
            buffer = keepSurplus ? Array(buffer[target...]) : []
        } else {
            chunk = buffer
            if chunk.count < target { chunk.append(contentsOf: [Int16](repeating: 0, count: target - chunk.count)) }
            buffer = []
        }
        let index = nextIndex
        let name = String(format: "%@-%06d.wav", track, index)
        let url = dir.appendingPathComponent(name)
        let writer = WavWriter(finalURL: url)
        writer.append(chunk)
        do {
            let final = try writer.close()
            onChunk(track, index, final.path, chunkStartSample / 16, endSample / 16)
            nextIndex += 1
        } catch {
            writer.discard()
            FileHandle.standardError.write(Data("audiocap: could not close \(name): \(error)\n".utf8))
        }
        chunkStartSample = endSample
    }
}
