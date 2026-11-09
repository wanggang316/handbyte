import Foundation
import Testing
@testable import HandbyteCore

@Suite("FullDiskAccess")
struct FullDiskAccessTests {
    @Test func settingsURLOpensThePrivacyPane() {
        #expect(FullDiskAccess.settingsURL.absoluteString == "x-apple.systempreferences:com.apple.preference.security?Privacy_AllFiles")
    }

    @Test func noProbeLocationMeansUnknown() throws {
        let fixture = try CleaningFixture()
        #expect(FullDiskAccess.probe(home: fixture.home) == .unknown)
    }

    @Test func readableProbeLocationMeansGranted() throws {
        let fixture = try CleaningFixture()
        try fixture.makeDirectory("Library/Safari", fileBytes: 0)
        #expect(FullDiskAccess.probe(home: fixture.home) == .granted)
    }

    @Test func unlistableProbeLocationMeansDenied() throws {
        let fixture = try CleaningFixture()
        let safari = try fixture.makeDirectory("Library/Safari", fileBytes: 0)
        try FileManager.default.setAttributes([.posixPermissions: 0o000], ofItemAtPath: safari.path)
        defer { try? FileManager.default.setAttributes([.posixPermissions: 0o755], ofItemAtPath: safari.path) }

        #expect(FullDiskAccess.probe(home: fixture.home) == .denied)
    }

    @Test func probeLocationsAreTheOnesTCCGuards() {
        #expect(FullDiskAccess.probePaths.contains("~/Library/Safari"))
        #expect(FullDiskAccess.probePaths.allSatisfy { $0.hasPrefix("~/Library/") })
    }
}
