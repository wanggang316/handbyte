import Foundation
import HandbyteCore

/// Decimal units (1 MB = 1,000,000 bytes), the same as Finder and Get Info,
/// so the numbers match what the user sees elsewhere on the Mac.
public enum ByteFormat {
    public static func string(_ bytes: Int64, locale: Locale = .autoupdatingCurrent) -> String {
        bytes.formatted(.byteCount(style: .file, spellsOutZero: false).locale(locale))
    }

    /// A partial size is a lower bound and reads as "≥ 1 MB".
    public static func string(_ size: SizeReport, locale: Locale = .autoupdatingCurrent) -> String {
        let text = string(size.allocatedBytes, locale: locale)
        return size.partial ? "≥ \(text)" : text
    }
}
