import Foundation
import Testing
@testable import HandbyteCore

private let names: Set<String> = ["node_modules", "target", "build"]

@Test func findsArtifactDirectoriesWithoutDescendingIntoMatches() throws {
    let home = try TemporaryHome()
    defer { home.cleanUp() }
    try home.writeFile("dev/app/node_modules/pkg/index.js", size: 10)
    try home.writeFile("dev/app/node_modules/pkg/node_modules/nested/index.js", size: 10)
    try home.writeFile("dev/rusty/target/debug/bin", size: 10)
    try home.writeFile("dev/rusty/src/main.rs", size: 10)
    try home.writeFile("dev/other/build", size: 10)

    let result = ProjectArtifactFinder.find(under: home.path("dev"), directoryNames: names)

    #expect(Set(result.matches.map(\.path)) == [
        home.path("dev/app/node_modules").path,
        home.path("dev/rusty/target").path,
    ])
    #expect(result.errorCount == 0)
    #expect(result.cancelled == false)
}

@Test func skipsVersionControlMetadataAndSymlinks() throws {
    let home = try TemporaryHome()
    defer { home.cleanUp() }
    try home.writeFile("dev/app/.git/node_modules/x", size: 10)
    try home.writeFile("dev/app/.svn/target/x", size: 10)
    try home.writeFile("dev/app/.hg/build/x", size: 10)
    try home.writeFile("elsewhere/node_modules/x", size: 10)
    try home.symlink("dev/app/link", to: "elsewhere")
    try home.symlink("dev/app/node_modules", to: "elsewhere/node_modules")

    let result = ProjectArtifactFinder.find(under: home.path("dev"), directoryNames: names)

    #expect(result.matches.isEmpty)
}

@Test func unreadableDirectoryCountsAsErrorAndKeepsOtherMatches() throws {
    let home = try TemporaryHome()
    defer { home.cleanUp() }
    try home.writeFile("dev/ok/build/out", size: 10)
    try home.writeFile("dev/locked/build/out", size: 10)
    try home.makeUnreadable("dev/locked")

    let result = ProjectArtifactFinder.find(under: home.path("dev"), directoryNames: names)

    #expect(result.matches.map(\.path) == [home.path("dev/ok/build").path])
    #expect(result.errorCount == 1)
}

@Test func missingRootYieldsNoMatchesAndNoError() throws {
    let home = try TemporaryHome()
    defer { home.cleanUp() }

    let result = ProjectArtifactFinder.find(under: home.path("missing"), directoryNames: names)

    #expect(result.matches.isEmpty)
    #expect(result.errorCount == 0)
}

@Test func cancellationStopsDiscovery() throws {
    let home = try TemporaryHome()
    defer { home.cleanUp() }
    try home.writeFile("dev/app/build/out", size: 10)

    let result = ProjectArtifactFinder.find(under: home.path("dev"), directoryNames: names, isCancelled: { true })

    #expect(result.cancelled == true)
    #expect(result.matches.isEmpty)
}
