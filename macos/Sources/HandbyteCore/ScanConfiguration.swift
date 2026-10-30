import Foundation

/// What one scan looks at. Defaults mirror the Go CLI's `scanner.Default()`.
public struct ScanConfiguration: Sendable, Hashable {
    /// Directory the catalog's `~` expands to. Injectable so tests use a fixture.
    public var home: URL
    /// Roots searched for project artifacts, as templates (`~/dev`) or absolute paths.
    public var projectRoots: [String] = Catalog.defaultProjectRoots
    /// Candidates with fewer allocated bytes are dropped (strict less-than).
    public var minimumSize: Int64 = 50 * 1024 * 1024
    public var skipProjectScan = false
    /// Directory walks in flight at once.
    public var workers = 6
    public var rules: [Rule] = Catalog.rules

    public init(home: URL = FileManager.default.homeDirectoryForCurrentUser) {
        self.home = home
    }
}

/// Parses the Go CLI's size syntax: an integer with an optional K/M/G/T suffix
/// (powers of 1024, case-insensitive), or plain bytes.
public enum ByteCount {
    public static func parse(_ text: String) -> Int64? {
        let trimmed = text.trimmingCharacters(in: .whitespaces)
        guard let last = trimmed.last else { return nil }
        let multipliers: [Character: Int64] = ["k": 1 << 10, "m": 1 << 20, "g": 1 << 30, "t": 1 << 40]
        if let multiplier = multipliers[Character(last.lowercased())] {
            guard let value = Int64(trimmed.dropLast()), value >= 0 else { return nil }
            return value.multipliedReportingOverflow(by: multiplier).overflow ? nil : value * multiplier
        }
        guard let value = Int64(trimmed), value >= 0 else { return nil }
        return value
    }
}
