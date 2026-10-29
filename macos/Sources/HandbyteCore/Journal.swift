// Operations journal: the audit trail of every file system change the app
// made or tried to make.
//
// One line per entry, seven tab-separated fields, newline-terminated:
//
//     <timestamp>\t<action>\t<outcome>\t<rule id>\t<bytes>\t<path>\t<detail>
//
// - timestamp: ISO-8601 in UTC with second precision, e.g. 2026-10-30T08:15:00Z
// - action: trash | removePermanently
// - outcome: moved | removed | failed
// - rule id: the catalog identifier, e.g. xcode-derived, artifact:node_modules
// - bytes: allocated bytes the scan measured; 0 for failed
// - path: the candidate path
// - detail: Trash destination for moved, error text for failed, else empty
//
// Inside path and detail a tab, newline, carriage return or backslash is
// written as \t, \n, \r or \\ so a line stays one line and splits on tabs.
// Preview and policy refusals change nothing and are not journaled. File
// contents are never logged. The Go CLI is expected to adopt the same file.

import Foundation

/// One applied (or attempted) file system change.
public struct JournalEntry: Sendable, Hashable {
    public enum Outcome: String, Sendable, Codable {
        case moved
        case removed
        case failed
    }

    public var timestamp: Date
    public var action: CleanAction
    public var outcome: Outcome
    public var ruleID: String
    public var bytes: Int64
    public var path: String
    /// Trash destination for `moved`, error text for `failed`, nil otherwise.
    public var detail: String?

    public init(timestamp: Date, action: CleanAction, outcome: Outcome, ruleID: String, bytes: Int64, path: String, detail: String?) {
        self.timestamp = timestamp
        self.action = action
        self.outcome = outcome
        self.ruleID = ruleID
        self.bytes = bytes
        self.path = path
        self.detail = detail
    }

    /// The journal line without its trailing newline.
    public var line: String {
        [
            timestamp.formatted(.iso8601),
            Self.actionName(action),
            outcome.rawValue,
            ruleID,
            String(bytes),
            Self.escape(path),
            Self.escape(detail ?? ""),
        ].joined(separator: "\t")
    }

    private static func actionName(_ action: CleanAction) -> String {
        switch action {
        case .trash: "trash"
        case .removePermanently: "removePermanently"
        case .preview: "preview"
        }
    }

    private static func escape(_ field: String) -> String {
        var out = ""
        out.reserveCapacity(field.utf8.count)
        for scalar in field.unicodeScalars {
            switch scalar {
            case "\\": out += "\\\\"
            case "\t": out += "\\t"
            case "\n": out += "\\n"
            case "\r": out += "\\r"
            default: out.unicodeScalars.append(scalar)
            }
        }
        return out
    }
}

public protocol Journal: Sendable {
    func record(_ entry: JournalEntry) throws
}

/// Appends entries to a log file, creating the file and its directory on first use.
public struct OperationsJournal: Journal {
    public let fileURL: URL

    public init(fileURL: URL) {
        self.fileURL = fileURL
    }

    /// `~/Library/Logs/handbyte/operations.log` for the given home.
    public static func defaultURL(home: URL) -> URL {
        home.appending(path: "Library/Logs/handbyte/operations.log")
    }

    public func record(_ entry: JournalEntry) throws {
        let fileManager = FileManager.default
        try fileManager.createDirectory(at: fileURL.deletingLastPathComponent(), withIntermediateDirectories: true)
        if !fileManager.fileExists(atPath: fileURL.path(percentEncoded: false)) {
            try Data().write(to: fileURL, options: .withoutOverwriting)
        }
        let handle = try FileHandle(forWritingTo: fileURL)
        defer { try? handle.close() }
        try handle.seekToEnd()
        try handle.write(contentsOf: Data((entry.line + "\n").utf8))
    }
}
