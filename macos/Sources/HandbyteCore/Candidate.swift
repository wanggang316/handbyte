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

    // The location is encoded as a plain `path` string, not a `file://` URL,
    // so the JSON joins with the Go CLI's `Path` in parity checks.
    private enum CodingKeys: String, CodingKey {
        case id, rule, path, identity, size, lastActivity, eligibility
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        self.init(
            rule: try container.decode(Rule.self, forKey: .rule),
            url: URL(filePath: try container.decode(String.self, forKey: .path), directoryHint: .isDirectory),
            identity: try container.decode(FileIdentity.self, forKey: .identity),
            size: try container.decode(SizeReport.self, forKey: .size),
            lastActivity: try container.decodeIfPresent(Date.self, forKey: .lastActivity),
            eligibility: try container.decode(Eligibility.self, forKey: .eligibility)
        )
    }

    public func encode(to encoder: Encoder) throws {
        var container = encoder.container(keyedBy: CodingKeys.self)
        try container.encode(id, forKey: .id)
        try container.encode(rule, forKey: .rule)
        try container.encode(url.fileSystemPath, forKey: .path)
        try container.encode(identity, forKey: .identity)
        try container.encode(size, forKey: .size)
        try container.encodeIfPresent(lastActivity, forKey: .lastActivity)
        try container.encode(eligibility, forKey: .eligibility)
    }
}
