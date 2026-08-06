#!/usr/bin/env swift

import AppKit
import ImageIO
import UniformTypeIdentifiers

let width = 1200
let height = 680
let output = CommandLine.arguments.dropFirst().first ?? "docs/assets/amh-demo.gif"

struct Scene {
    var senderLines: [(String, NSColor)] = []
    var receiverLines: [(String, NSColor)] = []
    var capsuleProgress: CGFloat? = nil
    var capsuleVisible = false
    var receiverActive = false
    var duration = 0.12
}

let background = NSColor(calibratedRed: 0.035, green: 0.063, blue: 0.102, alpha: 1)
let panel = NSColor(calibratedRed: 0.065, green: 0.102, blue: 0.157, alpha: 1)
let panelBorder = NSColor(calibratedRed: 0.145, green: 0.216, blue: 0.298, alpha: 1)
let text = NSColor(calibratedRed: 0.90, green: 0.94, blue: 0.97, alpha: 1)
let muted = NSColor(calibratedRed: 0.52, green: 0.61, blue: 0.69, alpha: 1)
let cyan = NSColor(calibratedRed: 0.35, green: 0.84, blue: 0.87, alpha: 1)
let green = NSColor(calibratedRed: 0.49, green: 0.91, blue: 0.62, alpha: 1)
let amber = NSColor(calibratedRed: 0.95, green: 0.76, blue: 0.36, alpha: 1)
let blue = NSColor(calibratedRed: 0.43, green: 0.66, blue: 0.96, alpha: 1)

let titleFont = NSFont.systemFont(ofSize: 27, weight: .bold)
let subtitleFont = NSFont.monospacedSystemFont(ofSize: 14, weight: .medium)
let labelFont = NSFont.monospacedSystemFont(ofSize: 13, weight: .semibold)
let lineFont = NSFont.monospacedSystemFont(ofSize: 16, weight: .regular)
let lineBoldFont = NSFont.monospacedSystemFont(ofSize: 16, weight: .semibold)
let capsuleFont = NSFont.monospacedSystemFont(ofSize: 15, weight: .semibold)

func roundedRect(_ rect: NSRect, radius: CGFloat, fill: NSColor, stroke: NSColor? = nil, lineWidth: CGFloat = 1) {
    let path = NSBezierPath(roundedRect: rect, xRadius: radius, yRadius: radius)
    fill.setFill()
    path.fill()
    if let stroke {
        stroke.setStroke()
        path.lineWidth = lineWidth
        path.stroke()
    }
}

func drawText(_ value: String, at point: NSPoint, font: NSFont, color: NSColor) {
    value.draw(at: point, withAttributes: [
        .font: font,
        .foregroundColor: color,
    ])
}

func drawPanel(_ rect: NSRect, label: String, active: Bool, lines: [(String, NSColor)]) {
    roundedRect(rect, radius: 18, fill: panel, stroke: active ? cyan : panelBorder, lineWidth: active ? 2 : 1)

    let labelWidth: CGFloat = label == "SENDER · CODEX" ? 142 : 208
    roundedRect(NSRect(x: rect.minX + 24, y: rect.maxY - 48, width: labelWidth, height: 26), radius: 7,
                fill: active ? NSColor(calibratedRed: 0.09, green: 0.24, blue: 0.26, alpha: 1) : NSColor(calibratedRed: 0.10, green: 0.15, blue: 0.22, alpha: 1))
    drawText(label, at: NSPoint(x: rect.minX + 34, y: rect.maxY - 43), font: labelFont, color: active ? cyan : muted)

    let dotY = rect.maxY - 42
    for (index, color) in [
        NSColor(calibratedRed: 0.95, green: 0.36, blue: 0.32, alpha: 1),
        NSColor(calibratedRed: 0.95, green: 0.73, blue: 0.30, alpha: 1),
        NSColor(calibratedRed: 0.36, green: 0.80, blue: 0.44, alpha: 1),
    ].enumerated() {
        color.setFill()
        NSBezierPath(ovalIn: NSRect(x: rect.maxX - 78 + CGFloat(index * 20), y: dotY, width: 9, height: 9)).fill()
    }

    let divider = NSBezierPath()
    divider.move(to: NSPoint(x: rect.minX + 22, y: rect.maxY - 66))
    divider.line(to: NSPoint(x: rect.maxX - 22, y: rect.maxY - 66))
    panelBorder.setStroke()
    divider.lineWidth = 1
    divider.stroke()

    var y = rect.maxY - 102
    for (line, color) in lines {
        let isHeading = line == "Mission Brief" || line.hasPrefix("Continue with")
        drawText(line, at: NSPoint(x: rect.minX + 28, y: y), font: isHeading ? lineBoldFont : lineFont, color: color)
        y -= 31
    }
}

