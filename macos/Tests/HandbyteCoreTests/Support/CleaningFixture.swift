import Foundation
@testable import HandbyteCore

/// A throwaway home directory under the temporary directory, plus the bookkeeping
/// needed to remove Trash entries that a test moved there.
///
/// `FileManager.trashItem` always lands in the real `~/.Trash`; no API points it
/// at a temporary trash. Register every trashed URL with `trackTrashed` so the
/// fixture removes it again.
final class CleaningFixture {
    let root: URL
    let home: URL
    private(set) var trashed: [URL] = []

    init() throws {
        root = URL(filePath: NSTemporaryDirectory(), directoryHint: .isDirectory)
            .appending(path: "handbyte-cleaning-\(UUID().uuidString)", directoryHint: .isDirectory)
            .standardizedFileURL
        home = root.appending(path: "home", directoryHint: .isDirectory)
        try FileManager.default.createDirectory(at: home, withIntermediateDirectories: true)
    }

    deinit {
        cleanUp()
    }

    /// A directory under `home` holding one file of `fileBytes` bytes.
    @discardableResult
    func makeDirectory(_ relativePath: String, fileBytes: Int = 4096) throws -> URL {
        let url = home.appending(path: relativePath, directoryHint: .isDirectory)
        try FileManager.default.createDirectory(at: url, withIntermediateDirectories: true)
        if fileBytes > 0 {
            let payload = Data(repeating: 0x41, count: fileBytes)
            try payload.write(to: url.appending(path: "payload.bin"))
        }
        return url
    }

    func makeSymlink(_ relativePath: String, to destination: URL) throws -> URL {
        let url = home.appending(path: relativePath)
        try FileManager.default.createDirectory(at: url.deletingLastPathComponent(), withIntermediateDirectories: true)
        try FileManager.default.createSymbolicLink(at: url, withDestinationURL: destination)
        return url
    }

    func rule(
        id: String = "library-caches",
        safety: Safety = .auto,
        owners: [String] = []
    ) -> Rule {
        Rule(
            id: id,
            kind: .preset(pathTemplate: "~/Library/Caches", expandChildren: true),
            category: .systemCache,
            safety: safety,
            ownerBundleIDs: owners,
            summary: "Test rule.",
            recovery: "Rebuilt on demand."
        )
    }

    /// A candidate whose identity is read from the file system right now, the
    /// way a scan would record it.
    func candidate(
        at url: URL,
        rule: Rule? = nil,
        allocatedBytes: Int64 = 4096
    ) throws -> Candidate {
        let status = try FileProbe.lstat(url).get()
        return Candidate(
            rule: rule ?? self.rule(),
            url: url,
            identity: status.identity,
            size: SizeReport(allocatedBytes: allocatedBytes, fileCount: 1, partial: false, errorCount: 0),
            lastActivity: nil,
            eligibility: .eligible
        )
    }

    /// A candidate for a path that is never stat-ed by the test, for protected
    /// roots and other checks that must fail before touching the file system.
    func candidate(atUnprobed url: URL, rule: Rule? = nil) -> Candidate {
        Candidate(
            rule: rule ?? self.rule(),
            url: url,
            identity: FileIdentity(dev: 1, ino: 1),
            size: SizeReport(allocatedBytes: 1, fileCount: 1, partial: false, errorCount: 0),
            lastActivity: nil,
            eligibility: .eligible
        )
    }

    func context(
        projectRoots: [URL] = [],
        exclusions: Set<String> = [],
        running: Set<String> = []
    ) -> PolicyContext {
        PolicyContext(
            home: home,
            projectRoots: projectRoots,
            exclusions: exclusions,
            runningApps: StaticRunningAppsProbe(running: running)
        )
    }

    func policy(
        projectRoots: [URL] = [],
        exclusions: Set<String> = [],
        running: Set<String> = []
    ) -> EligibilityPolicy {
        EligibilityPolicy(context: context(projectRoots: projectRoots, exclusions: exclusions, running: running))
    }

    func trackTrashed(_ url: URL) {
        trashed.append(url)
    }

    func cleanUp() {
        for url in trashed {
            try? FileManager.default.removeItem(at: url)
        }
        trashed.removeAll()
        try? FileManager.default.removeItem(at: root)
    }
}

/// A value guarded by a lock, for collecting results from `@Sendable` callbacks.
/// (`Synchronization.Mutex` needs macOS 15; the package targets 14.)
final class LockedBox<Value>: @unchecked Sendable {
    private let lock = NSLock()
    private var value: Value

    init(_ value: Value) {
        self.value = value
    }

    func withLock<Result>(_ body: (inout Value) throws -> Result) rethrows -> Result {
        lock.lock()
        defer { lock.unlock() }
        return try body(&value)
    }
}
