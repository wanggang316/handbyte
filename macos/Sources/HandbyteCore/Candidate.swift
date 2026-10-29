import Foundation

/// Device and inode of a file system object. Candidates are deduplicated and
/// re-checked before cleanup by identity, not by path string.
public struct FileIdentity: Sendable, Hashable, Codable {
    public let dev: UInt64
    public let ino: UInt64

    public init(dev: UInt64, ino: UInt64) {
        self.dev = dev
        self.ino = ino
    }
}

/// Measured size of a candidate. `partial` means some content could not be
/// read, so `allocatedBytes` is a lower bound.
public struct SizeReport: Sendable, Hashable, Codable {
    public let allocatedBytes: Int64
    public let fileCount: Int
    public let partial: Bool
    public let errorCount: Int

    public init(allocatedBytes: Int64, fileCount: Int, partial: Bool, errorCount: Int) {
        self.allocatedBytes = allocatedBytes
        self.fileCount = fileCount
        self.partial = partial
        self.errorCount = errorCount
    }
}

/// The policy verdict for a candidate. Only `eligible` items may be part of a
/// bulk selection.
public enum Eligibility: Sendable, Hashable, Codable {
    case eligible
    case reviewOnly(reason: String)
    case blocked(reason: String)
}

/// One cleanable item found by a scan.
public struct Candidate: Sendable, Hashable, Codable, Identifiable {
    public let id: String
    public let rule: Rule
    public let url: URL
    public let identity: FileIdentity
    public let size: SizeReport
    public let lastActivity: Date?
    public let eligibility: Eligibility

    public init(
        rule: Rule,
        url: URL,
        identity: FileIdentity,
        size: SizeReport,
        lastActivity: Date?,
        eligibility: Eligibility
    ) {
        self.id = Candidate.makeID(ruleID: rule.id, identity: identity)
        self.rule = rule
        self.url = url
        self.identity = identity
        self.size = size
        self.lastActivity = lastActivity
        self.eligibility = eligibility
    }

    /// Stable across rescans while the directory keeps its inode; distinct when
    /// two rules match the same directory.
    public static func makeID(ruleID: String, identity: FileIdentity) -> String {
        "\(ruleID)@\(identity.dev):\(identity.ino)"
    }
}
