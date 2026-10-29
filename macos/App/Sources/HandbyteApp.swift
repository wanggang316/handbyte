import HandbyteCore
import Sparkle
import SwiftUI

@main
struct HandbyteApp: App {
    // Debug builds are ad-hoc signed and never match the appcast, so the
    // updater only starts in Release builds.
    private let updaterController = SPUStandardUpdaterController(
        startingUpdater: Self.startsUpdater,
        updaterDelegate: nil,
        userDriverDelegate: nil
    )

    private static var startsUpdater: Bool {
        #if DEBUG
        return false
        #else
        return true
        #endif
    }

    var body: some Scene {
        WindowGroup {
            ContentView()
        }
        .commands {
            CommandGroup(after: .appInfo) {
                CheckForUpdatesView(updater: updaterController.updater)
            }
        }
    }
}
