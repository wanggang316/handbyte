import Foundation
import HandbyteCore

/// Environment variables for manual testing against a disposable fixture.
/// They win over the settings file for this launch only. Run the binary
/// inside the bundle directly so the shell environment reaches it, for example:
///
///     HANDBYTE_MIN_SIZE=1K HANDBYTE_PROJECT_ROOTS=$TMPDIR/hb-fixture \
///       .dd/Build/Products/Debug/Handbyte.app/Contents/MacOS/Handbyte
///
/// - `HANDBYTE_MIN_SIZE`: minimum candidate size in the Go CLI's syntax (`50M`).
/// - `HANDBYTE_PROJECT_ROOTS`: comma-separated project roots that replace the defaults.
enum LaunchOverrides {
    enum Field: Hashable {
        case minimumSize
        case projectRoots
    }

    static func apply(to configuration: inout ScanConfiguration, environment: [String: String] = ProcessInfo.processInfo.environment) {
        if let size = minimumSize(environment) {
            configuration.minimumSize = size
        }
        if let roots = projectRoots(environment) {
            configuration.projectRoots = roots
        }
    }

    /// Which settings the environment replaces, so the settings window can say so.
    static func overridden(environment: [String: String] = ProcessInfo.processInfo.environment) -> Set<Field> {
        var fields: Set<Field> = []
        if minimumSize(environment) != nil { fields.insert(.minimumSize) }
        if projectRoots(environment) != nil { fields.insert(.projectRoots) }
        return fields
    }

    private static func minimumSize(_ environment: [String: String]) -> Int64? {
        environment["HANDBYTE_MIN_SIZE"].flatMap(ByteCount.parse)
    }

    private static func projectRoots(_ environment: [String: String]) -> [String]? {
        guard let roots = environment["HANDBYTE_PROJECT_ROOTS"] else { return nil }
        let list = roots.split(separator: ",").map(String.init).filter { !$0.isEmpty }
        return list.isEmpty ? nil : list
    }
}
