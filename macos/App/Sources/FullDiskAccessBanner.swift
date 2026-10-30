import HandbyteCore
import SwiftUI

/// Shown while the probe reports denied. The grant takes effect after a
/// relaunch; the next scan probes again and the banner goes away.
struct FullDiskAccessBanner: View {
    @Environment(AppStore.self) private var store

    var body: some View {
        if store.fullDiskAccess == .denied {
            HStack(spacing: 12) {
                Image(systemName: "lock.shield.fill")
                    .foregroundStyle(.orange)
                VStack(alignment: .leading, spacing: 2) {
                    Text("Full Disk Access is off")
                        .font(.headline)
                    Text("Scans skip protected folders such as Mail and Safari data, so sizes are incomplete. Turn Handbyte on under Privacy & Security, then relaunch and scan again.")
                        .font(.callout)
                        .foregroundStyle(.secondary)
                }
                Spacer()
                Button("Open System Settings", action: store.openFullDiskAccessSettings)
                Button("Check Again", action: store.probeFullDiskAccess)
            }
            .padding(.horizontal, 16)
            .padding(.vertical, 10)
            .background(.orange.opacity(0.12))
            .overlay(alignment: .bottom) { Divider() }
            .accessibilityElement(children: .contain)
            .accessibilityLabel("Full Disk Access is off")
        }
    }
}

/// One line in the review footer and the report when the scan ran without access.
struct MissingAccessNote: View {
    var body: some View {
        Label("Scanned without Full Disk Access: protected folders were skipped", systemImage: "lock.shield")
            .foregroundStyle(.orange)
    }
}
