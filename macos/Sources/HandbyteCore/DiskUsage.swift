import Darwin
import Foundation
import os

/// Hard links seen during one scan. Shared by every walk of that scan so a
/// file reachable from two candidates is claimed by exactly one of them.
///
/// Attribution rule: the first walk to reach an inode claims it. Walks run
/// concurrently, so which of two overlapping candidates gets the bytes is not
/// stable between scans; the sum over all candidates is.
public final class HardLinkRegistry: Sendable {
    private let seen = OSAllocatedUnfairLock(initialState: Set<FileIdentity>())

    public init() {}

    /// True when `identity` was not claimed before.
    public func claim(_ identity: FileIdentity) -> Bool {
        seen.withLock { $0.insert(identity).inserted }
    }
}

/// Result of one directory walk.
public struct DiskUsageReport: Sendable, Hashable {
    /// st_blocks * 512 summed over counted entries. A lower bound when `partial`.
    public var allocatedBytes: Int64 = 0
    public var fileCount: Int = 0
    /// Some entries could not be read, or the walk was cancelled.
    public var partial: Bool = false
    /// Entries that reported FTS_DNR, FTS_ERR or FTS_NS.
    public var errorCount: Int = 0
    /// Newest modification time over counted entries.
    public var lastModified: Date?
    /// The walk stopped early because the task was cancelled.
    public var cancelled: Bool = false

    public init() {}

    public var sizeReport: SizeReport {
        SizeReport(allocatedBytes: allocatedBytes, fileCount: fileCount, partial: partial, errorCount: errorCount)
    }
}

/// lstat(2) facts the scanner needs before it walks a directory.
public struct FileStatus: Sendable, Hashable {
    public let identity: FileIdentity
    public let isDirectory: Bool
    public let isSymbolicLink: Bool
    public let modificationDate: Date

    /// nil when the path does not exist or cannot be stat'ed.
    public init?(lstatPath path: String) {
        var status = stat()
        guard lstat(path, &status) == 0 else { return nil }
        self.init(status)
    }

    init(_ status: stat) {
        identity = FileIdentity(dev: UInt64(UInt32(bitPattern: status.st_dev)), ino: UInt64(status.st_ino))
        let type = status.st_mode & S_IFMT
        isDirectory = type == S_IFDIR
        isSymbolicLink = type == S_IFLNK
        modificationDate = Date(status.st_mtimespec)
    }
}

extension URL {
    /// The path handed to fts(3), lstat(2) and shown to users. Directory URLs
    /// carry a trailing slash that fts would echo into every child path.
    public var fileSystemPath: String {
        let path = path(percentEncoded: false)
        if path.count > 1, path.hasSuffix("/") {
            return String(path.dropLast())
        }
        return path
    }
}

extension Date {
    init(_ time: timespec) {
        self.init(timeIntervalSince1970: TimeInterval(time.tv_sec) + TimeInterval(time.tv_nsec) / 1e9)
    }
}

/// Measures allocated bytes of a directory tree with fts(3).
///
/// Physical walk: symlinks are reported but never followed, directories
/// contribute no bytes, and the walk does not cross device boundaries.
public enum DiskUsage {
    /// Called every `progressInterval` counted files with the running totals.
    public typealias ProgressHandler = @Sendable (_ currentPath: String, _ bytes: Int64, _ files: Int) -> Void

    static let progressInterval = 256

    public static func measure(
        _ url: URL,
        hardLinks: HardLinkRegistry,
        isCancelled: @Sendable () -> Bool = { Task.isCancelled },
        progress: ProgressHandler? = nil
    ) -> DiskUsageReport {
        var report = DiskUsageReport()
        let root = strdup(url.fileSystemPath)
        defer { free(root) }
        var paths: [UnsafeMutablePointer<CChar>?] = [root, nil]
        let options = FTS_PHYSICAL | FTS_NOCHDIR | FTS_XDEV
        guard let fts = paths.withUnsafeMutableBufferPointer({ fts_open($0.baseAddress!, options, nil) }) else {
            report.partial = true
            report.errorCount = 1
            return report
        }
        defer { fts_close(fts) }

        var sinceProgress = 0
        while let entry = fts_read(fts) {
            if isCancelled() {
                report.partial = true
                report.cancelled = true
                break
            }
            switch Int32(entry.pointee.fts_info) {
            case FTS_DNR, FTS_ERR, FTS_NS:
                report.partial = true
                report.errorCount += 1
                continue
            case FTS_D, FTS_DP, FTS_DC, FTS_DOT, FTS_SL, FTS_SLNONE:
                continue
            default:
                break
            }
            guard let status = entry.pointee.fts_statp?.pointee else { continue }
            if status.st_nlink > 1 {
                let identity = FileIdentity(dev: UInt64(UInt32(bitPattern: status.st_dev)), ino: UInt64(status.st_ino))
                guard hardLinks.claim(identity) else { continue }
            }
            report.allocatedBytes += Int64(status.st_blocks) * 512
            report.fileCount += 1
            let modified = Date(status.st_mtimespec)
            if report.lastModified.map({ modified > $0 }) ?? true {
                report.lastModified = modified
            }

            sinceProgress += 1
            if sinceProgress >= progressInterval, let progress {
                sinceProgress = 0
                progress(String(cString: entry.pointee.fts_path), report.allocatedBytes, report.fileCount)
            }
        }
        return report
    }
}
