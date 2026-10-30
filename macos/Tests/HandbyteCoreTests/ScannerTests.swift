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
        try home.writeFile("Library/Caches/go-build/Mod-\(index)/a.o", size: 100_000)
    }
    try home.writeFile("Library/Caches/Homebrew/x.tar.gz", size: 50_000)
    try home.writeFile(".npm/_cacache/index", size: 100)

    let result = await DiskScanner().collect(configuration(
        home, rules: rules("npm-cache", "brew-cache", "go-build"), minimumSize: 8192
    ))

    #expect(result.candidates.map(\.rule.id) == ["go-build", "brew-cache"])
    let goBuild = try #require(result.candidates.first)
    #expect(goBuild.url.path == home.path("Library/Caches/go-build").path)
    #expect(goBuild.size.fileCount == 3)
    #expect(goBuild.size.allocatedBytes == (try home.allocatedBytes("Library/Caches/go-build/Mod-0/a.o")) * 3)
    #expect(goBuild.size.partial == false)
    #expect(goBuild.eligibility == .eligible)
    #expect(goBuild.identity == FileStatus(lstatPath: goBuild.url.path)?.identity)
    #expect(goBuild.lastActivity.map { Date().timeIntervalSince($0) < 60 } == true)
    #expect(result.filesSeen == 5)
    #expect(result.cancelled == false)
}

@Test func sameDirectoryReachedByTwoRulesYieldsTheSpecificRule() async throws {
    let home = try TemporaryHome()
    defer { home.cleanUp() }
    try home.writeFile("Library/Caches/go-build/00/obj", size: 10)
    try home.writeFile("Library/Caches/Google/Chrome/blob", size: 10)

    let result = await DiskScanner().collect(configuration(home))

    #expect(result.candidates.count == 2)
    let byPath = Dictionary(uniqueKeysWithValues: result.candidates.map { ($0.url.lastPathComponent, $0.rule.id) })
    #expect(byPath == ["go-build": "go-build", "Google": "library-caches"])
}

@Test func expandChildrenYieldsOneCandidatePerChildDirectory() async throws {
    let home = try TemporaryHome()
    defer { home.cleanUp() }
    try home.writeFile(".cache/uv/wheel", size: 10)
    try home.writeFile(".cache/pip/http/x", size: 10)
    try home.writeFile(".cache/stray-file", size: 10)
    try home.makeDirectory("elsewhere")
    try home.symlink(".cache/linked", to: "elsewhere")

    let result = await DiskScanner().collect(configuration(home, rules: rules("xdg-cache")))

    #expect(result.candidates.map(\.url.lastPathComponent).sorted() == ["pip", "uv"])
    #expect(result.candidates.allSatisfy { $0.rule.id == "xdg-cache" })
    #expect(result.categories == [.devCache: .complete])
}

@Test func categoryStatesReportCompletePartialAndUnavailable() async throws {
    let home = try TemporaryHome()
    defer { home.cleanUp() }
    try home.writeFile("Library/Developer/CoreSimulator/Caches/ok/a.o", size: 10)
    try home.writeFile("Library/Developer/CoreSimulator/Caches/locked/b.o", size: 10)
    try home.makeUnreadable("Library/Developer/CoreSimulator/Caches/locked")
    try home.writeFile("Library/Caches/Homebrew/x", size: 10)

    let result = await DiskScanner().collect(configuration(home, rules: rules("coresim-cache", "brew-cache", "downloads")))

    #expect(result.categories == [.xcode: .partial, .devCache: .complete, .other: .unavailable])
    let simulatorCaches = try #require(result.candidates.first { $0.rule.id == "coresim-cache" })
    #expect(simulatorCaches.size.partial == true)
    #expect(simulatorCaches.size.errorCount == 1)
    #expect(simulatorCaches.size.fileCount == 1)
}

@Test func unreadableParentOfExpandedChildrenMarksTheCategoryPartial() async throws {
    let home = try TemporaryHome()
    defer { home.cleanUp() }
    try home.writeFile("Library/Logs/app/x.log", size: 10)
    try home.makeUnreadable("Library/Logs")

    let result = await DiskScanner().collect(configuration(home, rules: rules("library-logs")))

    #expect(result.candidates.isEmpty)
    #expect(result.categories == [.trash: .partial])
}

@Test func scanCancelledBeforeStartReportsEveryCategoryCancelled() async throws {
    let home = try TemporaryHome()
    defer { home.cleanUp() }
    try home.writeFile("Library/Caches/Homebrew/x", size: 10)
    try home.writeFile("Downloads/x", size: 10)
    let scanConfiguration = configuration(home, rules: rules("brew-cache", "downloads"))

    let task = Task { await DiskScanner().collect(scanConfiguration) }
    task.cancel()
    let result = await task.value

    #expect(result.candidates.isEmpty)
    #expect(result.cancelled == true)
    #expect(result.categories == [.devCache: .cancelled, .other: .cancelled])
}

