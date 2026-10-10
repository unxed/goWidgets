import AppKit
import CoreGraphics
import Foundation

guard CommandLine.arguments.count == 3,
      let pidValue = Int32(CommandLine.arguments[1]) else {
    fatalError("usage: capture-imgy-macos.swift PID output.png")
}

let pid = pid_t(pidValue)
guard let app = NSRunningApplication(processIdentifier: pid) else {
    fatalError("imgy process is not running")
}
let output = URL(fileURLWithPath: CommandLine.arguments[2])
app.activate(options: [.activateIgnoringOtherApps])

for _ in 0..<80 {
    let windows = CGWindowListCopyWindowInfo([.optionOnScreenOnly, .excludeDesktopElements], kCGNullWindowID) as? [[String: Any]] ?? []
    if let window = windows.first(where: {
        ($0[kCGWindowOwnerPID as String] as? Int) == Int(pid) &&
        ($0[kCGWindowLayer as String] as? Int) == 0
    }),
       let windowNumber = window[kCGWindowNumber as String] as? Int {
        Thread.sleep(forTimeInterval: 1.0)
        guard let image = CGWindowListCreateImage(.null, .optionIncludingWindow, CGWindowID(windowNumber), [.bestResolution]) else {
            continue
        }
        let bitmap = NSBitmapImageRep(cgImage: image)
        guard let png = bitmap.representation(using: .png, properties: [:]) else {
            fatalError("could not encode imgy screenshot")
        }
        try png.write(to: output)
        exit(0)
    }
    Thread.sleep(forTimeInterval: 0.25)
}

fatalError("could not find an on-screen window for imgy PID \(pid)")
