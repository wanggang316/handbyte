// Render the Handbyte app icon into an Xcode app icon set.
//
// Usage: swift render-app-icon.swift <AppIcon.appiconset dir>
//
// The icon is drawn with Core Graphics at every size the set needs, so no
// binary source image is tracked. Rerun after a design change (make icon).
//
// Design (1024 pt canvas, Apple's macOS icon grid): a dark rounded square
// holds a 4x2 grid of green blocks, the "byte". The blocks on the right fade
// out, swept away by the green stroke below them: cleanup, one byte at a time.

import AppKit
import Foundation

struct Slot {
    let pixels: Int
    let points: Int
    let scale: Int
    var filename: String { "icon_\(points)x\(points)\(scale == 2 ? "@2x" : "").png" }
}

let slots: [Slot] = [
    Slot(pixels: 16, points: 16, scale: 1), Slot(pixels: 32, points: 16, scale: 2),
    Slot(pixels: 32, points: 32, scale: 1), Slot(pixels: 64, points: 32, scale: 2),
    Slot(pixels: 128, points: 128, scale: 1), Slot(pixels: 256, points: 128, scale: 2),
    Slot(pixels: 256, points: 256, scale: 1), Slot(pixels: 512, points: 256, scale: 2),
    Slot(pixels: 512, points: 512, scale: 1), Slot(pixels: 1024, points: 512, scale: 2),
]

func rgb(_ r: CGFloat, _ g: CGFloat, _ b: CGFloat, _ a: CGFloat = 1) -> CGColor {
    CGColor(srgbRed: r / 255, green: g / 255, blue: b / 255, alpha: a)
}

let green = rgb(34, 197, 94)
let greenBright = rgb(74, 222, 128)
let inkTop = rgb(22, 30, 48)
let inkBottom = rgb(9, 13, 22)

/// Draw the icon in a 1024 x 1024 design space with the origin at the top left.
func drawIcon(in ctx: CGContext) {
    let space = CGColorSpace(name: CGColorSpace.sRGB)!
    let plate = CGRect(x: 100, y: 100, width: 824, height: 824)
    let platePath = CGPath(roundedRect: plate, cornerWidth: 186, cornerHeight: 186, transform: nil)

    // Plate shadow, as in Apple's icon template.
    ctx.saveGState()
    ctx.setShadow(offset: CGSize(width: 0, height: 14), blur: 28, color: rgb(0, 0, 0, 0.42))
    ctx.setFillColor(inkBottom)
    ctx.addPath(platePath)
    ctx.fillPath()
    ctx.restoreGState()

    // Plate gradient and a soft green glow behind the sweep.
    ctx.saveGState()
    ctx.addPath(platePath)
    ctx.clip()
    let plateGradient = CGGradient(colorsSpace: space, colors: [inkTop, inkBottom] as CFArray, locations: [0, 1])!
    ctx.drawLinearGradient(plateGradient, start: CGPoint(x: 512, y: 100), end: CGPoint(x: 512, y: 924), options: [])
    let glow = CGGradient(colorsSpace: space, colors: [rgb(34, 197, 94, 0.22), rgb(34, 197, 94, 0)] as CFArray, locations: [0, 1])!
    ctx.drawRadialGradient(glow, startCenter: CGPoint(x: 760, y: 720), startRadius: 0,
                           endCenter: CGPoint(x: 760, y: 720), endRadius: 520, options: [])
    ctx.restoreGState()

    // The byte: 4 x 2 blocks. Opacity falls off to the right, where the
    // sweep has already passed.
    let block: CGFloat = 120
    let gap: CGFloat = 40
    let originX: CGFloat = 512 - (4 * block + 3 * gap) / 2
    let originY: CGFloat = 292
    let alphas: [[CGFloat]] = [[1, 1, 1, 0.42], [1, 0.42, 0.22, 0.1]]
    for row in 0..<2 {
        for col in 0..<4 {
            let rect = CGRect(x: originX + CGFloat(col) * (block + gap),
                              y: originY + CGFloat(row) * (block + gap),
                              width: block, height: block)
            let path = CGPath(roundedRect: rect, cornerWidth: 30, cornerHeight: 30, transform: nil)
            ctx.saveGState()
            ctx.setAlpha(alphas[row][col])
            ctx.setFillColor(green)
            ctx.addPath(path)
            ctx.fillPath()
            // Top highlight keeps the blocks from reading as flat stickers.
            ctx.addPath(path)
            ctx.clip()
            let sheen = CGGradient(colorsSpace: space, colors: [rgb(255, 255, 255, 0.26), rgb(255, 255, 255, 0)] as CFArray, locations: [0, 1])!
            ctx.drawLinearGradient(sheen, start: CGPoint(x: rect.midX, y: rect.minY), end: CGPoint(x: rect.midX, y: rect.maxY), options: [])
            ctx.restoreGState()
        }
    }

    // Crumbs drifting off the swept blocks.
    let crumbs: [(CGFloat, CGFloat, CGFloat, CGFloat)] = [
        (846, 352, 11, 0.34), (872, 316, 7, 0.22), (858, 486, 9, 0.26), (884, 528, 6, 0.16),
    ]
    for (x, y, r, a) in crumbs {
        ctx.setFillColor(rgb(34, 197, 94, a))
        ctx.fillEllipse(in: CGRect(x: x - r, y: y - r, width: 2 * r, height: 2 * r))
    }

    // The sweep: one stroke, left to right, with a bright head.
    let sweep = CGMutablePath()
    sweep.move(to: CGPoint(x: 236, y: 716))
    sweep.addQuadCurve(to: CGPoint(x: 776, y: 696), control: CGPoint(x: 520, y: 768))
    ctx.saveGState()
    ctx.setLineCap(.round)
    ctx.setLineWidth(46)
    ctx.setStrokeColor(green)
    ctx.addPath(sweep)
    ctx.strokePath()
    ctx.setShadow(offset: .zero, blur: 36, color: rgb(74, 222, 128, 0.8))
    ctx.setFillColor(greenBright)
    let head = CGPoint(x: 776, y: 696)
    ctx.fillEllipse(in: CGRect(x: head.x - 52, y: head.y - 52, width: 104, height: 104))
    ctx.restoreGState()
}