/// Cancels the scanning task from inside its own event handler.
private final class CancelOnFirstCandidate: Sendable {
    private let state = OSAllocatedUnfairLock(initialState: (task: Task<ScanResult, Never>?.none, requested: false))

    func attach(_ task: Task<ScanResult, Never>) {
        let cancelNow = state.withLock { state in
            state.task = task
            return state.requested
        }
        if cancelNow { task.cancel() }
    }

    func handle(_ event: ScanEvent) {
        guard case .candidateFound = event else { return }
        let task = state.withLock { state in
            state.requested = true
            return state.task
        }
        task?.cancel()
    }
}

@Test func cancellationStopsLaunchingNewWorkAndReportsCancelledCategories() async throws {
    let home = try TemporaryHome()
    defer { home.cleanUp() }
    try home.writeFile("Library/Caches/Homebrew/x", size: 10)
    try home.writeFile(".Trash/x", size: 10)
    try home.writeFile("Downloads/x", size: 10)
    let scanConfiguration = configuration(home, rules: rules("brew-cache", "trash", "downloads"), workers: 1)
    let canceller = CancelOnFirstCandidate()

    let task = Task { await DiskScanner().collect(scanConfiguration, onEvent: canceller.handle) }
    canceller.attach(task)
    let result = await task.value

    #expect(result.candidates.map(\.rule.id) == ["brew-cache"])
    #expect(result.cancelled == true)
    #expect(result.categories == [.devCache: .complete, .trash: .cancelled, .other: .cancelled])
}

@Test func eventStreamBracketsCandidatesAndEndsWithTheResult() async throws {
    let home = try TemporaryHome()
    defer { home.cleanUp() }
    try home.writeFile("Library/Caches/Homebrew/x", size: 10)
    try home.writeFile("Library/Developer/CoreSimulator/Caches/a.o", size: 10)

    var events: [ScanEvent] = []
    for await event in DiskScanner().scan(configuration(home, rules: rules("brew-cache", "coresim-cache", "downloads"))) {
        events.append(event)
    }

    guard case .finished(let result)? = events.last else {
        Issue.record("last event was \(String(describing: events.last))")
        return
    }
    #expect(result.candidates.count == 2)
    #expect(result.categories == [.devCache: .complete, .xcode: .complete, .other: .unavailable])

    var open: Set<HandbyteCore.Category> = []
    var closed: Set<HandbyteCore.Category> = []
    var progressCount = 0
    for event in events {
        switch event {
        case .categoryStarted(let category):
            #expect(open.insert(category).inserted)
        case .candidateFound(let candidate):
            #expect(open.contains(candidate.rule.category))
            #expect(!closed.contains(candidate.rule.category))
        case .categoryFinished(let category, let state):
            #expect(open.contains(category))
            #expect(closed.insert(category).inserted)
            #expect(state == result.categories[category])
        case .progress(let progress):
            progressCount += 1
            #expect(!progress.currentPath.isEmpty)
        case .finished:
            break
        }
    }
    #expect(closed == Set(result.categories.keys))
    #expect(progressCount >= 2)
}

@Test func projectArtifactsAreDiscoveredUnderConfiguredRoots() async throws {
    let home = try TemporaryHome()
    defer { home.cleanUp() }
    try home.writeFile("dev/app/node_modules/pkg/index.js", size: 10)
    try home.writeFile("dev/app/.git/node_modules/x", size: 10)
    try home.writeFile("src/tool/target/debug/bin", size: 10)
    try home.writeFile("elsewhere/build/out", size: 10)

    let result = await DiskScanner().collect(configuration(home, roots: ["~/dev", home.path("src").path, "~/missing"], skipProjectScan: false))

    #expect(Set(result.candidates.map(\.rule.id)) == ["artifact:node_modules", "artifact:target"])
    #expect(result.candidates.allSatisfy { $0.rule.category == .projectBuild })
    #expect(result.categories[.projectBuild] == .complete)

    let skipped = await DiskScanner().collect(configuration(home, roots: ["~/dev"], skipProjectScan: true))
    #expect(skipped.candidates.isEmpty)
    #expect(skipped.categories[.projectBuild] == nil)
}

@Test func presetBeatsArtifactPatternForTheSameDirectory() async throws {
    let home = try TemporaryHome()
    defer { home.cleanUp() }
    // `xdg-cache` expands its children, so `~/.cache/target` is a preset
    // candidate and also matches the `target` artifact pattern under the root.
    try home.writeFile(".cache/target/a.o", size: 10)

    let result = await DiskScanner().collect(configuration(home, roots: ["~/.cache"], skipProjectScan: false))

    #expect(result.candidates.map(\.rule.id) == ["xdg-cache"])
    #expect(result.candidates.first?.url.path == home.path(".cache/target").path)
}
