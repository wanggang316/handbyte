import Foundation
import Testing
import HandbyteCore
@testable import HandbyteUI

@Suite("Settings")
struct SettingsTests {
    @Test func defaultsMatchTheScanConfigurationAndKeepTheTrash() {
        let settings = Settings()
        #expect(settings.deletionMode == .trash)
        #expect(settings.projectRoots == Catalog.defaultProjectRoots)
        #expect(settings.exclusions.isEmpty)
        #expect(settings.minimumSize == 50 * 1024 * 1024)
    }

    @Test func fileRoundTripsEveryField() throws {
        var settings = Settings()
        settings.deletionMode = .removePermanently
        settings.projectRoots = ["~/src", "/Volumes/Work"]
        settings.exclusions = ["xcode-archives", "/Users/alice/Library/Caches/keep"]
        settings.minimumSize = 1024

        let data = try SettingsFile.encode(settings)
        #expect(try SettingsFile.decode(data) == settings)
    }

    @Test func fileCarriesTheFormatVersion() throws {
        let data = try SettingsFile.encode(Settings())
        let object = try #require(try JSONSerialization.jsonObject(with: data) as? [String: Any])
        #expect(object["version"] as? Int == SettingsFile.currentVersion)
        #expect(object["settings"] is [String: Any])
    }

    @Test func missingFieldsFallBackToDefaults() throws {
        let data = Data(#"{"version": 1, "settings": {"deletionMode": "removePermanently"}}"#.utf8)
        let settings = try SettingsFile.decode(data)
        #expect(settings.deletionMode == .removePermanently)
        #expect(settings.projectRoots == Catalog.defaultProjectRoots)
        #expect(settings.minimumSize == 50 * 1024 * 1024)
        #expect(settings.exclusions.isEmpty)
    }

    @Test func newerVersionIsRejectedInsteadOfGuessed() {
        let data = Data(#"{"version": 99, "settings": {}}"#.utf8)
        #expect(throws: SettingsFile.Error.unsupportedVersion(99)) {
            try SettingsFile.decode(data)
        }
    }
}
