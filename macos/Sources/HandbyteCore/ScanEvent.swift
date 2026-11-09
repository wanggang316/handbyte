import Foundation

/// How much of a category the scan covered.
public enum CategoryState: String, Sendable, Codable {
    /// Every location was measured without errors.
    case complete
    /// Some location could not be listed or read; sizes are lower bounds.
    case partial
    /// No location of this category exists on this machine.
    case unavailable
    /// The scan was cancelled before every location was measured.
    case cancelled
}

public struct ScanProgress: Sendable, Hashable {
    public let currentPath: String
    /// Allocated bytes counted so far, including candidates below the minimum size.
    public let bytesSeen: Int64
    public let filesSeen: Int

    public init(currentPath: String, bytesSeen: Int64, filesSeen: Int) {
        self.currentPath = currentPath
        self.bytesSeen = bytesSeen
        self.filesSeen = filesSeen
    }
}

public struct ScanResult: Sendable, Hashable {
    /// Sorted by allocated bytes descending, then by path.
    public let candidates: [Candidate]
    /// One entry per category the configuration's rules cover.
    public let categories: [Category: CategoryState]
    public let bytesSeen: Int64
    public let filesSeen: Int
    public let cancelled: Bool

    public init(candidates: [Candidate], categories: [Category: CategoryState], bytesSeen: Int64, filesSeen: Int, cancelled: Bool) {
        self.candidates = candidates
        self.categories = categories
        self.bytesSeen = bytesSeen
        self.filesSeen = filesSeen
        self.cancelled = cancelled
    }

    /// Candidates whose size is a lower bound.
    public var partialCount: Int {
        candidates.filter(\.size.partial).count
    }
}

public enum ScanEvent: Sendable {
    case categoryStarted(Category)
    case candidateFound(Candidate)
    case progress(ScanProgress)
    case categoryFinished(Category, CategoryState)
    case finished(ScanResult)
}
