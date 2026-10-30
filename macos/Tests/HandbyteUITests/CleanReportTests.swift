import Foundation
import Testing
import HandbyteCore
@testable import HandbyteUI

@Suite("Clean report")
struct CleanReportTests {
    let a = Fixtures.candidate(path: "/Users/alice/Library/Caches/a", bytes: 100)
    let b = Fixtures.candidate(path: "/Users/alice/Library/Caches/b", bytes: 200)
    let c = Fixtures.candidate(path: "/Users/alice/Library/Caches/c", bytes: 300)
    let d = Fixtures.candidate(path: "/Users/alice/Library/Caches/d", bytes: 400)
    let e = Fixtures.candidate(path: "/Users/alice/Library/Caches/e", bytes: 500)
    let f = Fixtures.candidate(path: "/Users/alice/Library/Caches/f", bytes: 600)

    @Test func bucketsOutcomesByStatusAndSumsClaimedBytes() {
        let trash = URL(filePath: "/Users/alice/.Trash/a", directoryHint: .isDirectory)
        let report = CleanReport(outcomes: [
            CleanOutcome(candidate: a, status: .moved(trash), bytesClaimed: 100),
            CleanOutcome(candidate: b, status: .removed, bytesClaimed: 200),
            CleanOutcome(candidate: c, status: .previewed, bytesClaimed: 0),
            CleanOutcome(candidate: d, status: .skipped(reason: "Changed since scan"), bytesClaimed: 0),
            CleanOutcome(candidate: e, status: .failed(error: "Permission denied"), bytesClaimed: 0),
            CleanOutcome(candidate: f, status: .notAttempted, bytesClaimed: 0),
        ])

        #expect(report.moved.map(\.candidate) == [a])
        #expect(report.removed.map(\.candidate) == [b])
        #expect(report.previewed.map(\.candidate) == [c])
        #expect(report.skipped.map(\.candidate) == [d])
        #expect(report.skipped.map(\.detail) == ["Changed since scan"])
        #expect(report.failed.map(\.detail) == ["Permission denied"])
        #expect(report.notAttempted.map(\.candidate) == [f])
        #expect(report.bytesClaimed == 300)
        #expect(report.total == 6)
    }

    @Test func partialRunCountsAddUpToTheTotal() {
        let report = CleanReport(outcomes: [
            CleanOutcome(candidate: a, status: .moved(a.url), bytesClaimed: 100),
            CleanOutcome(candidate: b, status: .notAttempted, bytesClaimed: 0),
            CleanOutcome(candidate: c, status: .notAttempted, bytesClaimed: 0),
        ])
        #expect(report.moved.count + report.notAttempted.count == report.total)
        #expect(report.isPartial)
        #expect(report.bytesClaimed == 100)
    }

    @Test func emptyReportIsNotPartial() {
        #expect(!CleanReport(outcomes: []).isPartial)
    }
}
