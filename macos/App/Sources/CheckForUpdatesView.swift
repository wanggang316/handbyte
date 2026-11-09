import Sparkle
import SwiftUI

/// "Check for Updates…" menu item that follows Sparkle's own enabled state,
/// so it stays disabled while the updater is not started or is busy.
struct CheckForUpdatesView: View {
    @State private var canCheck = false
    private let updater: SPUUpdater

    init(updater: SPUUpdater) {
        self.updater = updater
    }

    var body: some View {
        Button("Check for Updates…") {
            updater.checkForUpdates()
        }
        .disabled(!canCheck)
        .onReceive(updater.publisher(for: \.canCheckForUpdates)) { canCheck = $0 }
    }
}
