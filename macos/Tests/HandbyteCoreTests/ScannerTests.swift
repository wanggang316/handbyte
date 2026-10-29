import Foundation
import os
import Testing
@testable import HandbyteCore

private func rules(_ ids: String...) -> [Rule] {
    ids.map { id in Catalog.rules.first { $0.id == id }! }
}

private func configuration(
    _ home: TemporaryHome,
    rules: [Rule] = Catalog.rules,
    minimumSize: Int64 = 0,
    roots: [String] = [],
    skipProjectScan: Bool = true,
    workers: Int = 6
) -> ScanConfiguration {
    var configuration = ScanConfiguration(home: home.url)
    configuration.rules = rules
    configuration.minimumSize = minimumSize
    configuration.projectRoots = roots
    configuration.skipProjectScan = skipProjectScan
    configuration.workers = workers
    return configuration
}

@Test func defaultsMatchTheGoCLI() {
    let configuration = ScanConfiguration(home: URL(filePath: "/Users/alice", directoryHint: .isDirectory))
    #expect(configuration.minimumSize == 50 * 1024 * 1024)
    #expect(configuration.workers == 6)
    #expect(configuration.skipProjectScan == false)
    #expect(configuration.projectRoots == Catalog.defaultProjectRoots)
    #expect(configuration.rules == Catalog.rules)
}

@Test func presetsAreMeasuredFilteredBySizeAndSortedDescending() async throws {
    let home = try TemporaryHome()
    defer { home.cleanUp() }
    for index in 0..<3 {
        try home.writeFile("Library/Developer/Xcode/DerivedData/Mod-\(index)/a.o", size: 100_000)
    }
    try home.writeFile("Library/Caches/Homebrew/x.tar.gz", size: 50_000)
    try home.writeFile(".npm/_cacache/index", size: 100)

    let result = await DiskScanner().scan(configuration(
        home, rules: rules("npm-cache", "brew-cache", "xcode-derived"), minimumSize: 8192
    ))

    #expect(result.candidates.map(\.rule.id) == ["xcode-derived", "brew-cache"])
    let derived = try #require(result.candidates.first)
    #expect(derived.url.path == home.path("Library/Developer/Xcode/DerivedData").path)
    #expect(derived.size.fileCount == 3)
    #expect(derived.size.allocatedBytes == (try home.allocatedBytes("Library/Developer/Xcode/DerivedData/Mod-0/a.o")) * 3)
    #expect(derived.size.partial == false)
    #expect(derived.eligibility == .eligible)
    #expect(derived.identity == FileStatus(lstatPath: derived.url.path)?.identity)
    #expect(derived.lastActivity.map { Date().timeIntervalSince($0) < 60 } == true)
    #expect(result.filesSeen == 5)
    #expect(result.cancelled == false)
}

@Test func sameDirectoryReachedByTwoRulesYieldsTheSpecificRule() async throws {
    let home = try TemporaryHome()
    defer { home.cleanUp() }
    try home.writeFile("Library/Caches/go-build/00/obj", size: 10)
    try home.writeFile("Library/Caches/Google/Chrome/blob", size: 10)

    let result = await DiskScanner().scan(configuration(home))

    #expect(result.candidates.count == 2)
    let byPath = Dictionary(uniqueKeysWithValues: result.candidates.map { ($0.url.lastPathComponent, $0.rule.id) })
    #expect(byPath == ["go-build": "go-build", "Google": "library-caches"])
}