func drawCapsule(progress: CGFloat) {
    let startX: CGFloat = 472
    let endX: CGFloat = 674
    let x = startX + (endX - startX) * progress
    let y: CGFloat = 520

    let trail = NSBezierPath()
    trail.move(to: NSPoint(x: startX - 42, y: y + 19))
    trail.line(to: NSPoint(x: endX + 50, y: y + 19))
    panelBorder.withAlphaComponent(0.75).setStroke()
    trail.lineWidth = 2
    trail.stroke()

    roundedRect(NSRect(x: x, y: y, width: 134, height: 38), radius: 12,
                fill: NSColor(calibratedRed: 0.10, green: 0.26, blue: 0.28, alpha: 1), stroke: cyan, lineWidth: 2)
    drawText("mission.amh", at: NSPoint(x: x + 15, y: y + 10), font: capsuleFont, color: text)
}

func render(_ scene: Scene) -> CGImage {
    let image = NSImage(size: NSSize(width: width, height: height))
    image.lockFocus()
    NSGraphicsContext.current?.imageInterpolation = .none

    background.setFill()
    NSRect(x: 0, y: 0, width: width, height: height).fill()

    // Sparse grid keeps the background dimensional while remaining GIF-friendly.
    NSColor(calibratedRed: 0.07, green: 0.11, blue: 0.16, alpha: 1).setStroke()
    for x in stride(from: 0, through: width, by: 40) {
        let path = NSBezierPath()
        path.move(to: NSPoint(x: x, y: 0))
        path.line(to: NSPoint(x: x, y: height))
        path.lineWidth = 0.5
        path.stroke()
    }
    for y in stride(from: 0, through: height, by: 40) {
        let path = NSBezierPath()
        path.move(to: NSPoint(x: 0, y: y))
        path.line(to: NSPoint(x: width, y: y))
        path.lineWidth = 0.5
        path.stroke()
    }

    drawText("AGENT MISSION HANDOFF", at: NSPoint(x: 48, y: 623), font: titleFont, color: text)
    drawText("ONE FILE · TWO PROMPTS · WRITABLE SESSION", at: NSPoint(x: 49, y: 594), font: subtitleFont, color: muted)

    drawPanel(NSRect(x: 48, y: 62, width: 520, height: 492), label: "SENDER · CODEX", active: !scene.receiverActive, lines: scene.senderLines)
    drawPanel(NSRect(x: 632, y: 62, width: 520, height: 492), label: "RECEIVER · CLAUDE CODE", active: scene.receiverActive, lines: scene.receiverLines)

    if scene.capsuleVisible {
        drawCapsule(progress: scene.capsuleProgress ?? 0)
    }

    image.unlockFocus()
    var rect = NSRect(x: 0, y: 0, width: width, height: height)
    return image.cgImage(forProposedRect: &rect, context: nil, hints: nil)!
}

var scenes: [Scene] = []
func add(_ scene: Scene, hold: Double = 0.12) {
    var copy = scene
    copy.duration = hold
    scenes.append(copy)
}

var scene = Scene()
add(scene, hold: 0.7)

