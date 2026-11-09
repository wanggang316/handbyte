import Darwin
import Foundation

/// Detects Full Disk Access by trying to list a directory that TCC guards.
/// `stat` on such a directory succeeds without the grant; only `opendir`
/// fails, with EPERM. The app must be relaunched after the user toggles the
/// setting, and ad-hoc signed Debug builds get a fresh TCC entry per build.
public enum FullDiskAccess {
    public enum Status: Sendable, Hashable {
        case granted
        case denied
        /// None of the probe directories exists, so nothing can be concluded.
        case unknown
    }

    /// Tried in order; the first one that exists decides. `~/Library/Safari`
    /// is present on every macOS from 14 on, Mail only once Mail was opened.
    public static let probePaths: [String] = [
        "~/Library/Safari",
        "~/Library/Containers/com.apple.Safari",
        "~/Library/Mail",
        "~/Library/Containers/com.apple.mail",
    ]

    /// The Privacy & Security > Full Disk Access pane in System Settings.
    public static let settingsURL = URL(string: "x-apple.systempreferences:com.apple.preference.security?Privacy_AllFiles")!

    public static func probe(home: URL = FileManager.default.homeDirectoryForCurrentUser) -> Status {
        for template in probePaths {
            let path = PathTemplate.expand(template, home: home).path(percentEncoded: false)
            var st = stat()
            guard stat(path, &st) == 0, (st.st_mode & S_IFMT) == S_IFDIR else { continue }
            if let dir = opendir(path) {
                closedir(dir)
                return .granted
            }
            if errno == EPERM || errno == EACCES {
                return .denied
            }
        }
        return .unknown
    }
}
