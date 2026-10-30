import Darwin
import Foundation

/// Finds build-artifact directories under a project root by exact basename.
///
/// Matches are not entered, so `node_modules` nested inside `node_modules`
/// is counted with its parent, and version control metadata is never walked.
/// Symlinks are reported by the physical walk and never followed.
public enum ProjectArtifactFinder {
    public struct Result: Sendable, Hashable {
        /// Matched directories in walk order.
        public var matches: [URL] = []
        /// Directories that could not be read or stat'ed.
        public var errorCount: Int = 0
        /// The walk stopped early because the task was cancelled.
        public var cancelled: Bool = false

        public init() {}
    }

    static let versionControlDirectories: Set<String> = [".git", ".svn", ".hg"]

    public typealias ProgressHandler = @Sendable (_ currentPath: String) -> Void

    static let progressInterval = 512

    /// A root that does not exist or is not a directory yields an empty result.
    public static func find(
        under root: URL,
        directoryNames: Set<String>,
        isCancelled: @Sendable () -> Bool = { Task.isCancelled },
        progress: ProgressHandler? = nil
    ) -> Result {
        var result = Result()
        let rootPath = root.fileSystemPath
        guard FileStatus(lstatPath: rootPath)?.isDirectory == true else { return result }

        let cRoot = strdup(rootPath)
        defer { free(cRoot) }
        var paths: [UnsafeMutablePointer<CChar>?] = [cRoot, nil]
        guard let fts = paths.withUnsafeMutableBufferPointer({
            fts_open($0.baseAddress!, FTS_PHYSICAL | FTS_NOCHDIR, nil)
        }) else {
            result.errorCount = 1
            return result
        }
        defer { fts_close(fts) }

        var sinceProgress = 0
        while let entry = fts_read(fts) {
            if isCancelled() {
                result.cancelled = true
                break
            }
            switch Int32(entry.pointee.fts_info) {
            case FTS_DNR, FTS_ERR, FTS_NS:
                result.errorCount += 1
            case FTS_D:
                let path = String(cString: entry.pointee.fts_path)
                // fts_name is a flexible array member; the basename also ends fts_path.
                let nameOffset = Int(entry.pointee.fts_pathlen) - Int(entry.pointee.fts_namelen)
                let name = String(cString: entry.pointee.fts_path + nameOffset)
                if directoryNames.contains(name) {
                    result.matches.append(URL(filePath: path, directoryHint: .isDirectory))
                    fts_set(fts, entry, FTS_SKIP)
                } else if versionControlDirectories.contains(name) {
                    fts_set(fts, entry, FTS_SKIP)
                }
                sinceProgress += 1
                if sinceProgress >= progressInterval, let progress {
                    sinceProgress = 0
                    progress(path)
                }
            default:
                continue
            }
        }
        return result
    }
}
