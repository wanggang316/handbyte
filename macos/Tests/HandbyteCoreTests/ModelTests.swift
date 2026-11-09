import Foundation
import Testing
@testable import HandbyteCore

private let sampleRule = Rule(
    id: "xcode-derived",
    kind: .preset(pathTemplate: "~/Library/Developer/Xcode/DerivedData", expandChildren: false),
    category: .xcode,
    safety: .auto,
    ownerBundleIDs: ["com.apple.dt.Xcode"],
    summary: "Xcode build intermediates.",
    recovery: "Regenerated on next build."
)

@Test func categoryAndSafetyUseGoWireValues() {
    #expect(Category.systemCache.rawValue == "System & App Caches")
    #expect(Category.devCache.rawValue == "Developer Caches")
    #expect(Category.xcode.rawValue == "Xcode & iOS")
    #expect(Category.projectBuild.rawValue == "Project Build Artifacts")
    #expect(Category.vms.rawValue == "VMs & Containers")
    #expect(Category.trash.rawValue == "Trash & Logs")
    #expect(Category.other.rawValue == "Other Large Directories")
    #expect(Safety.auto.rawValue == 0)
    #expect(Safety.review.rawValue == 1)
    #expect(Safety.caution.rawValue == 2)
    #expect(Safety.auto < Safety.review && Safety.review < Safety.caution)
}

@Test func candidateIDDerivesFromRuleAndFileIdentity() {
    let identity = FileIdentity(dev: 16777234, ino: 42)
    let candidate = Candidate(
        rule: sampleRule,
        url: URL(filePath: "/Users/alice/Library/Developer/Xcode/DerivedData", directoryHint: .isDirectory),
        identity: identity,
        size: SizeReport(allocatedBytes: 4096, fileCount: 1, partial: false, errorCount: 0),
        lastActivity: nil,
        eligibility: .eligible
    )
    #expect(candidate.id == "xcode-derived@16777234:42")

    let samePathOtherRule = Candidate(
        rule: Rule(id: "library-caches", kind: .preset(pathTemplate: "~/Library/Caches", expandChildren: true),
                   category: .systemCache, safety: .auto, ownerBundleIDs: [], summary: "s", recovery: "r"),
        url: candidate.url, identity: identity, size: candidate.size, lastActivity: nil, eligibility: .eligible
    )
    #expect(samePathOtherRule.id != candidate.id)
}

@Test func candidateRoundTripsThroughJSON() throws {
    let candidate = Candidate(
        rule: sampleRule,
        url: URL(filePath: "/Users/alice/Library/Developer/Xcode/DerivedData", directoryHint: .isDirectory),
        identity: FileIdentity(dev: 1, ino: 2),
        size: SizeReport(allocatedBytes: 1_048_576, fileCount: 12, partial: true, errorCount: 1),
        lastActivity: Date(timeIntervalSince1970: 1_700_000_000),
        eligibility: .reviewOnly(reason: "Safety level requires review")
    )
    let data = try JSONEncoder().encode(candidate)
    let decoded = try JSONDecoder().decode(Candidate.self, from: data)
    #expect(decoded == candidate)
}

@Test func cleanOutcomeDistinguishesStatuses() {
    let candidate = Candidate(
        rule: sampleRule,
        url: URL(filePath: "/Users/alice/x", directoryHint: .isDirectory),
        identity: FileIdentity(dev: 1, ino: 2),
        size: SizeReport(allocatedBytes: 10, fileCount: 1, partial: false, errorCount: 0),
        lastActivity: nil,
        eligibility: .blocked(reason: "Symlink")
    )
    let preview = CleanOutcome(candidate: candidate, status: .previewed, bytesClaimed: 0)
    let moved = CleanOutcome(candidate: candidate, status: .moved(URL(filePath: "/Users/alice/.Trash/x")), bytesClaimed: 10)
    let skipped = CleanOutcome(candidate: candidate, status: .skipped(reason: "Symlink"), bytesClaimed: 0)
    #expect(preview != moved)
    #expect(skipped.status == .skipped(reason: "Symlink"))
    #expect(CleanAction.trash != CleanAction.removePermanently)
}

@Test func candidateEncodesItsLocationAsAPlainPath() throws {
    let candidate = Candidate(
        rule: sampleRule,
        url: URL(filePath: "/Users/alice/Library/Application Support/x", directoryHint: .isDirectory),
        identity: FileIdentity(dev: 1, ino: 2),
        size: SizeReport(allocatedBytes: 10, fileCount: 1, partial: false, errorCount: 0),
        lastActivity: nil,
        eligibility: .eligible
    )
    let encoder = JSONEncoder()
    encoder.outputFormatting = [.sortedKeys, .withoutEscapingSlashes]
    let json = try #require(String(data: try encoder.encode(candidate), encoding: .utf8))
    #expect(json.contains(#""path":"/Users/alice/Library/Application Support/x""#))
    #expect(!json.contains("file:"))
    #expect(!json.contains(#""url""#))
}
