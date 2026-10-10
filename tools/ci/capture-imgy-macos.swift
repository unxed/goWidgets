import AppKit
import CoreGraphics
import Foundation
import ScreenCaptureKit

guard CommandLine.arguments.count == 3,
      let pidValue = Int32(CommandLine.arguments[1]) else {
    fatalError("usage: capture-imgy-macos.swift PID output.png")
}

let pid = pid_t(pidValue)
guard let app = NSRunningApplication(processIdentifier: pid) else {
    fatalError("imgy process is not running")
}
let output = URL(fileURLWithPath: CommandLine.arguments[2])
app.activate(options: [])

Task {
    do {
        var captured: CGImage?
        for _ in 0..<80 {
            let content = try await SCShareableContent.excludingDesktopWindows(false, onScreenWindowsOnly: true)
            if let window = content.windows.first(where: { $0.owningApplication?.processID == pid }) {
                try await Task.sleep(for: .seconds(1))
                let filter = SCContentFilter(desktopIndependentWindow: window)
                let configuration = SCStreamConfiguration()
                configuration.width = max(1, Int(window.frame.width * CGFloat(window.pointPixelScale)))
                configuration.height = max(1, Int(window.frame.height * CGFloat(window.pointPixelScale)))
                captured = try await SCScreenshotManager.captureImage(contentFilter: filter, configuration: configuration)
                break
            }
            try await Task.sleep(for: .milliseconds(250))
        }
        guard let image = captured else {
            throw NSError(domain: "imgy-screenshot", code: 1, userInfo: [NSLocalizedDescriptionKey: "could not find an on-screen window for imgy PID \(pid)"])
        }
        let bitmap = NSBitmapImageRep(cgImage: image)
        guard let png = bitmap.representation(using: .png, properties: [:]) else {
            throw NSError(domain: "imgy-screenshot", code: 2, userInfo: [NSLocalizedDescriptionKey: "could not encode imgy screenshot"])
        }
        try png.write(to: output)
        exit(0)
    } catch {
        fputs("imgy screenshot failed: \(error)\n", stderr)
        exit(1)
    }
}

dispatchMain()
