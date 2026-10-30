import Foundation
import HandbyteCore

/// The user's persistent choices. Defaults equal a `ScanConfiguration` with
/// Trash deletion, so a fresh install behaves like the Go CLI.
public struct Settings: Sendable, Hashable, Codable {
    public enum DeletionMode: String, Sendable, Hashable, Codable, CaseIterable {
        case trash
        case removePermanently
    }

    public var deletionMode: DeletionMode = .trash
    /// Templates (`~/dev`) or absolute paths, in the order the user keeps them.
    public var projectRoots: [String] = Catalog.defaultProjectRoots
    /// Rule ids or absolute paths the policy blocks with "Excluded by user".
    public var exclusions: [String] = []
    public var minimumSize: Int64 = 50 * 1024 * 1024

    public init() {}

    private enum CodingKeys: String, CodingKey {
        case deletionMode, projectRoots, exclusions, minimumSize
    }

    /// A field missing from an older or hand-edited file keeps its default.
    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        let defaults = Settings()
        deletionMode = try container.decodeIfPresent(DeletionMode.self, forKey: .deletionMode) ?? defaults.deletionMode
        projectRoots = try container.decodeIfPresent([String].self, forKey: .projectRoots) ?? defaults.projectRoots
        exclusions = try container.decodeIfPresent([String].self, forKey: .exclusions) ?? defaults.exclusions
        minimumSize = try container.decodeIfPresent(Int64.self, forKey: .minimumSize) ?? defaults.minimumSize
    }
}

/// The on-disk format: a JSON object with a `version` and the `settings`.
///
/// A JSON file under Application Support was chosen over `UserDefaults`
/// because it is home-relative like the operations log (tests inject a
/// fixture home and never touch the real preferences domain), the user can
/// read and back it up, and a version number makes a future migration an
/// explicit step rather than a guess over loose keys.
public enum SettingsFile {
    public static let currentVersion = 1

    public enum Error: Swift.Error, Hashable {
        case unsupportedVersion(Int)
    }

    private struct Envelope: Codable {
        var version: Int
        var settings: Settings
    }

    private struct VersionOnly: Decodable {
        var version: Int
    }

    public static func encode(_ settings: Settings) throws -> Data {
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
        return try encoder.encode(Envelope(version: currentVersion, settings: settings))
    }

    public static func decode(_ data: Data) throws -> Settings {
        let decoder = JSONDecoder()
        let version = try decoder.decode(VersionOnly.self, from: data).version
        guard version == currentVersion else {
            throw Error.unsupportedVersion(version)
        }
        return try decoder.decode(Envelope.self, from: data).settings
    }
}

/// Reads and writes the settings file. Writes are atomic so a crash mid-save
/// leaves the previous file intact.
public struct SettingsStore: Sendable {
    public let fileURL: URL

    public init(fileURL: URL) {
        self.fileURL = fileURL
    }

    /// `~/Library/Application Support/Handbyte/settings.json` for the given home.
    public static func defaultURL(home: URL) -> URL {
        home.appending(path: "Library/Application Support/Handbyte/settings.json")
    }

    /// Defaults when no file exists; throws when the file cannot be decoded,
    /// so the caller decides whether to fall back or warn.
    public func load() throws -> Settings {
        guard FileManager.default.fileExists(atPath: fileURL.path(percentEncoded: false)) else {
            return Settings()
        }
        return try SettingsFile.decode(try Data(contentsOf: fileURL))
    }

    public func save(_ settings: Settings) throws {
        try FileManager.default.createDirectory(at: fileURL.deletingLastPathComponent(), withIntermediateDirectories: true)
        try SettingsFile.encode(settings).write(to: fileURL, options: .atomic)
    }
}

// MARK: Scan and policy inputs

extension Settings {
    /// The configuration for one scan. Launch overrides are applied by the
    /// caller afterwards so they win over the file.
    public func scanConfiguration(home: URL) -> ScanConfiguration {
        var configuration = ScanConfiguration(home: home)
        configuration.projectRoots = projectRoots
        configuration.minimumSize = minimumSize
        return configuration
    }

    public func policyContext(home: URL, runningApps: any RunningAppsProbe) -> PolicyContext {
        PolicyContext(
            home: home,
            projectRoots: projectRoots.map { PathTemplate.expand($0, home: home) },
            exclusions: Set(exclusions),
            runningApps: runningApps
        )
    }
}

// MARK: Exclusions

extension Settings {
    /// "Never clean this" records the candidate's path, not its rule id or
    /// candidate id: a rule id would also hide every sibling of a child-expanded
    /// preset, and the candidate id changes when the directory is recreated.
    /// The path is what the user saw and keeps matching across rescans.
    public static func exclusionEntry(for candidate: Candidate) -> String {
        normalizedEntry(candidate.url.path(percentEncoded: false))
    }

    public mutating func exclude(_ candidate: Candidate) {
        addExclusion(Self.exclusionEntry(for: candidate))
    }

    public func excludes(_ candidate: Candidate) -> Bool {
        exclusions.contains(Self.exclusionEntry(for: candidate)) || exclusions.contains(candidate.rule.id)
    }

    /// Adds a rule id or an absolute path. Blank and duplicate entries are ignored.
    public mutating func addExclusion(_ entry: String) {
        let entry = Self.normalizedEntry(entry)
        guard !entry.isEmpty, !exclusions.contains(entry) else { return }
        exclusions.append(entry)
    }

    public mutating func removeExclusion(_ entry: String) {
        exclusions.removeAll { $0 == entry }
    }

    /// Trims whitespace and, for a path, the trailing slash, so one directory
    /// has one spelling in the list.
    private static func normalizedEntry(_ text: String) -> String {
        var entry = text.trimmingCharacters(in: .whitespacesAndNewlines)
        while entry.count > 1, entry.hasSuffix("/") {
            entry.removeLast()
        }
        return entry
    }
}

// MARK: Deletion mode

extension Settings.DeletionMode {
    public var cleanAction: CleanAction {
        switch self {
        case .trash: .trash
        case .removePermanently: .removePermanently
        }
    }

    /// Permanent deletion needs the user's explicit acknowledgement in the
    /// confirmation sheet; the Trash does not.
    public var requiresAcknowledgement: Bool {
        self == .removePermanently
    }
}

/// The single place that turns the deletion mode into a `CleanAction`.
/// `removePermanently` is produced only when the user acknowledged the
/// distinct confirmation; otherwise the request is refused with `nil`.
public enum CleanConfirmation {
    public static func action(for mode: Settings.DeletionMode, acknowledged: Bool) -> CleanAction? {
        if mode.requiresAcknowledgement, !acknowledged {
            return nil
        }
        return mode.cleanAction
    }
}
