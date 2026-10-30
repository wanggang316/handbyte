import Foundation

/// Groups candidates in the UI. Raw values are the strings the Go CLI prints.
public enum Category: String, Sendable, Codable, CaseIterable {
    case systemCache = "System & App Caches"
    case devCache = "Developer Caches"
    case xcode = "Xcode & iOS"
    case projectBuild = "Project Build Artifacts"
    case vms = "VMs & Containers"
    case trash = "Trash & Logs"
    case other = "Other Large Directories"
}

/// How risky deletion is. Raw values are the integers the Go CLI prints.
public enum Safety: Int, Sendable, Codable, CaseIterable, Comparable {
    /// Regenerable cache; safe to remove without review.
    case auto = 0
    /// Likely safe but worth a glance, for example project build artifacts.
    case review = 1
    /// User data or live state; needs deliberate confirmation.
    case caution = 2

    public static func < (lhs: Safety, rhs: Safety) -> Bool {
        lhs.rawValue < rhs.rawValue
    }
}

/// Expands the `~` shorthand the catalog uses for the home directory.
public enum PathTemplate {
    public static func expand(_ template: String, home: URL) -> URL {
        if template == "~" {
            return home
        }
        if template.hasPrefix("~/") {
            let relative = String(template.dropFirst(2))
            return home.appending(path: relative, directoryHint: .isDirectory)
        }
        return URL(filePath: template, directoryHint: .isDirectory)
    }
}

/// One compiled-in cleanup rule. `id` is the exact string the Go CLI prints as
/// `Source`, so both products can be compared by identifier.
public struct Rule: Sendable, Hashable, Codable {
    public enum Kind: Sendable, Hashable, Codable {
        /// A known location under the home directory, written with a leading `~/`.
        /// With `expandChildren` each direct child directory becomes its own candidate.
        case preset(pathTemplate: String, expandChildren: Bool)
        /// A directory name matched anywhere under the configured project roots.
        case projectArtifact(directoryName: String)
    }

    public let id: String
    public let kind: Kind
    public let category: Category
    public let safety: Safety
    /// Apps whose running state blocks cleanup of this rule's candidates.
    public let ownerBundleIDs: [String]
    /// What the item is.
    public let summary: String
    /// What happens after removal and how the data comes back.
    public let recovery: String

    public init(
        id: String,
        kind: Kind,
        category: Category,
        safety: Safety,
        ownerBundleIDs: [String],
        summary: String,
        recovery: String
    ) {
        self.id = id
        self.kind = kind
        self.category = category
        self.safety = safety
        self.ownerBundleIDs = ownerBundleIDs
        self.summary = summary
        self.recovery = recovery
    }

    public var pathTemplate: String? {
        if case .preset(let pathTemplate, _) = kind { return pathTemplate }
        return nil
    }

    public var expandChildren: Bool {
        if case .preset(_, let expandChildren) = kind { return expandChildren }
        return false
    }

    public var directoryName: String? {
        if case .projectArtifact(let directoryName) = kind { return directoryName }
        return nil
    }

    /// The preset location under `home`; nil for project artifact rules, which
    /// have no fixed location.
    public func location(in home: URL) -> URL? {
        pathTemplate.map { PathTemplate.expand($0, home: home) }
    }
}
