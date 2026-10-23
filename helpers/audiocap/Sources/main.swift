import Foundation

let usageText = "usage: audiocap record --out <dir> [--chunk-seconds 30]\n       audiocap check\n"

func printUsage() {
    FileHandle.standardError.write(Data(usageText.utf8))
}

func badArgs(_ message: String) -> Never {
    printUsage()
    fail("bad_args", message, exitCode: ExitCode.failure)
}

struct RecordArgs {
    var out: String
    var chunkSeconds: Int
}

func parseRecordArgs(_ rest: [String]) -> RecordArgs {
    var out: String?
    var chunk = 30
    var i = 0
    while i < rest.count {
        switch rest[i] {
        case "--out":
            guard i + 1 < rest.count else { badArgs("--out needs a folder.") }
            out = rest[i + 1]
            i += 2
        case "--chunk-seconds":
            guard i + 1 < rest.count, let n = Int(rest[i + 1]), (1...300).contains(n) else {
                badArgs("--chunk-seconds must be a whole number from 1 to 300.")
            }
            chunk = n
            i += 2
        default:
            badArgs("Unknown argument \(rest[i]).")
        }
    }
    guard let out = out, !out.isEmpty else { badArgs("--out <dir> is required.") }
    return RecordArgs(out: out, chunkSeconds: chunk)
}

let argv = Array(CommandLine.arguments.dropFirst())
guard let command = argv.first else {
    printUsage()
    exit(1)
}

switch command {
case "check":
    Task { await runCheck() }
    dispatchMain()
case "record":
    let parsed = parseRecordArgs(Array(argv.dropFirst()))
    let dir = URL(fileURLWithPath: parsed.out, isDirectory: true).standardizedFileURL
    let recorder = Recorder(dir: dir, chunkSeconds: parsed.chunkSeconds)

    var sources: [DispatchSourceSignal] = []
    for sig in [SIGINT, SIGTERM] {
        signal(sig, SIG_IGN)
        let src = DispatchSource.makeSignalSource(signal: sig, queue: .main)
        src.setEventHandler { recorder.stop() }
        src.resume()
        sources.append(src)
    }
    FileHandle.standardInput.readabilityHandler = { handle in
        if handle.availableData.isEmpty {
            handle.readabilityHandler = nil
            recorder.stop()
        }
    }
    Task { await recorder.run() }
    dispatchMain()
default:
    badArgs("Unknown command \(command).")
}
