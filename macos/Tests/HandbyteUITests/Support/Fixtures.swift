import Foundation
import HandbyteCore

/// In-memory candidates for view-model tests. Nothing here touches the disk.
enum Fixtures {
    static func rule(
        id: String = "library-caches",
        category: HandbyteCore.Category = .systemCache,
        safety: Safety = .auto,
        summary: String = "Test rule."
    ) -> Rule {
        Rule(
            id: id,
            kind: .preset(pathTemplate: "~/Library/Caches", expandChildren: true),
            category: category,
            safety: safety,
            ownerBundleIDs: [],
            summary: summary,
            recovery: "Rebuilt on demand."
        )
    }

    /// FNV-1a of the path: distinct paths get distinct identities.
    private static func inode(for path: String) -> UInt64 {
        path.utf8.reduce(UInt64(14_695_981_039_346_656_037)) { ($0 ^ UInt64($1)) &* 1_099_511_628_211 }
    }

    static func candidate(
        path: String,
        rule: Rule = rule(),
        bytes: Int64 = 4096,
        files: Int = 1,
        partial: Bool = false,
        lastActivity: Date? = nil,
        eligibility: Eligibility = .eligible
    ) -> Candidate {
        Candidate(
            rule: rule,
            url: URL(filePath: path, directoryHint: .isDirectory),
            identity: FileIdentity(dev: 1, ino: inode(for: path)),
            size: SizeReport(allocatedBytes: bytes, fileCount: files, partial: partial, errorCount: partial ? 1 : 0),
            lastActivity: lastActivity,
            eligibility: eligibility
        )
    }
}
