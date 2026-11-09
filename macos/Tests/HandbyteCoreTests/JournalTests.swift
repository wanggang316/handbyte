import Foundation
import Testing
@testable import HandbyteCore

@Suite("OperationsJournal")
struct OperationsJournalTests {
    @Test func defaultLocationIsUnderLibraryLogs() {
        let home = URL(filePath: "/Volumes/Test/alice", directoryHint: .isDirectory)
        #expect(OperationsJournal.defaultURL(home: home).path == "/Volumes/Test/alice/Library/Logs/handbyte/operations.log")
    }

    @Test func appendsOneTabSeparatedLinePerEntryAndCreatesTheDirectory() throws {
        let fixture = try CleaningFixture()
        let journal = OperationsJournal(fileURL: OperationsJournal.defaultURL(home: fixture.home))
        let stamp = Date(timeIntervalSince1970: 1_700_000_000)

        try journal.record(JournalEntry(
            timestamp: stamp, action: .trash, outcome: .moved, ruleID: "xcode-derived", bytes: 4096,
            path: "/Users/alice/Library/Developer/Xcode/DerivedData", detail: "/Users/alice/.Trash/DerivedData"))
        try journal.record(JournalEntry(
            timestamp: stamp, action: .removePermanently, outcome: .failed, ruleID: "npm-cache", bytes: 0,
            path: "/Users/alice/.npm/_cacache", detail: "Operation not permitted"))

        let lines = try String(contentsOf: journal.fileURL, encoding: .utf8).split(separator: "\n", omittingEmptySubsequences: false)
        #expect(lines == [
            "2023-11-14T22:13:20Z\ttrash\tmoved\txcode-derived\t4096\t/Users/alice/Library/Developer/Xcode/DerivedData\t/Users/alice/.Trash/DerivedData",
            "2023-11-14T22:13:20Z\tremovePermanently\tfailed\tnpm-cache\t0\t/Users/alice/.npm/_cacache\tOperation not permitted",
            "",
        ])
    }

    @Test func escapesSeparatorsInsidePathAndDetail() throws {
        let fixture = try CleaningFixture()
        let journal = OperationsJournal(fileURL: fixture.root.appending(path: "ops.log"))

        try journal.record(JournalEntry(
            timestamp: Date(timeIntervalSince1970: 0), action: .trash, outcome: .removed, ruleID: "r", bytes: 1,
            path: "/tmp/a\tb\nc\\d", detail: nil))

        let text = try String(contentsOf: journal.fileURL, encoding: .utf8)
        #expect(text == "1970-01-01T00:00:00Z\ttrash\tremoved\tr\t1\t/tmp/a\\tb\\nc\\\\d\t\n")
    }
}
