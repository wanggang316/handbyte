import HandbyteCore
import Sparkle
import SwiftUI

@main
struct HandbyteApp: App {
    // Not started yet: Sparkle refuses to start without SUPublicEDKey.
    // Ticket #9 adds the key and starts the updater.
    private let updaterController = SPUStandardUpdaterController(
        startingUpdater: false,
        updaterDelegate: nil,
        userDriverDelegate: nil
    )
    @State private var store = AppStore()

    var body: some Scene {
        WindowGroup {
            ContentView()
                .environment(store)
        }
    }
}
