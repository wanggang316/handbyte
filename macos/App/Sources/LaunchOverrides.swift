import Foundation
import HandbyteCore

/// Environment variables for manual testing against a disposable fixture.
/// Run the binary inside the bundle directly so the shell environment
/// reaches it, for example:
///
///     HANDBYTE_MIN_SIZE=1K HANDBYTE_PROJECT_ROOTS=$TMPDIR/hb-fixture \
///       .dd/Build/Products/Debug/Handbyte.app/Contents/MacOS/Handbyte
///
/// - `HANDBYTE_MIN_SIZE`: minimum candidate size in the Go CLI's syntax (`50M`).
/// - `HANDBYTE_PROJECT_ROOTS`: comma-separated project roots that replace the defaults.
enum LaunchOverrides {
    static func apply(to configuration: inout ScanConfiguration, environment: [String: String] = ProcessInfo.processInfo.environment) {
        if let text = environment["HANDBYTE_MIN_SIZE"], let size = ByteCount.parse(text) {
            configuration.minimumSize = size
        }
        if let roots = environment["HANDBYTE_PROJECT_ROOTS"] {
            let list = roots.split(separator: ",").map(String.init).filter { !$0.isEmpty }
            if !list.isEmpty {
                configuration.projectRoots = list
            }
        }
    }
}
