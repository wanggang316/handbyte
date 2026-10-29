import Foundation

/// A throwaway home directory under the system temporary directory. Tests
/// build fixtures with the helpers below and never touch real user data.
final class TemporaryHome: @unchecked Sendable {
    let url: URL
    private let fileManager = FileManager.default
    private var lockedDirectories: [URL] = []

    init() throws {
        let base = URL(filePath: NSTemporaryDirectory(), directoryHint: .isDirectory)
            .appending(path: "handbyte-tests", directoryHint: .isDirectory)
            .appending(path: UUID().uuidString, directoryHint: .isDirectory)
        try fileManager.createDirectory(at: base, withIntermediateDirectories: true)
        // Resolve /var -> /private/var once so fixture paths compare equal to
        // the paths the walker reports.
        url = base.resolvingSymlinksInPath()
    }

    deinit {
        cleanUp()
    }

    /// Restores permissions changed by `makeUnreadable` and removes the tree.
    func cleanUp() {
        for directory in lockedDirectories {
            try? fileManager.setAttributes([.posixPermissions: 0o700], ofItemAtPath: directory.path)
        }
        lockedDirectories.removeAll()
        try? fileManager.removeItem(at: url)
    }

    func path(_ relative: String) -> URL {
        url.appending(path: relative, directoryHint: .isDirectory)
    }

    @discardableResult
    func makeDirectory(_ relative: String) throws -> URL {
        let directory = path(relative)
        try fileManager.createDirectory(at: directory, withIntermediateDirectories: true)
        return directory
    }

    /// Writes `size` non-zero bytes so the file allocates at least `size` on disk.
    @discardableResult
    func writeFile(_ relative: String, size: Int) throws -> URL {
        let file = url.appending(path: relative, directoryHint: .notDirectory)
        try fileManager.createDirectory(at: file.deletingLastPathComponent(), withIntermediateDirectories: true)
        try Data(repeating: 0x61, count: size).write(to: file)
        return file
    }

    func hardLink(_ relative: String, to target: String) throws {
        let link = url.appending(path: relative, directoryHint: .notDirectory)
        try fileManager.createDirectory(at: link.deletingLastPathComponent(), withIntermediateDirectories: true)
        try fileManager.linkItem(at: url.appending(path: target), to: link)
    }

    func symlink(_ relative: String, to target: String) throws {
        try fileManager.createSymbolicLink(
            at: url.appending(path: relative),
            withDestinationURL: url.appending(path: target)
        )
    }

    /// chmod 000; reverted by `cleanUp` so the temporary tree can be removed.
    func makeUnreadable(_ relative: String) throws {
        let directory = path(relative)
        try fileManager.setAttributes([.posixPermissions: 0o000], ofItemAtPath: directory.path)
        lockedDirectories.append(directory)
    }

    func setModificationDate(_ date: Date, of relative: String) throws {
        try fileManager.setAttributes([.modificationDate: date], ofItemAtPath: url.appending(path: relative).path)
    }

    /// Allocated size the way the walker measures it: st_blocks * 512.
    func allocatedBytes(_ relative: String) throws -> Int64 {
        var status = stat()
        guard lstat(url.appending(path: relative).path, &status) == 0 else {
            throw POSIXError(POSIXErrorCode(rawValue: errno) ?? .EIO)
        }
        return Int64(status.st_blocks) * 512
    }
}
