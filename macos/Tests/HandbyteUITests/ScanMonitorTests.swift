import Foundation
import Testing
import HandbyteCore
@testable import HandbyteUI

@Suite("Scan monitor")
struct ScanMonitorTests {
    @Test func tracksCategoryStatesCountersAndFoundCandidates() {
        var monitor = ScanMonitor(rules: [
            Fixtures.rule(id: "a", category: .xcode),
            Fixtures.rule(id: "b", category: .devCache),
            Fixtures.rule(id: "c", category: .trash),
        ])
        #expect(monitor.rows.map(\.category) == [.devCache, .xcode, .trash])
        #expect(monitor.rows.allSatisfy { $0.state == .pending })

        monitor.apply(.categoryStarted(.xcode))
        #expect(monitor.row(.xcode)?.state == .running)

        let found = Fixtures.candidate(path: "/Users/alice/Library/Developer/Xcode/DerivedData", bytes: 10)
        monitor.apply(.candidateFound(found))
        monitor.apply(.progress(ScanProgress(currentPath: "/Users/alice/x", bytesSeen: 4096, filesSeen: 7)))
        #expect(monitor.found == [found])
        #expect(monitor.bytesSeen == 4096)
        #expect(monitor.filesSeen == 7)
        #expect(monitor.currentPath == "/Users/alice/x")

        monitor.apply(.categoryFinished(.xcode, .partial))
        monitor.apply(.categoryFinished(.trash, .unavailable))
        #expect(monitor.row(.xcode)?.state == .finished(.partial))
        #expect(monitor.row(.trash)?.state == .finished(.unavailable))
        #expect(monitor.row(.devCache)?.state == .pending)
    }

    @Test func finishedResultOverridesEveryRow() {
        var monitor = ScanMonitor(rules: [Fixtures.rule(id: "a", category: .xcode)])
        monitor.apply(.finished(ScanResult(
            candidates: [], categories: [.xcode: .cancelled], bytesSeen: 1, filesSeen: 1, cancelled: true
        )))
        #expect(monitor.row(.xcode)?.state == .finished(.cancelled))
    }
}
