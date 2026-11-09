import Foundation
import HandbyteCore

/// Live view of a scan, rebuilt from `ScanEvent`s.
public struct ScanMonitor: Sendable, Equatable {
    public enum RowState: Sendable, Equatable {
        case pending
        case running
        case finished(CategoryState)
    }

    public struct Row: Sendable, Equatable, Identifiable {
        public let category: HandbyteCore.Category
        public var state: RowState = .pending

        public var id: HandbyteCore.Category { category }
    }

    /// Categories the configured rules cover, in catalog order.
    public private(set) var rows: [Row]
    public private(set) var found: [Candidate] = []
    public private(set) var bytesSeen: Int64 = 0
    public private(set) var filesSeen = 0
    public private(set) var currentPath = ""

    public init(rules: [Rule]) {
        let covered = Set(rules.map(\.category))
        rows = HandbyteCore.Category.allCases.filter(covered.contains).map { Row(category: $0) }
    }

    public func row(_ category: HandbyteCore.Category) -> Row? {
        rows.first { $0.category == category }
    }

    public mutating func apply(_ event: ScanEvent) {
        switch event {
        case .categoryStarted(let category):
            set(category, .running)
        case .candidateFound(let candidate):
            found.append(candidate)
        case .progress(let progress):
            bytesSeen = progress.bytesSeen
            filesSeen = progress.filesSeen
            currentPath = progress.currentPath
        case .categoryFinished(let category, let state):
            set(category, .finished(state))
        case .finished(let result):
            bytesSeen = result.bytesSeen
            filesSeen = result.filesSeen
            for (category, state) in result.categories {
                set(category, .finished(state))
            }
        }
    }

    private mutating func set(_ category: HandbyteCore.Category, _ state: RowState) {
        guard let index = rows.firstIndex(where: { $0.category == category }) else { return }
        rows[index].state = state
    }
}
