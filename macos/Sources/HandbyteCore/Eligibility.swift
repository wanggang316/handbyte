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

/// The safety gate. The scanner labels every candidate `eligible`; the app
/// applies the policy to the finished scan and the cleaner runs it again
/// immediately before each action.
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
            entry.hasPrefix("/") ? URL(filePath: entry).standardizedFileSystemPath : entry
        })
        var roots: Set<String> = [
            "/",
            context.home.standardizedFileSystemPath,
            context.home.appending(path: "Library").standardizedFileSystemPath,
            context.home.appending(path: "Library/Containers").standardizedFileSystemPath,
        ]
        for root in context.projectRoots {
            roots.insert(root.standardizedFileSystemPath)
        }
        protectedRoots = roots
    }

    public func evaluate(_ candidate: Candidate) -> Eligibility {
        let path = candidate.url.standardizedFileSystemPath
        if exclusions.contains(candidate.rule.id) || exclusions.contains(candidate.id) || exclusions.contains(path) {
            return .blocked(reason: "Excluded by user")
        }
        if protectedRoots.contains(path) {
            return .blocked(reason: "Protected location")
        }
        switch FileStatus.lstat(path) {
        case .failure(.missing):
            return .blocked(reason: "Missing")
        case .failure(.other(let errno)):
            return .blocked(reason: "Cannot inspect: \(String(cString: strerror(errno)))")
        case .success(let status):
            if status.isSymbolicLink {
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
}
