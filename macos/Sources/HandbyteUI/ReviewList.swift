import Foundation
import HandbyteCore

/// One expandable section of the review list.
public struct CandidateGroup: Sendable, Equatable, Identifiable {
    public let category: HandbyteCore.Category
    public let candidates: [Candidate]

    public var id: HandbyteCore.Category { category }

    public var totalBytes: Int64 {
        candidates.reduce(0) { $0 + $1.size.allocatedBytes }
    }

    /// Some member size is a lower bound, so the total is one too.
    public var partial: Bool {
        candidates.contains(where: \.size.partial)
    }
}

/// The review list as the user sees it: grouped by category, filtered by the
/// search field, sorted. Totals always cover the whole result.
public struct ReviewList: Sendable, Equatable {
    public enum SortOrder: String, Sendable, CaseIterable, Identifiable {
        case size
        case name

        public var id: String { rawValue }
    }

    public let candidates: [Candidate]
    public var query = ""
    public var sortOrder: SortOrder = .size

    public init(candidates: [Candidate]) {
        self.candidates = candidates
    }

    public var totalBytes: Int64 {
        candidates.reduce(0) { $0 + $1.size.allocatedBytes }
    }

    public var totalCount: Int { candidates.count }

    public var visibleCount: Int {
        groups.reduce(0) { $0 + $1.candidates.count }
    }

    /// Non-empty groups in catalog order (`Category.allCases`).
    public var groups: [CandidateGroup] {
        let visible = candidates.filter(matchesQuery)
        return HandbyteCore.Category.allCases.compactMap { category in
            let members = visible.filter { $0.rule.category == category }
            guard !members.isEmpty else { return nil }
            return CandidateGroup(category: category, candidates: members.sorted(by: areInOrder))
        }
    }

    private func matchesQuery(_ candidate: Candidate) -> Bool {
        let needle = query.trimmingCharacters(in: .whitespaces)
        guard !needle.isEmpty else { return true }
        return [candidate.url.fileSystemPath, candidate.rule.id, candidate.rule.summary]
            .contains { $0.localizedCaseInsensitiveContains(needle) }
    }

    private func areInOrder(_ lhs: Candidate, _ rhs: Candidate) -> Bool {
        switch sortOrder {
        case .size:
            if lhs.size.allocatedBytes != rhs.size.allocatedBytes {
                return lhs.size.allocatedBytes > rhs.size.allocatedBytes
            }
            return lhs.url.fileSystemPath < rhs.url.fileSystemPath
        case .name:
            let order = lhs.url.lastPathComponent.localizedStandardCompare(rhs.url.lastPathComponent)
            if order != .orderedSame { return order == .orderedAscending }
            return lhs.url.fileSystemPath < rhs.url.fileSystemPath
        }
    }
}
