import Foundation
import HandbyteCore

/// JSON document printed by `hbscan --json`.
struct ScanDocument: Encodable {
    let home: String
    let partialCount: Int
    let cancelled: Bool
    let bytesSeen: Int64
    let filesSeen: Int
    let categories: [String: CategoryState]
    let candidates: [Candidate]

    init(home: URL, result: ScanResult) {
        self.home = home.fileSystemPath
        partialCount = result.partialCount
        cancelled = result.cancelled
        bytesSeen = result.bytesSeen
        filesSeen = result.filesSeen
        categories = Dictionary(uniqueKeysWithValues: result.categories.map { ($0.key.rawValue, $0.value) })
        candidates = result.candidates
    }
}

func formatBytes(_ bytes: Int64) -> String {
    let units = ["B", "K", "M", "G", "T"]
    var value = Double(bytes)
    var unit = 0
    while value >= 1024, unit < units.count - 1 {
        value /= 1024
        unit += 1
    }
    return unit == 0 ? "\(bytes) B" : String(format: "%.1f %@", value, units[unit])
}

func printTable(_ result: ScanResult) {
    for candidate in result.candidates {
        let bound = candidate.size.partial ? ">=" : "  "
        print("\(bound)\(formatBytes(candidate.size.allocatedBytes).padding(toLength: 10, withPad: " ", startingAt: 0)) "
            + "\(candidate.rule.id.padding(toLength: 22, withPad: " ", startingAt: 0)) \(candidate.url.fileSystemPath)")
    }
    let states = result.categories
        .sorted { $0.key.rawValue < $1.key.rawValue }
        .map { "\($0.key.rawValue): \($0.value.rawValue)" }
        .joined(separator: ", ")
    print("\(result.candidates.count) candidates, \(result.partialCount) partial; \(states)")
}

let options: Options
do {
    options = try Options(arguments: Array(CommandLine.arguments.dropFirst()))
} catch {
    FileHandle.standardError.write(Data("hbscan: \(error)\n\(Options.usage)\n".utf8))
    exit(2)
}

let result = await DiskScanner().collect(options.configuration)

if options.json {
    let encoder = JSONEncoder()
    encoder.outputFormatting = [.prettyPrinted, .sortedKeys, .withoutEscapingSlashes]
    encoder.dateEncodingStrategy = .iso8601
    let data = try encoder.encode(ScanDocument(home: options.configuration.home, result: result))
    FileHandle.standardOutput.write(data)
    FileHandle.standardOutput.write(Data("\n".utf8))
} else {
    printTable(result)
}
