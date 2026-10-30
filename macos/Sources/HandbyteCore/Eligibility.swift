import Darwin
import Foundation

/// What the policy needs besides the candidate itself.
public struct PolicyContext: Sendable {
    public var home: URL
    public var projectRoots: [URL]
    /// Rule ids, candidate ids or absolute paths the user marked "never clean this".
    public var exclusions: Set<String>
    public var runningApps: any RunningAppsProbe

    public init(home: URL, projectRoots: [URL], exclusions: Set<String>, runningApps: any RunningAppsProbe) {
        self.home = home
        self.projectRoots = projectRoots
        self.exclusions = exclusions
        self.runningApps = runningApps
    }
}

/// The safety gate. The scanner runs it once to label candidates and the
/// cleaner runs it again immediately before each action.
///
/// Checks run in this order and the first failure wins:
/// 1. user exclusion;
/// 2. protected root (`/`, home, `~/Library`, `~/Library/Containers`, each
///    project root), compared on standardized paths without touching the disk;
/// 3. symbolic link (`lstat`);
/// 4. identity: a different `(dev, ino)` than the scan recorded, or nothing
///    at the path any more;
/// 5. an owner app of the rule is running.
///
/// Those produce `blocked`, which the cleaner refuses. A candidate that passes
/// them is `reviewOnly` when its rule is `Safety.caution` (the UI's "hold"
/// tier, kept out of every bulk selection) and `eligible` otherwise;
/// `Safety.review` stays `eligible` and the UI renders it as the review tier.
public struct EligibilityPolicy: Sendable {
    public let context: PolicyContext
    private let exclusions: Set<String>
    private let protectedRoots: Set<String>

    public init(context: PolicyContext) {
        self.context = context
        exclusions = Set(context.exclusions.map { entry in
            entry.hasPrefix("/") ? Self.normalizedPath(URL(filePath: entry)) : entry
        })
        var roots: Set<String> = [
            "/",
            Self.normalizedPath(context.home),
            Self.normalizedPath(context.home.appending(path: "Library")),
            Self.normalizedPath(context.home.appending(path: "Library/Containers")),
        ]
        for root in context.projectRoots {
            roots.insert(Self.normalizedPath(root))
        }
        protectedRoots = roots
    }

    public func evaluate(_ candidate: Candidate) -> Eligibility {
        let path = Self.normalizedPath(candidate.url)
        if exclusions.contains(candidate.rule.id) || exclusions.contains(candidate.id) || exclusions.contains(path) {
            return .blocked(reason: "Excluded by user")
        }
        if protectedRoots.contains(path) {
            return .blocked(reason: "Protected location")
        }
        switch FileProbe.lstat(candidate.url) {
        case .failure(.missing):
            return .blocked(reason: "Missing")
        case .failure(.other(let errno)):
            return .blocked(reason: "Cannot inspect: \(String(cString: strerror(errno)))")
        case .success(let status):
            if status.isSymlink {
                return .blocked(reason: "Symbolic link")
            }
            if status.identity != candidate.identity {
                return .blocked(reason: "Changed since scan")
            }
        }
        if let owner = candidate.rule.ownerBundleIDs.first(where: context.runningApps.isRunning(bundleID:)) {
            return .blocked(reason: "\(owner) is running")
        }
        if candidate.rule.safety == .caution {
            return .reviewOnly(reason: "Needs deliberate confirmation")
        }
        return .eligible
    }

    public func apply(to candidates: [Candidate]) -> [Candidate] {
        candidates.map { candidate in
            Candidate(
                rule: candidate.rule,
                url: candidate.url,
                identity: candidate.identity,
                size: candidate.size,
                lastActivity: candidate.lastActivity,
                eligibility: evaluate(candidate)
            )
        }
    }

    /// Absolute path without `.`/`..` segments or a trailing slash, so that the
    /// same directory written two ways compares equal. Symlinks are not
    /// resolved: the candidate path itself is what the user sees and selected.
    static func normalizedPath(_ url: URL) -> String {
        let path = url.standardizedFileURL.path(percentEncoded: false)
        if path.count > 1, path.hasSuffix("/") {
            return String(path.dropLast())
        }
        return path
    }
}

/// `lstat` reduced to what the policy and the tests need.
enum FileProbe {
    struct Status: Sendable, Hashable {
        let identity: FileIdentity
        let isSymlink: Bool
    }

    enum Failure: Error, Hashable {
        case missing
        case other(errno: Int32)
    }

    /// The trailing slash of a directory URL is dropped first: `lstat("link/")`
    /// follows the link, which would hide a symlink swap.
    static func lstat(_ url: URL) -> Result<Status, Failure> {
        var st = stat()
        guard Darwin.lstat(EligibilityPolicy.normalizedPath(url), &st) == 0 else {
            return .failure(errno == ENOENT || errno == ENOTDIR ? .missing : .other(errno: errno))
        }
        return .success(Status(
            identity: FileIdentity(dev: UInt64(UInt32(bitPattern: st.st_dev)), ino: UInt64(st.st_ino)),
            isSymlink: (st.st_mode & S_IFMT) == S_IFLNK
        ))
    }
}
