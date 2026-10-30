import Foundation
import Testing
import os
@testable import HandbyteCore

@Test func hardLinkedFileIsCountedOnce() throws {
    let home = try TemporaryHome()
    defer { home.cleanUp() }
    try home.writeFile("cache/a.bin", size: 10_000)
    try home.hardLink("cache/b.bin", to: "cache/a.bin")
    try home.hardLink("cache/sub/c.bin", to: "cache/a.bin")
    let expected = try home.allocatedBytes("cache/a.bin")

    let report = DiskUsage.measure(home.path("cache"), hardLinks: HardLinkRegistry())

    #expect(report.fileCount == 1)
    #expect(report.allocatedBytes == expected)
    #expect(report.partial == false)
    #expect(report.errorCount == 0)
}

@Test func hardLinkSharedByTwoWalksIsClaimedByTheFirst() throws {
    let home = try TemporaryHome()
    defer { home.cleanUp() }
    try home.writeFile("one/a.bin", size: 10_000)
    try home.makeDirectory("two")
    try home.hardLink("two/a.bin", to: "one/a.bin")
    let registry = HardLinkRegistry()

    let first = DiskUsage.measure(home.path("one"), hardLinks: registry)
    let second = DiskUsage.measure(home.path("two"), hardLinks: registry)

    #expect(first.fileCount == 1)
    #expect(second.fileCount == 0)
    #expect(second.allocatedBytes == 0)
    #expect(DiskUsage.measure(home.path("two"), hardLinks: HardLinkRegistry()).fileCount == 1)
}

@Test func symlinksAreNotFollowedOrCounted() throws {
    let home = try TemporaryHome()
    defer { home.cleanUp() }
    try home.writeFile("cache/real.bin", size: 4_096)
    try home.writeFile("outside/big.bin", size: 1_000_000)
    try home.symlink("cache/to-dir", to: "outside")
    try home.symlink("cache/to-file", to: "outside/big.bin")
    try home.symlink("cache/dangling", to: "missing")

    let report = DiskUsage.measure(home.path("cache"), hardLinks: HardLinkRegistry())

    #expect(report.fileCount == 1)
    #expect(report.allocatedBytes == (try home.allocatedBytes("cache/real.bin")))
    #expect(report.partial == false)
}

@Test func unreadableSubdirectoryMarksPartialAndKeepsReadableTotal() throws {
    let home = try TemporaryHome()
    defer { home.cleanUp() }
    try home.writeFile("cache/readable/a.bin", size: 8_192)
    try home.writeFile("cache/locked/hidden.bin", size: 8_192)
    try home.makeUnreadable("cache/locked")

    let report = DiskUsage.measure(home.path("cache"), hardLinks: HardLinkRegistry())

    #expect(report.partial == true)
    #expect(report.errorCount == 1)
    #expect(report.fileCount == 1)
    #expect(report.allocatedBytes == (try home.allocatedBytes("cache/readable/a.bin")))
}

@Test func reportsNewestModificationTime() throws {
    let home = try TemporaryHome()
    defer { home.cleanUp() }
    try home.writeFile("cache/old.bin", size: 10)
    try home.writeFile("cache/sub/new.bin", size: 10)
    let old = Date(timeIntervalSince1970: 1_000_000)
    let new = Date(timeIntervalSince1970: 1_600_000_000)
    try home.setModificationDate(old, of: "cache/old.bin")
    try home.setModificationDate(new, of: "cache/sub/new.bin")
    try home.setModificationDate(old, of: "cache/sub")

    let report = DiskUsage.measure(home.path("cache"), hardLinks: HardLinkRegistry())

    #expect(report.lastModified.map { abs($0.timeIntervalSince(new)) < 1 } == true)
}

@Test func missingDirectoryIsPartialWithOneError() throws {
    let home = try TemporaryHome()
    defer { home.cleanUp() }

    let report = DiskUsage.measure(home.path("nope"), hardLinks: HardLinkRegistry())

    #expect(report.partial == true)
    #expect(report.errorCount == 1)
    #expect(report.fileCount == 0)
}

@Test func cancellationStopsTheWalkAndMarksItCancelled() throws {
    let home = try TemporaryHome()
    defer { home.cleanUp() }
    for index in 0..<20 {
        try home.writeFile("cache/f\(index).bin", size: 10)
    }

    let report = DiskUsage.measure(home.path("cache"), hardLinks: HardLinkRegistry(), isCancelled: { true })

    #expect(report.cancelled == true)
    #expect(report.partial == true)
    #expect(report.fileCount == 0)
}

@Test func progressIsReportedWithRunningTotals() throws {
    let home = try TemporaryHome()
    defer { home.cleanUp() }
    for index in 0..<(DiskUsage.progressInterval * 2 + 1) {
        try home.writeFile("cache/f\(index).bin", size: 1)
    }
    let seen = OSAllocatedUnfairLock(initialState: [Int]())

    let report = DiskUsage.measure(home.path("cache"), hardLinks: HardLinkRegistry(), progress: { path, _, files in
        #expect(path.hasPrefix(home.path("cache").path))
        seen.withLock { $0.append(files) }
    })

    #expect(seen.withLock { $0 } == [DiskUsage.progressInterval, DiskUsage.progressInterval * 2])
    #expect(report.fileCount == DiskUsage.progressInterval * 2 + 1)
}
