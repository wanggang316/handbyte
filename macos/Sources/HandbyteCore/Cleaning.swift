import Foundation

public enum CleanAction: Sendable, Hashable, Codable {
    /// Move to the Trash through the system file manager, with "Put Back" metadata.
    case trash
    /// Delete without a Trash copy. Off by default; needs its own confirmation.
    case removePermanently
    /// Dry run. Changes nothing and claims no bytes.
    case preview
}

/// Result of applying a `CleanAction` to one candidate.
public struct CleanOutcome: Sendable, Hashable {
    public enum Status: Sendable, Hashable {
        case moved(URL)
        case removed
        case previewed
        case skipped(reason: String)
        case failed(error: String)
        case notAttempted
    }

    public let candidate: Candidate
    public let status: Status
    /// Bytes measured by the scan for a moved or removed item; zero otherwise.
    /// Never a measured free-space delta.
    public let bytesClaimed: Int64

    public init(candidate: Candidate, status: Status, bytesClaimed: Int64) {
        self.candidate = candidate
        self.status = status
        self.bytesClaimed = bytesClaimed
    }
}
