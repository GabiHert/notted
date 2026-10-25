import AVFoundation
import CoreGraphics
import ScreenCaptureKit

enum ExitCode {
    static let failure: Int32 = 1
    static let micDenied: Int32 = 2
    static let screenDenied: Int32 = 3
    static let noDevice: Int32 = 4
}

/// Prints an `error` event and exits.
func fail(_ code: String, _ message: String, exitCode: Int32) -> Never {
    emit(ErrorEvent(code: code, message: message))
    FileHandle.standardError.write(Data("audiocap: \(message)\n".utf8))
    exit(exitCode)
}

/// True when the microphone is allowed; asks once when the answer is still open.
func microphoneAllowed(asking: Bool) async -> Bool {
    switch AVCaptureDevice.authorizationStatus(for: .audio) {
    case .authorized: return true
    case .notDetermined:
        return asking ? await AVCaptureDevice.requestAccess(for: .audio) : false
    default: return false
    }
}

/// `audiocap check`: asks for and tests both permissions, prints one event for each, exits.
func runCheck() async -> Never {
    let mic = await microphoneAllowed(asking: true)
    emit(PermissionEvent(name: "microphone", granted: mic))
    var screen = CGPreflightScreenCaptureAccess()
    if !screen {
        _ = CGRequestScreenCaptureAccess()
        screen = CGPreflightScreenCaptureAccess()
    }
    emit(PermissionEvent(name: "screen", granted: screen))
    if !mic { exit(ExitCode.micDenied) }
    if !screen { exit(ExitCode.screenDenied) }
    exit(0)
}

/// `audiocap status`: prints both permissions without asking for anything, exits 0.
func runStatus() -> Never {
    emit(PermissionEvent(name: "microphone", status: micStatusName(AVCaptureDevice.authorizationStatus(for: .audio))))
    emit(PermissionEvent(name: "screen", status: screenStatusName(CGPreflightScreenCaptureAccess())))
    exit(0)
}

/// The `record` pre-checks. Returns the display to capture, or exits with the contract's code.
func recordPreflight(mic: MicCapture) async -> SCDisplay {
    guard await microphoneAllowed(asking: true) else {
        fail("mic_denied", "Microphone permission is denied.", exitCode: ExitCode.micDenied)
    }
    let content: SCShareableContent
    do {
        content = try await SCShareableContent.excludingDesktopWindows(false, onScreenWindowsOnly: true)
    } catch {
        fail("screen_denied", "Screen recording permission is denied: \(error.localizedDescription)", exitCode: ExitCode.screenDenied)
    }
    guard let display = content.displays.first else {
        fail("no_device", "No display is available for system audio.", exitCode: ExitCode.noDevice)
    }
    guard mic.inputFormat() != nil else {
        fail("no_device", "No microphone input device is available.", exitCode: ExitCode.noDevice)
    }
    return display
}
