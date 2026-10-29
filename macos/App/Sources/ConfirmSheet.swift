import HandbyteCore
import HandbyteUI
import SwiftUI

/// The complete list of what is about to move to the Trash. Return does not
/// confirm: only Cancel has a keyboard shortcut.
struct ConfirmSheet: View {
    @Environment(AppStore.self) private var store
    let candidates: [Candidate]

    private var totalBytes: Int64 {
        candidates.reduce(0) { $0 + $1.size.allocatedBytes }
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("Move \(candidates.count) items to the Trash?")
                .font(.title2.weight(.semibold))
            Text("Each item is checked again right before it moves. Items that changed since the scan are skipped and reported. You can put items back from the Trash in Finder.")
                .foregroundStyle(.secondary)
            List(candidates) { candidate in
                HStack {
                    VStack(alignment: .leading, spacing: 2) {
                        HStack(spacing: 6) {
                            Text(candidate.url.lastPathComponent)
                            SafetyBadge(safety: candidate.rule.safety)
                        }
                        Text(candidate.url.fileSystemPath)
                            .font(.caption.monospaced())
                            .foregroundStyle(.secondary)
                            .lineLimit(1)
                            .truncationMode(.middle)
                    }
                    Spacer()
                    Text("Move to Trash")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                    Text(ByteFormat.string(candidate.size))
                        .monospacedDigit()
                        .frame(width: 90, alignment: .trailing)
                }
            }
            .frame(minHeight: 240)
            HStack {
                Text("Total: \(ByteFormat.string(totalBytes))")
                    .monospacedDigit()
                Spacer()
                Button("Cancel", action: store.dismissConfirmation)
                    .keyboardShortcut(.cancelAction)
                Button("Move to Trash", action: store.confirmClean)
            }
        }
        .padding(20)
        .frame(minWidth: 640, idealWidth: 720, minHeight: 420)
    }
}
