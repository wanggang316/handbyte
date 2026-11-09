import Foundation

/// Reported after every candidate, including skipped and not-attempted ones,
/// so counters in the UI always add up to `total`.
public struct CleanProgress: Sendable, Hashable {
    public let completed: Int
    public let total: Int
    /// Running total of bytes claimed so far.
    public let bytesClaimed: Int64
    public let latest: CleanOutcome
}

/// The only code path that changes the file system.
///
/// Candidates are processed in order, one at a time. The policy runs again
/// immediately before each action; a `blocked` verdict becomes
/// `skipped(reason)` and nothing is touched. A `reviewOnly` verdict is a
/// selection restriction for the UI, not a refusal: the user selected the
/// item deliberately, so the cleaner acts on it. Cancellation is honoured
/// between items; the remaining candidates are reported `notAttempted`.
public struct Cleaner: Sendable {
    private let now: @Sendable () -> Date

    public init(now: @escaping @Sendable () -> Date = Date.init) {
        self.now = now
    }

    public func clean(
        _ candidates: [Candidate],
        action: CleanAction,
        policy: EligibilityPolicy,
        journal: (any Journal)?,
        progress: (@Sendable (CleanProgress) -> Void)? = nil
    ) async -> [CleanOutcome] {
        var outcomes: [CleanOutcome] = []
        outcomes.reserveCapacity(candidates.count)
        var bytesClaimed: Int64 = 0

        for candidate in candidates {
            let outcome: CleanOutcome
            if Task.isCancelled {
                outcome = CleanOutcome(candidate: candidate, status: .notAttempted, bytesClaimed: 0)
            } else {
                outcome = apply(action, to: candidate, policy: policy, journal: journal)
            }
            outcomes.append(outcome)
            bytesClaimed += outcome.bytesClaimed
            progress?(CleanProgress(
                completed: outcomes.count,
                total: candidates.count,
                bytesClaimed: bytesClaimed,
                latest: outcome
            ))
            await Task.yield()
        }
        return outcomes
    }

    private func apply(
        _ action: CleanAction,
        to candidate: Candidate,
        policy: EligibilityPolicy,
        journal: (any Journal)?
    ) -> CleanOutcome {
        if case .blocked(let reason) = policy.evaluate(candidate) {
            return CleanOutcome(candidate: candidate, status: .skipped(reason: reason), bytesClaimed: 0)
        }

        let status: CleanOutcome.Status
        do {
            switch action {
            case .preview:
                return CleanOutcome(candidate: candidate, status: .previewed, bytesClaimed: 0)
            case .trash:
                var destination: NSURL?
                try FileManager.default.trashItem(at: candidate.url, resultingItemURL: &destination)
                // The API documents a non-nil result on success; fall back to the
                // source URL rather than fail an action that already happened.
                status = .moved((destination as URL?) ?? candidate.url)
            case .removePermanently:
                try FileManager.default.removeItem(at: candidate.url)
                status = .removed
            }
        } catch {
            status = .failed(error: error.localizedDescription)
        }

        let outcome = CleanOutcome(
            candidate: candidate,
            status: status,
            bytesClaimed: Self.claimsBytes(status) ? candidate.size.allocatedBytes : 0
        )
        // The file system change already happened; a journal write failure
        // must not turn it into a reported failure.
        try? journal?.record(Self.entry(for: outcome, action: action, at: now()))
        return outcome
    }

    private static func claimsBytes(_ status: CleanOutcome.Status) -> Bool {
        switch status {
        case .moved, .removed: true
        case .previewed, .skipped, .failed, .notAttempted: false
        }
    }

    private static func entry(for outcome: CleanOutcome, action: CleanAction, at timestamp: Date) -> JournalEntry {
        let (result, detail): (JournalEntry.Outcome, String?) = switch outcome.status {
        case .moved(let destination): (.moved, destination.fileSystemPath)
        case .removed: (.removed, nil)
        case .failed(let error): (.failed, error)
        case .previewed, .skipped, .notAttempted:
            preconditionFailure("only applied actions are journaled")
        }
        return JournalEntry(
            timestamp: timestamp,
            action: action,
            outcome: result,
            ruleID: outcome.candidate.rule.id,
            bytes: outcome.bytesClaimed,
            path: outcome.candidate.url.fileSystemPath,
            detail: detail
        )
    }
}
