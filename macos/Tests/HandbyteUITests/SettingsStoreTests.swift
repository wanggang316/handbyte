import Foundation
import Testing
import HandbyteCore
@testable import HandbyteUI

@Suite("Settings store")
struct SettingsStoreTests {
    private func temporaryHome() throws -> URL {
        let url = FileManager.default.temporaryDirectory
            .appending(path: "hb-settings-\(UUID().uuidString)", directoryHint: .isDirectory)
        try FileManager.default.createDirectory(at: url, withIntermediateDirectories: true)
        return url
    }

    @Test func fileLivesUnderApplicationSupport() {
        let home = URL(filePath: "/Users/alice", directoryHint: .isDirectory)
        #expect(SettingsStore.defaultURL(home: home).path(percentEncoded: false)
            == "/Users/alice/Library/Application Support/Handbyte/settings.json")
    }

    @Test func missingFileLoadsDefaults() throws {
        let home = try temporaryHome()
        defer { try? FileManager.default.removeItem(at: home) }
        let store = SettingsStore(fileURL: SettingsStore.defaultURL(home: home))
        #expect(try store.load() == Settings())
    }

    @Test func saveCreatesTheDirectoryAndLoadReadsItBack() throws {
        let home = try temporaryHome()
        defer { try? FileManager.default.removeItem(at: home) }
        let store = SettingsStore(fileURL: SettingsStore.defaultURL(home: home))
        var settings = Settings()
        settings.exclusions = ["/Users/alice/Library/Caches/keep"]
        settings.minimumSize = 4096

        try store.save(settings)

        #expect(FileManager.default.fileExists(atPath: store.fileURL.path(percentEncoded: false)))
        #expect(try store.load() == settings)
    }

    @Test func corruptFileFailsLoudly() throws {
        let home = try temporaryHome()
        defer { try? FileManager.default.removeItem(at: home) }
        let store = SettingsStore(fileURL: SettingsStore.defaultURL(home: home))
        try FileManager.default.createDirectory(at: store.fileURL.deletingLastPathComponent(), withIntermediateDirectories: true)
        try Data("not json".utf8).write(to: store.fileURL)

        #expect(throws: (any Error).self) { try store.load() }
    }
}
