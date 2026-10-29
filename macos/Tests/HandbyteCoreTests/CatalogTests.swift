import Foundation
import Testing
@testable import HandbyteCore

/// Rule identifiers printed by the Go CLI as `Source`, taken from
/// `internal/scanner/presets.go`. `scripts/sync-check.sh` guards the live file;
/// this literal guards the Swift side against accidental edits.
private let goRuleIDs: Set<String> = [
    "xcode-derived", "xcode-devicesupport", "xcode-archives", "coresim-cache",
    "go-build", "go-mod-cache", "cargo-registry", "cargo-git", "rustup",
    "npm-cache", "pnpm-store", "yarn-cache", "gradle-cache", "maven-cache",
    "swiftpm-cache", "playwright", "cypress", "brew-cache", "jetbrains", "codex",
    "xdg-cache", "library-caches", "library-logs", "containers",
    "orbstack", "apple-container", "claude-vm", "docker",
    "trash", "downloads",
    "artifact:node_modules", "artifact:target", "artifact:build", "artifact:.build",
    "artifact:dist", "artifact:.next", "artifact:.nuxt", "artifact:.turbo",
    "artifact:.parcel-cache", "artifact:.gradle", "artifact:.dart_tool",
    "artifact:DerivedData", "artifact:Pods", "artifact:__pycache__",
    "artifact:.pytest_cache", "artifact:.mypy_cache", "artifact:.ruff_cache",
    "artifact:.venv", "artifact:venv",
]

@Test func catalogIDsMatchGoCLI() {
    #expect(Set(Catalog.rules.map(\.id)) == goRuleIDs)
}

@Test func catalogRowCountsMatchGo() {
    #expect(Catalog.presets.count == 33)
    #expect(Catalog.projectArtifacts.count == 19)
}

@Test func catalogHasNoDuplicateRows() {
    let presetRows = Catalog.presets.map { "\($0.id)|\($0.pathTemplate ?? "")" }
    #expect(Set(presetRows).count == presetRows.count)

    let artifactNames = Catalog.projectArtifacts.compactMap(\.directoryName)
    #expect(Set(artifactNames).count == artifactNames.count)
    #expect(artifactNames.count == Catalog.projectArtifacts.count)
}

@Test func presetExpandsTildeAgainstInjectedHome() throws {
    let home = URL(filePath: "/Volumes/Test/alice", directoryHint: .isDirectory)

    let derived = try #require(Catalog.presets.first { $0.id == "xcode-derived" })
    #expect(derived.location(in: home)?.path == "/Volumes/Test/alice/Library/Developer/Xcode/DerivedData")
    #expect(derived.expandChildren == false)

    let deviceSupport = try #require(Catalog.presets.first { $0.pathTemplate == "~/Library/Developer/Xcode/iOS DeviceSupport" })
    #expect(deviceSupport.location(in: home)?.path == "/Volumes/Test/alice/Library/Developer/Xcode/iOS DeviceSupport")

    let caches = try #require(Catalog.presets.first { $0.id == "library-caches" })
    #expect(caches.expandChildren == true)
}

@Test func everyPresetTemplateStartsWithTilde() {
    for rule in Catalog.presets {
        #expect(rule.pathTemplate?.hasPrefix("~/") == true, "\(rule.id)")
        #expect(rule.directoryName == nil, "\(rule.id)")
    }
}

@Test func artifactRulesUseArtifactPrefixAndNoPath() {
    let home = URL(filePath: "/Volumes/Test/alice", directoryHint: .isDirectory)
    for rule in Catalog.projectArtifacts {
        #expect(rule.id == "artifact:\(rule.directoryName ?? "")", "\(rule.id)")
        #expect(rule.category == .projectBuild, "\(rule.id)")
        #expect(rule.pathTemplate == nil, "\(rule.id)")
        #expect(rule.expandChildren == false, "\(rule.id)")
        #expect(rule.location(in: home) == nil, "\(rule.id)")
    }
}

@Test func defaultProjectRootsExpandAgainstInjectedHome() {
    let home = URL(filePath: "/Volumes/Test/alice", directoryHint: .isDirectory)
    #expect(Catalog.defaultProjectRoots == ["~/dev", "~/Projects", "~/Code", "~/workspace"])
    #expect(Catalog.defaultProjectRoots.map { PathTemplate.expand($0, home: home).path } == [
        "/Volumes/Test/alice/dev",
        "/Volumes/Test/alice/Projects",
        "/Volumes/Test/alice/Code",
        "/Volumes/Test/alice/workspace",
    ])
}

@Test func pathTemplateExpansionKeepsAbsolutePathsUntouched() {
    let home = URL(filePath: "/Volumes/Test/alice", directoryHint: .isDirectory)
    #expect(PathTemplate.expand("/opt/data", home: home).path == "/opt/data")
    #expect(PathTemplate.expand("~", home: home).path == "/Volumes/Test/alice")
}

@Test func selectedRulesCarryGoCategoryAndSafety() throws {
    let archives = try #require(Catalog.presets.first { $0.id == "xcode-archives" })
    #expect(archives.category == .xcode)
    #expect(archives.safety == .caution)

    let rustup = try #require(Catalog.presets.first { $0.id == "rustup" })
    #expect(rustup.safety == .review)
    #expect(rustup.expandChildren == true)

    let pycache = try #require(Catalog.projectArtifacts.first { $0.id == "artifact:__pycache__" })
    #expect(pycache.safety == .auto)

    let orbstack = try #require(Catalog.presets.first { $0.id == "orbstack" })
    #expect(orbstack.ownerBundleIDs == ["dev.kdrag0n.MacVirt"])
}

@Test func everyRuleHasSummaryAndRecovery() {
    for rule in Catalog.rules {
        #expect(!rule.summary.isEmpty, "\(rule.id)")
        #expect(!rule.recovery.isEmpty, "\(rule.id)")
    }
}
