import Foundation

/// What `level` reports for digital silence, and the floor of every reading.
let silenceFloorDb = -120.0

struct LevelReading: Equatable {
    var rmsDb: Double
    var peakDb: Double
}

/// dBFS of a sample size as a fraction of full scale (0...1), rounded to 0.1 dB; 0 reads as the floor.
func levelDb(_ fraction: Double) -> Double {
    guard fraction > 0 else { return silenceFloorDb }
    let db = max(20 * log10(fraction), silenceFloorDb)
    return (db * 10).rounded() / 10
}

/// Turns 16 kHz mono samples into one reading per 100 ms; a remainder waits for the next call.
struct LevelMeter {
    static let windowSamples = 1600
    private var pending: [Int16] = []

    mutating func add(_ samples: [Int16]) -> [LevelReading] {
        pending.append(contentsOf: samples)
        var readings: [LevelReading] = []
        var start = 0
        while pending.count - start >= LevelMeter.windowSamples {
            readings.append(LevelMeter.reading(pending[start..<start + LevelMeter.windowSamples]))
            start += LevelMeter.windowSamples
        }
        pending.removeFirst(start)
        return readings
    }

    private static func reading(_ window: ArraySlice<Int16>) -> LevelReading {
        var sum = 0.0
        var peak = 0.0
        for s in window {
            let v = abs(Double(s)) / 32768
            sum += v * v
            peak = max(peak, v)
        }
        return LevelReading(rmsDb: levelDb((sum / Double(window.count)).squareRoot()), peakDb: levelDb(min(peak, 1)))
    }
}

/// The seconds of `audiocap level [--seconds N]`: 1...60, 10 by default; nil for anything else.
func parseLevelSeconds(_ rest: [String]) -> Int? {
    if rest.isEmpty { return 10 }
    guard rest.count == 2, rest[0] == "--seconds", let n = Int(rest[1]), (1...60).contains(n) else { return nil }
    return n
}
