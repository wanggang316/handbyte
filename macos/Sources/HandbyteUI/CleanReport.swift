import Foundation
import HandbyteCore

/// One row of the report: the outcome plus the text the row shows.
public struct ReportItem: Sendable, Equatable, Identifiable {
    public let outcome: CleanOutcome
    /// Trash location for moved, reason for skipped, error for failed, else nil.
    public let detail: String?

    public var id: Candidate.ID { candidate.id }
    public var candidate: Candidate { outcome.candidate }

    public init(_ outcome: CleanOutcome) {
        self.outcome = outcome
        detail = switch outcome.status {
        case .moved(let url): url.fileSystemPath
        case .skipped(let reason): reason
        case .failed(let error): error
        case .removed, .previewed, .notAttempted: nil
        }
    }
}

/// `[CleanOutcome]` bucketed by status. Every outcome lands in exactly one
/// bucket, so the counts add up to `total`.
public struct CleanReport: Sendable, Equatable {
    public private(set) var moved: [ReportItem] = []
    public private(set) var removed: [ReportItem] = []
    public private(set) var previewed: [ReportItem] = []
    public private(set) var skipped: [ReportItem] = []
    public private(set) var failed: [ReportItem] = []
    public private(set) var notAttempted: [ReportItem] = []
    public let bytesClaimed: Int64
    public let total: Int

    public init(outcomes: [CleanOutcome]) {
        for outcome in outcomes {
            let item = ReportItem(outcome)
            switch outcome.status {
            case .moved: moved.append(item)
            case .removed: removed.append(item)
            case .previewed: previewed.append(item)
            case .skipped: skipped.append(item)
            case .failed: failed.append(item)
            case .notAttempted: notAttempted.append(item)
            }
        }
        bytesClaimed = outcomes.reduce(0) { $0 + $1.bytesClaimed }
        total = outcomes.count
    }

    /// The run was cancelled before every item was tried.
    public var isPartial: Bool {
        !notAttempted.isEmpty
    }
}
