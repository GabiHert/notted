import Foundation

var failures = 0
var checks = 0
func check(_ cond: Bool, _ msg: String) {
    checks += 1
    if !cond {
        failures += 1
        FileHandle.standardError.write(Data("FAIL: \(msg)\n".utf8))
    }
}

func le32(_ d: Data, _ o: Int) -> UInt32 {
    UInt32(d[o]) | UInt32(d[o + 1]) << 8 | UInt32(d[o + 2]) << 16 | UInt32(d[o + 3]) << 24
}
func le16(_ d: Data, _ o: Int) -> UInt16 { UInt16(d[o]) | UInt16(d[o + 1]) << 8 }

func samples(of url: URL) -> [Int16] {
    guard let d = try? Data(contentsOf: url), d.count >= 44 else { return [] }
    return stride(from: 44, to: d.count - 1, by: 2).map { Int16(bitPattern: le16(d, $0)) }
}

let args = CommandLine.arguments
guard args.count > 1 else {
    FileHandle.standardError.write(Data("usage: audiocap-tests <tmp dir>\n".utf8))
    exit(2)
}
let root = URL(fileURLWithPath: args[1], isDirectory: true)
let fm = FileManager.default
func fresh(_ name: String) -> URL {
    let u = root.appendingPathComponent(name, isDirectory: true)
    try? fm.removeItem(at: u)
    try! fm.createDirectory(at: u, withIntermediateDirectories: true)
    return u
}

// 1. WavWriter
do {
    let dir = fresh("wav")
    let final = dir.appendingPathComponent("x.wav")
    let w = WavWriter(finalURL: final)
    w.append([1, 2, 3, 4, 5])
    check(w.sampleCount == 5, "sampleCount 5, got \(w.sampleCount)")
    check(fm.fileExists(atPath: final.path + ".part"), "part exists while open")
    let out = try! w.close()
    check(out == final, "close returns final URL")
    check(!fm.fileExists(atPath: final.path + ".part"), "part gone after close")
    check(fm.fileExists(atPath: final.path), "final exists")
    let d = try! Data(contentsOf: final)
    check(d.count == 44 + 10, "file size 54, got \(d.count)")
    check(String(decoding: d[0..<4], as: UTF8.self) == "RIFF", "RIFF tag")
    check(le32(d, 4) == 36 + 10, "riff size")
    check(String(decoding: d[8..<16], as: UTF8.self) == "WAVEfmt ", "WAVEfmt tag")
    check(le32(d, 16) == 16, "fmt size")
    check(le16(d, 20) == 1, "pcm format")
    check(le16(d, 22) == 1, "channels 1")
    check(le32(d, 24) == 16000, "rate 16000")
    check(le32(d, 28) == 32000, "byte rate")
    check(le16(d, 32) == 2, "block align")
    check(le16(d, 34) == 16, "bits 16")
    check(String(decoding: d[36..<40], as: UTF8.self) == "data", "data tag")
    check(le32(d, 40) == 10, "data size")
    check(samples(of: final) == [1, 2, 3, 4, 5], "samples round-trip")
}

typealias Rec = (track: String, index: Int, path: String, startMs: Int, endMs: Int)

// 2. TrackWriter rotation
do {
    let dir = fresh("rotate")
    var recs: [Rec] = []
    let tw = TrackWriter(track: "mic", dir: dir, samplesPerChunk: 32000) { t, i, p, s, e in
        recs.append((t, i, p, s, e))
    }
    var next = 0
    var total = 0
    let pieces = [1000, 7000, 3500, 5000, 2000, 6500, 4000, 7000, 1000, 2500, 6000, 4500, 3000, 7000, 4000, 6000, 1000]
    for p in pieces where total < 70000 {
        let n = min(p, 70000 - total)
        tw.append((0..<n).map { Int16(truncatingIfNeeded: next + $0 + 1) })
        next += n
        total += n
    }
    if total < 70000 { tw.append((0..<(70000 - total)).map { _ in 7 }) }
    tw.closeChunk(endSample: 32000)
    tw.closeChunk(endSample: 64000)
    tw.finish(endSample: 70000)
    check(recs.count == 3, "3 callbacks, got \(recs.count)")
    if recs.count == 3 {
        check(recs.map { $0.index } == [1, 2, 3], "indexes")
        check(recs.map { $0.startMs } == [0, 2000, 4000], "start ms \(recs.map { $0.startMs })")
        check(recs.map { $0.endMs } == [2000, 4000, 4375], "end ms \(recs.map { $0.endMs })")
        check(recs.allSatisfy { $0.track == "mic" }, "track name")
        check(recs[0].path.hasSuffix("/mic-000001.wav"), "name 1: \(recs[0].path)")
        check(recs[2].path.hasSuffix("/mic-000003.wav"), "name 3: \(recs[2].path)")
        check(recs[0].path.hasPrefix("/"), "absolute path")
        let counts = recs.map { samples(of: URL(fileURLWithPath: $0.path)).count }
        check(counts == [32000, 32000, 6000], "sample counts \(counts)")
        let first = samples(of: URL(fileURLWithPath: recs[0].path))
        check(first.first == 1 && first[31999] == Int16(truncatingIfNeeded: 32000), "first chunk content continuity")
    }
    let leftovers = (try? fm.contentsOfDirectory(atPath: dir.path).filter { $0.hasSuffix(".part") }) ?? ["?"]
    check(leftovers.isEmpty, "no .part left: \(leftovers)")
}

