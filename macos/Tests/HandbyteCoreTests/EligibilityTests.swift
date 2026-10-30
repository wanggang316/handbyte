import Foundation
import Testing
@testable import HandbyteCore

@Suite("EligibilityPolicy")
struct EligibilityPolicyTests {
    @Test func regenerableCandidateIsEligible() throws {
        let fixture = try CleaningFixture()
        let url = try fixture.makeDirectory("Library/Caches/com.example.app")
        let candidate = try fixture.candidate(at: url)

        #expect(fixture.policy().evaluate(candidate) == .eligible)
    }

    @Test func exclusionByIDOrPathBlocksFirst() throws {
        let fixture = try CleaningFixture()
        let url = try fixture.makeDirectory("Library/Caches/com.example.app")
        let candidate = try fixture.candidate(at: url)
        // Missing on disk as well: the exclusion reason must still win.
        try FileManager.default.removeItem(at: url)

        #expect(fixture.policy(exclusions: [candidate.id]).evaluate(candidate) == .blocked(reason: "Excluded by user"))
        #expect(fixture.policy(exclusions: [url.path + "/"]).evaluate(candidate) == .blocked(reason: "Excluded by user"))
        #expect(fixture.policy(exclusions: ["other-id"]).evaluate(candidate) == .blocked(reason: "Missing"))
    }

    @Test func protectedRootsAreBlockedWithoutProbing() throws {
        let fixture = try CleaningFixture()
        let projectRoot = fixture.home.appending(path: "dev", directoryHint: .isDirectory)
        let policy = fixture.policy(projectRoots: [projectRoot])
        let protected = [
            URL(filePath: "/", directoryHint: .isDirectory),
            fixture.home,
            fixture.home.appending(path: "Library", directoryHint: .isDirectory),
            fixture.home.appending(path: "Library/Containers", directoryHint: .isDirectory),
            fixture.home.appending(path: "Library/../Library/Containers/", directoryHint: .isDirectory),
            projectRoot,
            fixture.home.appending(path: "dev", directoryHint: .notDirectory),
        ]
        for url in protected {
            #expect(policy.evaluate(fixture.candidate(atUnprobed: url)) == .blocked(reason: "Protected location"), "\(url.path)")
        }

        let child = try fixture.makeDirectory("Library/Containers/com.example.app")
        #expect(policy.evaluate(try fixture.candidate(at: child)) == .eligible)
    }

    @Test func symlinkIsBlocked() throws {
        let fixture = try CleaningFixture()
        let target = try fixture.makeDirectory("real")
        let link = try fixture.makeSymlink("Library/Caches/link", to: target)

        #expect(fixture.policy().evaluate(try fixture.candidate(at: link)) == .blocked(reason: "Symbolic link"))
    }

    @Test func symlinkSwapAfterScanIsBlocked() throws {
        let fixture = try CleaningFixture()
        let url = try fixture.makeDirectory("Library/Caches/com.example.app")
        let candidate = try fixture.candidate(at: url)
        let elsewhere = try fixture.makeDirectory("elsewhere")
        try FileManager.default.removeItem(at: url)
        try FileManager.default.createSymbolicLink(at: url, withDestinationURL: elsewhere)

        #expect(fixture.policy().evaluate(candidate) == .blocked(reason: "Symbolic link"))
    }

    @Test func identityChangeSinceScanIsBlocked() throws {
        let fixture = try CleaningFixture()
        let url = try fixture.makeDirectory("Library/Caches/com.example.app")
        let candidate = try fixture.candidate(at: url)
        try FileManager.default.removeItem(at: url)
        try fixture.makeDirectory("Library/Caches/com.example.app")

        #expect(fixture.policy().evaluate(candidate) == .blocked(reason: "Changed since scan"))
    }

    @Test func missingTargetIsBlocked() throws {
        let fixture = try CleaningFixture()
        let url = try fixture.makeDirectory("Library/Caches/com.example.app")
        let candidate = try fixture.candidate(at: url)
        try FileManager.default.removeItem(at: url)

        #expect(fixture.policy().evaluate(candidate) == .blocked(reason: "Missing"))
    }

    @Test func runningOwnerIsBlocked() throws {
        let fixture = try CleaningFixture()
        let url = try fixture.makeDirectory("Library/Developer/Xcode/DerivedData")
        let rule = fixture.rule(id: "xcode-derived", owners: ["com.apple.dt.Xcode"])
        let candidate = try fixture.candidate(at: url, rule: rule)

        #expect(fixture.policy(running: ["com.apple.dt.Xcode"]).evaluate(candidate) == .blocked(reason: "com.apple.dt.Xcode is running"))
        #expect(fixture.policy(running: ["com.apple.Safari"]).evaluate(candidate) == .eligible)
    }

    @Test func safetyMapsToSelectionTier() throws {
        let fixture = try CleaningFixture()
        let url = try fixture.makeDirectory("Downloads")
        let policy = fixture.policy()

        let caution = try fixture.candidate(at: url, rule: fixture.rule(id: "downloads", safety: .caution))
        #expect(policy.evaluate(caution) == .reviewOnly(reason: "Needs deliberate confirmation"))

        let review = try fixture.candidate(at: url, rule: fixture.rule(id: "trash", safety: .review))
        #expect(policy.evaluate(review) == .eligible)
    }

    @Test func applyRewritesEligibilityAndKeepsOrder() throws {
        let fixture = try CleaningFixture()
        let keep = try fixture.candidate(at: try fixture.makeDirectory("Library/Caches/a"))
        let gone = try fixture.candidate(at: try fixture.makeDirectory("Library/Caches/b"))
        try FileManager.default.removeItem(at: gone.url)

        let evaluated = fixture.policy().apply(to: [gone, keep])

        #expect(evaluated.map(\.id) == [gone.id, keep.id])
        #expect(evaluated[0].eligibility == .blocked(reason: "Missing"))
        #expect(evaluated[1].eligibility == .eligible)
        #expect(evaluated[1].size == keep.size)
    }
}
