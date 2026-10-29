import Foundation
import HandbyteCore

/// Command line of `hbscan`. Flags mirror the Go CLI where both have them.
struct Options {
    var json = false
    var configuration = ScanConfiguration()

    static let usage = """
        usage: hbscan [--json] [--home <dir>] [--roots <a,b>] [--min <size>] [--no-projects] [--workers <n>]

          --json          print the scan as JSON instead of a table
          --home <dir>    home directory the rule catalog expands against
          --roots <a,b>   project roots to search for build artifacts
          --min <size>    drop candidates smaller than this (50M; K/M/G/T suffixes)
          --no-projects   skip project artifact discovery
          --workers <n>   directory walks in flight at once (6)
        """

    struct ParseError: Error, CustomStringConvertible {
        let description: String
    }

    init(arguments: [String]) throws {
        var iterator = arguments.makeIterator()
        while let argument = iterator.next() {
            let (flag, inlineValue) = Self.split(argument)
            func value() throws -> String {
                if let inlineValue { return inlineValue }
                guard let next = iterator.next() else {
                    throw ParseError(description: "\(flag) needs a value")
                }
                return next
            }
            switch flag {
            case "--json":
                json = true
            case "--no-projects":
                configuration.skipProjectScan = true
            case "--home":
                configuration.home = URL(filePath: try value(), directoryHint: .isDirectory)
            case "--roots":
                configuration.projectRoots = try value().split(separator: ",").map(String.init).filter { !$0.isEmpty }
            case "--min":
                let text = try value()
                guard let size = ByteCount.parse(text) else {
                    throw ParseError(description: "--min: cannot parse size \"\(text)\"")
                }
                configuration.minimumSize = size
            case "--workers":
                let text = try value()
                guard let workers = Int(text), workers >= 1 else {
                    throw ParseError(description: "--workers: expected a positive integer, got \"\(text)\"")
                }
                configuration.workers = workers
            case "-h", "--help":
                print(Self.usage)
                exit(0)
            default:
                throw ParseError(description: "unknown argument \(argument)")
            }
        }
    }

    /// `--min=50M` and `--min 50M` are both accepted.
    private static func split(_ argument: String) -> (flag: String, value: String?) {
        guard argument.hasPrefix("--"), let equals = argument.firstIndex(of: "=") else {
            return (argument, nil)
        }
        return (String(argument[..<equals]), String(argument[argument.index(after: equals)...]))
    }
}
