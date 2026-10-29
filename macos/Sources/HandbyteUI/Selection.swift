import Foundation
import HandbyteCore

/// Which candidates the user picked for cleanup.
///
/// Rules, in the order they bite:
/// - A `blocked` candidate is never selectable. A `reviewOnly` candidate can be
///   picked one by one but is skipped by every bulk operation.
/// - Bulk operations (`selectAllSafe`, `toggleGroup`) add only `eligible`
///   candidates whose rule is `Safety.auto`.
/// - A parent and a child path cannot both be selected. The latest pick wins:
///   selecting one deselects the other, so the user sees the change instead
///   of a disabled row. Bulk operations pick the outermost of nested
///   candidates and drop their descendants.
public struct Selection: Sendable, Equatable {
    public enum GroupState: Sendable, Equatable {
        case none
        case some
        case all
    }

    public let candidates: [Candidate]
    public private(set) var selectedIDs: Set<Candidate.ID> = []
    private let byID: [Candidate.ID: Candidate]

    public init(candidates: [Candidate]) {
        self.candidates = candidates
        byID = Dictionary(candidates.map { ($0.id, $0) }, uniquingKeysWith: { first, _ in first })
    }

    public func isSelected(_ id: Candidate.ID) -> Bool {
        selectedIDs.contains(id)
    }

    /// In the order of `candidates`.
    public var selectedCandidates: [Candidate] {
        candidates.filter { selectedIDs.contains($0.id) }
    }

    public var selectedBytes: Int64 {
        selectedCandidates.reduce(0) { $0 + $1.size.allocatedBytes }
    }

    /// False only for `blocked`; the UI disables the row and shows the reason.
    public func canSelect(_ candidate: Candidate) -> Bool {
        if case .blocked = candidate.eligibility { return false }
        return true
    }

    /// Safe enough for a bulk pick: eligible and regenerable without review.
    public static func isBulkSelectable(_ candidate: Candidate) -> Bool {
        candidate.eligibility == .eligible && candidate.rule.safety == .auto
    }

    /// Selected candidates that are an ancestor or a descendant of `candidate`.
    public func overlapping(_ candidate: Candidate) -> [Candidate] {
        selectedCandidates.filter { other in
            other.id != candidate.id && PathRelation.areNested(other.url, candidate.url)
        }
    }

    public func hasBulkSelectable(_ category: HandbyteCore.Category) -> Bool {
        candidates.contains { $0.rule.category == category && Self.isBulkSelectable($0) }
    }

    /// `.all` when every bulk-selectable member is selected (hand-picked review
    /// members do not count against it), `.none` when nothing in the group is
    /// selected, `.some` otherwise.
    public func groupState(_ category: HandbyteCore.Category) -> GroupState {
        let members = candidates.filter { $0.rule.category == category }
        let selected = members.filter { selectedIDs.contains($0.id) }
        if selected.isEmpty { return .none }
        let bulk = members.filter(Self.isBulkSelectable)
        let allBulkSelected = !bulk.isEmpty && bulk.allSatisfy { selectedIDs.contains($0.id) }
        return allBulkSelected ? .all : .some
    }

    public mutating func toggle(_ id: Candidate.ID) {
        guard let candidate = byID[id], canSelect(candidate) else { return }
        if selectedIDs.contains(id) {
            selectedIDs.remove(id)
        } else {
            select(candidate)
        }
    }

    public mutating func toggleGroup(_ category: HandbyteCore.Category) {
        if groupState(category) == .all {
            for candidate in candidates where candidate.rule.category == category {
                selectedIDs.remove(candidate.id)
            }
        } else {
            bulkSelect(candidates.filter { $0.rule.category == category })
        }
    }

    public mutating func selectAllSafe() {
        bulkSelect(candidates)
    }

    public mutating func clear() {
        selectedIDs.removeAll()
    }

    private mutating func select(_ candidate: Candidate) {
        for other in overlapping(candidate) {
            selectedIDs.remove(other.id)
        }
        selectedIDs.insert(candidate.id)
    }

    private mutating func bulkSelect(_ pool: [Candidate]) {
        let safe = pool.filter(Self.isBulkSelectable)
        let outermost = safe.filter { candidate in
            !safe.contains { $0.id != candidate.id && PathRelation.isAncestor($0.url, of: candidate.url) }
        }
        for candidate in outermost {
            select(candidate)
        }
    }
}

/// Parent/child tests on standardized paths, without touching the disk.
public enum PathRelation {
    public static func isAncestor(_ ancestor: URL, of descendant: URL) -> Bool {
        let a = components(ancestor)
        let d = components(descendant)
        return a.count < d.count && Array(d.prefix(a.count)) == a
    }

    public static func areNested(_ lhs: URL, _ rhs: URL) -> Bool {
        isAncestor(lhs, of: rhs) || isAncestor(rhs, of: lhs)
    }

    private static func components(_ url: URL) -> [String] {
        url.standardizedFileURL.pathComponents
    }
}
