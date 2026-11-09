import Foundation

import Testing
@testable import HandbyteCore

private let realTrash = FileManager.default.homeDirectoryForCurrentUser
    .appending(path: ".Trash", directoryHint: .isDirectory).standardizedFileURL

private func isInsideTrash(_ url: URL) -> Bool {
    url.standardizedFileURL.path(percentEncoded: false).hasPrefix(realTrash.path(percentEncoded: false))
}

private func journalLines(_ journal: OperationsJournal) -> [String] {
    guard let text = try? String(contentsOf: journal.fileURL, encoding: .utf8) else { return [] }
    return text.split(separator: "\n").map(String.init)
}

@Suite("Cleaner", .serialized)
struct CleanerTests {
    @Test func trashMovesEachItemIntoTheTrashAndClaimsScannedBytes() async throws {
        let fixture = try CleaningFixture()
        let a = try fixture.candidate(at: try fixture.makeDirectory("Library/Caches/hb-a"), allocatedBytes: 1000)
        let b = try fixture.candidate(at: try fixture.makeDirectory("Library/Caches/hb-b"), allocatedBytes: 24)
        let journal = OperationsJournal(fileURL: fixture.root.appending(path: "ops.log"))

        let outcomes = await Cleaner().clean([a, b], action: .trash, policy: fixture.policy(), journal: journal)
        for case .moved(let url) in outcomes.map(\.status) { fixture.trackTrashed(url) }

        #expect(outcomes.count == 2)
        for (outcome, candidate) in zip(outcomes, [a, b]) {
            #expect(outcome.candidate == candidate)
            #expect(outcome.bytesClaimed == candidate.size.allocatedBytes)
            guard case .moved(let destination) = outcome.status else {
                Issue.record("expected moved, got \(outcome.status)")
                continue
            }
            #expect(isInsideTrash(destination), "\(destination.path)")
            #expect(FileManager.default.fileExists(atPath: destination.appending(path: "payload.bin").path))
            #expect(!FileManager.default.fileExists(atPath: candidate.url.path))
        }
        let lines = journalLines(journal)
        #expect(lines.count == 2)
        #expect(lines.allSatisfy { $0.contains("\ttrash\tmoved\tlibrary-caches\t") })
        // Directory URLs carry a trailing slash; the journal writes paths without it.
        let fields = lines.first?.split(separator: "\t", omittingEmptySubsequences: false).map(String.init) ?? []
        #expect(fields.count == 7)
        #expect(fields[4] == "1000")
        #expect(fields[5] == a.url.fileSystemPath)
        #expect(fields[6].hasPrefix(realTrash.fileSystemPath), "\(fields[6])")
        #expect(!fields[6].hasSuffix("/"))
    }

    @Test func previewChangesNothingAndClaimsZero() async throws {
        let fixture = try CleaningFixture()
        let candidate = try fixture.candidate(at: try fixture.makeDirectory("Library/Caches/hb-preview"), allocatedBytes: 1000)
        let journal = OperationsJournal(fileURL: fixture.root.appending(path: "ops.log"))

        let outcomes = await Cleaner().clean([candidate], action: .preview, policy: fixture.policy(), journal: journal)

        #expect(outcomes.map(\.status) == [.previewed])
        #expect(outcomes.map(\.bytesClaimed) == [0])
        #expect(FileManager.default.fileExists(atPath: candidate.url.path))
        #expect(journalLines(journal).isEmpty)
    }

    @Test func permanentRemovalOnlyWhenRequested() async throws {
        let fixture = try CleaningFixture()
        let candidate = try fixture.candidate(at: try fixture.makeDirectory("Library/Caches/hb-remove"), allocatedBytes: 512)
        let journal = OperationsJournal(fileURL: fixture.root.appending(path: "ops.log"))

        let outcomes = await Cleaner().clean([candidate], action: .removePermanently, policy: fixture.policy(), journal: journal)

        #expect(outcomes.map(\.status) == [.removed])
        #expect(outcomes.map(\.bytesClaimed) == [512])
        #expect(!FileManager.default.fileExists(atPath: candidate.url.path))
        let lines = journalLines(journal)
        #expect(lines.count == 1)
        #expect(lines.first?.contains("\tremovePermanently\tremoved\tlibrary-caches\t512\t") == true)
    }