// 3. Padding
do {
    let dir = fresh("pad")
    var recs: [Rec] = []
    let tw = TrackWriter(track: "others", dir: dir, samplesPerChunk: 32000) { t, i, p, s, e in
        recs.append((t, i, p, s, e))
    }
    tw.append([Int16](repeating: 5, count: 10000))
    tw.closeChunk(endSample: 32000)
    check(recs.count == 1, "one callback")
    if let r = recs.first {
        let s = samples(of: URL(fileURLWithPath: r.path))
        check(s.count == 32000, "padded to 32000, got \(s.count)")
        check(s[9999] == 5 && s[10000] == 0 && s[31999] == 0, "padding is zeros")
        check(s[10000...].allSatisfy { $0 == 0 }, "all 22000 padded zero")
        check(r.path.hasSuffix("/others-000001.wav"), "others name")
    }
}

// 4. Surplus
do {
    let dir = fresh("surplus")
    var recs: [Rec] = []
    let tw = TrackWriter(track: "mic", dir: dir, samplesPerChunk: 32000) { t, i, p, s, e in
        recs.append((t, i, p, s, e))
    }
    tw.append((0..<40000).map { Int16(truncatingIfNeeded: $0 % 30000) })
    tw.closeChunk(endSample: 32000)
    tw.closeChunk(endSample: 64000)
    check(recs.count == 2, "two callbacks")
    if recs.count == 2 {
        let a = samples(of: URL(fileURLWithPath: recs[0].path))
        let b = samples(of: URL(fileURLWithPath: recs[1].path))
        check(a.count == 32000, "first holds 32000, got \(a.count)")
        check(b.count == 32000, "second padded to 32000, got \(b.count)")
        check(Array(b[0..<8000]) == (32000..<40000).map { Int16(truncatingIfNeeded: $0 % 30000) }, "surplus moved into next chunk")
        check(b[8000...].allSatisfy { $0 == 0 }, "rest of second chunk is zero")
    }
}

// 5. Empty last chunk
do {
    let dir = fresh("empty")
    var recs: [Rec] = []
    let tw = TrackWriter(track: "mic", dir: dir, samplesPerChunk: 32000) { t, i, p, s, e in
        recs.append((t, i, p, s, e))
    }
    tw.append([Int16](repeating: 1, count: 32000))
    tw.closeChunk(endSample: 32000)
    tw.finish(endSample: 32000)
    check(recs.count == 1, "no callback for empty last chunk, got \(recs.count)")
    let files = (try? fm.contentsOfDirectory(atPath: dir.path)) ?? []
    check(files == ["mic-000001.wav"], "only one file: \(files)")
}

// 5b. finish drops surplus
do {
    let dir = fresh("drop")
    var recs: [Rec] = []
    let tw = TrackWriter(track: "mic", dir: dir, samplesPerChunk: 32000) { t, i, p, s, e in
        recs.append((t, i, p, s, e))
    }
    tw.append([Int16](repeating: 1, count: 40000))
    tw.finish(endSample: 32000)
    check(recs.count == 1, "one callback")
    check(samples(of: URL(fileURLWithPath: recs[0].path)).count == 32000, "surplus dropped at finish")
}

// 6. Event encoding
do {
    let chunk = ChunkEvent(track: "mic", index: 1, path: "/tmp/x/mic-000001.wav", startMs: 0, endMs: 2000)
    check(encodeEvent(chunk) == #"{"end_ms":2000,"event":"chunk","index":1,"path":"/tmp/x/mic-000001.wav","start_ms":0,"track":"mic"}"#,
          "chunk event: \(encodeEvent(chunk))")
    check(encodeEvent(StartedEvent(t0Ms: 1700000000000)) == #"{"event":"started","t0_ms":1700000000000}"#, "started")
    check(encodeEvent(PermissionEvent(name: "microphone", granted: true)) == #"{"event":"permission","granted":true,"name":"microphone"}"#, "permission")
    check(encodeEvent(ErrorEvent(code: "bad_args", message: "x")) == #"{"code":"bad_args","event":"error","message":"x"}"#, "error")
    check(encodeEvent(StoppedEvent()) == #"{"event":"stopped"}"#, "stopped")
}

// 7. Alignment: audio buffered before t0 is discarded
do {
    let dir = fresh("align")
    var recs: [Rec] = []
    let tw = TrackWriter(track: "mic", dir: dir, samplesPerChunk: 32000) { t, i, p, s, e in
        recs.append((t, i, p, s, e))
    }
    tw.append([Int16](repeating: 9, count: 5000))
    tw.discardBuffered()
    tw.append([Int16](repeating: 3, count: 4000))
    tw.closeChunk(endSample: 32000)
    check(recs.count == 1, "one callback after discard")
    if let r = recs.first {
        let s = samples(of: URL(fileURLWithPath: r.path))
        check(s.count == 32000, "chunk is 32000 samples")
        check(s[0..<4000].allSatisfy { $0 == 3 }, "pre-t0 audio gone, post-t0 audio first")
        check(s[4000...].allSatisfy { $0 == 0 }, "rest is padding")
    }
}

// 8. Early stop: finishing a writer that never got audio or a chunk makes no file
do {
    let dir = fresh("early")
    var count = 0
    let tw = TrackWriter(track: "others", dir: dir, samplesPerChunk: 32000) { _, _, _, _, _ in count += 1 }
    tw.finish(endSample: 0)
    check(count == 0, "no callback on early finish")
    check(((try? fm.contentsOfDirectory(atPath: dir.path)) ?? ["?"]).isEmpty, "no files on early finish")
}

print("\(checks - failures)/\(checks) checks passed")
exit(failures == 0 ? 0 : 1)
