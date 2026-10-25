import AVFoundation
import Foundation

struct StartedEvent: Codable {
    var event = "started"
    var t0Ms: Int64
    enum CodingKeys: String, CodingKey { case event, t0Ms = "t0_ms" }
    init(t0Ms: Int64) { self.t0Ms = t0Ms }
}

struct ChunkEvent: Codable {
    var event = "chunk"
    var track: String
    var index: Int
    var path: String
    var startMs: Int
    var endMs: Int
    enum CodingKeys: String, CodingKey {
        case event, track, index, path
        case startMs = "start_ms", endMs = "end_ms"
    }
    init(track: String, index: Int, path: String, startMs: Int, endMs: Int) {
        self.track = track; self.index = index; self.path = path
        self.startMs = startMs; self.endMs = endMs
    }
}

struct PermissionEvent: Codable {
    var event = "permission"
    var name: String
    var granted: Bool
    /// "allowed", "not_asked" or "denied"; only `status` events carry it.
    var status: String?
    init(name: String, granted: Bool) { self.name = name; self.granted = granted }
    init(name: String, status: String) { self.name = name; self.granted = status == "allowed"; self.status = status }
}

func micStatusName(_ s: AVAuthorizationStatus) -> String {
    switch s {
    case .authorized: return "allowed"
    case .notDetermined: return "not_asked"
    default: return "denied"
    }
}

/// Screen recording has no "not asked" answer: macOS reports only allowed or not.
func screenStatusName(_ granted: Bool) -> String { granted ? "allowed" : "denied" }

struct ErrorEvent: Codable {
    var event = "error"
    var code: String
    var message: String
    init(code: String, message: String) { self.code = code; self.message = message }
}

struct StoppedEvent: Codable {
    var event = "stopped"
    init() {}
}

func encodeEvent<E: Encodable>(_ e: E) -> String {
    let enc = JSONEncoder()
    enc.outputFormatting = [.sortedKeys, .withoutEscapingSlashes]
    guard let data = try? enc.encode(e) else { return "{}" }
    return String(decoding: data, as: UTF8.self)
}

/// Writes one event as a JSON line on stdout, unbuffered.
func emit<E: Encodable>(_ e: E) {
    FileHandle.standardOutput.write(Data((encodeEvent(e) + "\n").utf8))
}