    @Test func blockedCandidatesAreSkippedWithThePolicyReason() async throws {
        let fixture = try CleaningFixture()
        let missing = try fixture.candidate(at: try fixture.makeDirectory("Library/Caches/hb-missing"))
        try FileManager.default.removeItem(at: missing.url)
        let excluded = try fixture.candidate(at: try fixture.makeDirectory("Library/Caches/hb-excluded"))
        let owned = try fixture.candidate(
            at: try fixture.makeDirectory("Library/Developer/Xcode/DerivedData"),
            rule: fixture.rule(id: "xcode-derived", owners: ["com.apple.dt.Xcode"]))
        let journal = OperationsJournal(fileURL: fixture.root.appending(path: "ops.log"))
        let policy = fixture.policy(exclusions: [excluded.id], running: ["com.apple.dt.Xcode"])

        let outcomes = await Cleaner().clean([missing, excluded, owned], action: .trash, policy: policy, journal: journal)

        #expect(outcomes.map(\.status) == [
            .skipped(reason: "Missing"),
            .skipped(reason: "Excluded by user"),
            .skipped(reason: "com.apple.dt.Xcode is running"),
        ])
        #expect(outcomes.map(\.bytesClaimed) == [0, 0, 0])
        #expect(FileManager.default.fileExists(atPath: excluded.url.path))
        #expect(FileManager.default.fileExists(atPath: owned.url.path))
        #expect(journalLines(journal).isEmpty)
    }

    @Test func reviewOnlyCandidateIsCleanedWhenSelectedExplicitly() async throws {
        let fixture = try CleaningFixture()
        let candidate = try fixture.candidate(
            at: try fixture.makeDirectory("Downloads"),
            rule: fixture.rule(id: "downloads", safety: .caution))

        let outcomes = await Cleaner().clean([candidate], action: .removePermanently, policy: fixture.policy(), journal: nil)

        #expect(outcomes.map(\.status) == [.removed])
        #expect(!FileManager.default.fileExists(atPath: candidate.url.path))
    }

    @Test func cancellationLeavesLaterItemsNotAttempted() async throws {
        let fixture = try CleaningFixture()
        var candidates: [Candidate] = []
        for name in ["one", "two", "three", "four"] {
            candidates.append(try fixture.candidate(at: try fixture.makeDirectory("Library/Caches/hb-\(name)")))
        }
        let policy = fixture.policy()
        let journal = OperationsJournal(fileURL: fixture.root.appending(path: "ops.log"))
        let progressLog = LockedBox<[CleanProgress]>([])

        let task = Task { [candidates] in
            await Cleaner().clean(candidates, action: .removePermanently, policy: policy, journal: journal) { progress in
                progressLog.withLock { $0.append(progress) }
                if progress.completed == 2 {
                    withUnsafeCurrentTask { $0?.cancel() }
                }
            }
        }
        let outcomes = await task.value

        #expect(outcomes.map(\.status) == [.removed, .removed, .notAttempted, .notAttempted])
        #expect(outcomes.map(\.bytesClaimed) == [4096, 4096, 0, 0])
        #expect(!FileManager.default.fileExists(atPath: candidates[1].url.path))
        #expect(FileManager.default.fileExists(atPath: candidates[2].url.path))
        #expect(FileManager.default.fileExists(atPath: candidates[3].url.path))
        #expect(journalLines(journal).count == 2)

        let reports = progressLog.withLock { $0 }
        #expect(reports.map(\.completed) == [1, 2, 3, 4])
        #expect(reports.map(\.total) == [4, 4, 4, 4])
        #expect(reports.map(\.bytesClaimed) == [4096, 8192, 8192, 8192])
        #expect(reports.map(\.latest.candidate.id) == candidates.map(\.id))
    }

    @Test func failedActionIsReportedAndJournaledWithoutClaimingBytes() async throws {
        let fixture = try CleaningFixture()
        let locked = try fixture.makeDirectory("Library/Caches/locked", fileBytes: 0)
        let candidate = try fixture.candidate(at: try fixture.makeDirectory("Library/Caches/locked/inner"), allocatedBytes: 77)
        try FileManager.default.setAttributes([.posixPermissions: 0o555], ofItemAtPath: locked.path)
        defer { try? FileManager.default.setAttributes([.posixPermissions: 0o755], ofItemAtPath: locked.path) }
        let journal = OperationsJournal(fileURL: fixture.root.appending(path: "ops.log"))

        let outcomes = await Cleaner().clean([candidate], action: .removePermanently, policy: fixture.policy(), journal: journal)

        let outcome = try #require(outcomes.first)
        guard case .failed(let message) = outcome.status else {
            Issue.record("expected failed, got \(outcome.status)")
            return
        }
        #expect(!message.isEmpty)
        #expect(outcome.bytesClaimed == 0)
        #expect(FileManager.default.fileExists(atPath: candidate.url.path))
        let lines = journalLines(journal)
        #expect(lines.count == 1)
        #expect(lines.first?.contains("\tremovePermanently\tfailed\tlibrary-caches\t0\t") == true)
    }
}