func render(pixels: Int) -> Data {
    let space = CGColorSpace(name: CGColorSpace.sRGB)!
    let ctx = CGContext(data: nil, width: pixels, height: pixels, bitsPerComponent: 8, bytesPerRow: 0,
                        space: space, bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue)!
    ctx.setAllowsAntialiasing(true)
    ctx.setShouldAntialias(true)
    ctx.interpolationQuality = .high
    // Flip to a top-left origin and scale the 1024 pt design to the target.
    ctx.translateBy(x: 0, y: CGFloat(pixels))
    ctx.scaleBy(x: CGFloat(pixels) / 1024, y: -CGFloat(pixels) / 1024)
    drawIcon(in: ctx)
    let image = ctx.makeImage()!
    let rep = NSBitmapImageRep(cgImage: image)
    return rep.representation(using: .png, properties: [:])!
}

guard CommandLine.arguments.count == 2 else {
    FileHandle.standardError.write("usage: render-app-icon.swift <AppIcon.appiconset dir>\n".data(using: .utf8)!)
    exit(2)
}
let outDir = URL(fileURLWithPath: CommandLine.arguments[1], isDirectory: true)
try FileManager.default.createDirectory(at: outDir, withIntermediateDirectories: true)

var images: [[String: String]] = []
for slot in slots {
    try render(pixels: slot.pixels).write(to: outDir.appendingPathComponent(slot.filename))
    images.append(["filename": slot.filename, "idiom": "mac", "scale": "\(slot.scale)x", "size": "\(slot.points)x\(slot.points)"])
}
let contents: [String: Any] = ["images": images, "info": ["author": "xcode", "version": 1]]
let json = try JSONSerialization.data(withJSONObject: contents, options: [.prettyPrinted, .sortedKeys])
try json.write(to: outDir.appendingPathComponent("Contents.json"))
print("wrote \(slots.count) images to \(outDir.path)")