let senderSteps: [[(String, NSColor)]] = [
    [("You", muted)],
    [("You", muted), ("Package the current task as an AMH file.", text)],
    [("You", muted), ("Package the current task as an AMH file.", text), ("", text), ("Agent", cyan)],
    [("You", muted), ("Package the current task as an AMH file.", text), ("", text), ("Agent", cyan), ("$ amh pack", blue)],
    [("You", muted), ("Package the current task as an AMH file.", text), ("", text), ("Agent", cyan), ("$ amh pack", blue), ("✓ Packed current Codex mission", green)],
    [("You", muted), ("Package the current task as an AMH file.", text), ("", text), ("Agent", cyan), ("$ amh pack", blue), ("✓ Packed current Codex mission", green), ("  37 turns · 11 capabilities", muted), ("  → mission.amh", amber)],
]
for (index, lines) in senderSteps.enumerated() {
    scene.senderLines = lines
    add(scene, hold: index == senderSteps.count - 1 ? 0.8 : 0.28)
}

scene.capsuleVisible = true
for step in 0...12 {
    scene.capsuleProgress = CGFloat(step) / 12
    if step > 6 { scene.receiverActive = true }
    add(scene, hold: 0.07)
}
add(scene, hold: 0.45)

let receiverSteps: [[(String, NSColor)]] = [
    [("You", muted)],
    [("You", muted), ("Continue this task.", text)],
    [("You", muted), ("Continue this task.", text), ("", text), ("Agent", cyan)],
    [("You", muted), ("Continue this task.", text), ("", text), ("Agent", cyan), ("$ amh continue mission.amh", blue)],
    [("Mission Brief", cyan)],
    [("Mission Brief", cyan), ("Objective: fix checkout timeouts", text)],
    [("Mission Brief", cyan), ("Objective: fix checkout timeouts", text), ("History: 37 turns from Codex", text)],
    [("Mission Brief", cyan), ("Objective: fix checkout timeouts", text), ("History: 37 turns from Codex", text), ("Completed: reproduced pool exhaustion", green)],
    [("Mission Brief", cyan), ("Objective: fix checkout timeouts", text), ("History: 37 turns from Codex", text), ("Completed: reproduced pool exhaustion", green), ("Open: likely connection leak", amber)],
    [("Mission Brief", cyan), ("Objective: fix checkout timeouts", text), ("History: 37 turns from Codex", text), ("Completed: reproduced pool exhaustion", green), ("Open: likely connection leak", amber), ("Next: inspect pool lifecycle", blue)],
    [("Mission Brief", cyan), ("Objective: fix checkout timeouts", text), ("History: 37 turns from Codex", text), ("Completed: reproduced pool exhaustion", green), ("Open: likely connection leak", amber), ("Next: inspect pool lifecycle", blue), ("", text), ("Continue with this next action?", text)],
]
for (index, lines) in receiverSteps.enumerated() {
    scene.receiverLines = lines
    add(scene, hold: index == receiverSteps.count - 1 ? 2.3 : 0.24)
}

let outputURL = URL(fileURLWithPath: output)
try FileManager.default.createDirectory(at: outputURL.deletingLastPathComponent(), withIntermediateDirectories: true)
guard let destination = CGImageDestinationCreateWithURL(outputURL as CFURL, UTType.gif.identifier as CFString, scenes.count, nil) else {
    fatalError("Unable to create GIF destination")
}

CGImageDestinationSetProperties(destination, [
    kCGImagePropertyGIFDictionary: [kCGImagePropertyGIFLoopCount: 0]
] as CFDictionary)

for scene in scenes {
    let properties: [CFString: Any] = [
        kCGImagePropertyGIFDictionary: [
            kCGImagePropertyGIFDelayTime: scene.duration,
            kCGImagePropertyGIFUnclampedDelayTime: scene.duration,
        ]
    ]
    CGImageDestinationAddImage(destination, render(scene), properties as CFDictionary)
}

guard CGImageDestinationFinalize(destination) else {
    fatalError("Unable to finalize GIF")
}

print("Generated \(output) with \(scenes.count) frames")
