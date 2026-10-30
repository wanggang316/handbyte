import Foundation
import Testing
import HandbyteCore
@testable import HandbyteUI

@Suite("Settings feed the scan and the policy")
struct SettingsPolicyTests {
    let home = URL(filePath: "/Users/alice", directoryHint: .isDirectory)

    @Test func scanConfigurationTakesRootsAndMinimumSize() {
        var settings = Settings()
        settings.projectRoots = ["~/src"]
        settings.minimumSize = 1024

        let configuration = settings.scanConfiguration(home: home)

        #expect(configuration.home == home)
        #expect(configuration.projectRoots == ["~/src"])
        #expect(configuration.minimumSize == 1024)
        #expect(configuration.rules == Catalog.rules)
    }

    @Test func policyContextExpandsRootsAndCarriesExclusions() {
        var settings = Settings()
        settings.projectRoots = ["~/src", "/Volumes/Work"]
        settings.exclusions = ["xcode-archives", "/Users/alice/Library/Caches/keep"]

        let context = settings.policyContext(home: home, runningApps: StaticRunningAppsProbe())

        #expect(context.home == home)
        #expect(context.projectRoots.map { $0.path(percentEncoded: false) } == ["/Users/alice/src/", "/Volumes/Work/"])
        #expect(context.exclusions == ["xcode-archives", "/Users/alice/Library/Caches/keep"])
    }

    @Test func excludingACandidateRecordsItsPathOnce() {
        let candidate = Fixtures.candidate(path: "/Users/alice/Library/Caches/com.example.app/")
        var settings = Settings()

        settings.exclude(candidate)
        settings.exclude(candidate)

        #expect(settings.exclusions == ["/Users/alice/Library/Caches/com.example.app"])
        #expect(settings.excludes(candidate))
    }

    @Test func addingAnEntryTrimsAndIgnoresBlanksAndDuplicates() {
        var settings = Settings()
        settings.addExclusion("  xcode-archives ")
        settings.addExclusion("xcode-archives")
        settings.addExclusion("   ")
        settings.addExclusion("/Users/alice/Library/Caches/keep/")
        #expect(settings.exclusions == ["xcode-archives", "/Users/alice/Library/Caches/keep"])

        settings.removeExclusion("xcode-archives")
        #expect(settings.exclusions == ["/Users/alice/Library/Caches/keep"])
    }

    @Test func excludedPathStaysBlockedWhenTheDirectoryIsRecreated() throws {
        let fixtureHome = FileManager.default.temporaryDirectory
            .appending(path: "hb-policy-\(UUID().uuidString)", directoryHint: .isDirectory)
        let directory = fixtureHome.appending(path: "Library/Caches/com.example.app", directoryHint: .isDirectory)
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(at: fixtureHome) }

        let rule = Fixtures.rule()
        let first = try #require(Fixtures.candidate(onDisk: directory, rule: rule))
        var settings = Settings()
        settings.exclude(first)

        // A rescan after the directory was replaced: new inode, same path.
        try FileManager.default.removeItem(at: directory)
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        let second = try #require(Fixtures.candidate(onDisk: directory, rule: rule))
        #expect(second.identity != first.identity)

        let policy = EligibilityPolicy(context: settings.policyContext(home: fixtureHome, runningApps: StaticRunningAppsProbe()))
        #expect(policy.evaluate(second) == .blocked(reason: "Excluded by user"))
        #expect(policy.evaluate(first) == .blocked(reason: "Excluded by user"))
    }

    @Test func ruleIDExclusionBlocksEveryCandidateOfTheRule() {
        var settings = Settings()
        settings.addExclusion("xcode-archives")
        let archives = Fixtures.candidate(
            path: "/Users/alice/Library/Developer/Xcode/Archives", rule: Fixtures.rule(id: "xcode-archives", category: .xcode)
        )
        let policy = EligibilityPolicy(context: settings.policyContext(home: home, runningApps: StaticRunningAppsProbe()))
        // Missing on disk; the exclusion check runs before any probe.
        #expect(policy.evaluate(archives) == .blocked(reason: "Excluded by user"))
    }
}

@Suite("Clean confirmation")
struct CleanConfirmationTests {
    @Test func trashNeedsNoAcknowledgement() {
        #expect(!Settings.DeletionMode.trash.requiresAcknowledgement)
        #expect(CleanConfirmation.action(for: .trash, acknowledged: false) == .trash)
        #expect(CleanConfirmation.action(for: .trash, acknowledged: true) == .trash)
    }

    @Test func permanentDeletionIsUnreachableWithoutAcknowledgement() {
        #expect(Settings.DeletionMode.removePermanently.requiresAcknowledgement)
        #expect(CleanConfirmation.action(for: .removePermanently, acknowledged: false) == nil)
        #expect(CleanConfirmation.action(for: .removePermanently, acknowledged: true) == .removePermanently)
    }
}
