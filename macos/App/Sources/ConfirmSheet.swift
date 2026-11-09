import HandbyteCore
import HandbyteUI
import SwiftUI

/// The complete list of what is about to be cleaned. Return never confirms:
/// only Cancel has a keyboard shortcut. Permanent deletion shows a red,
/// differently worded sheet and needs the acknowledgement box ticked.
struct ConfirmSheet: View {
    @Environment(AppStore.self) private var store
    let candidates: [Candidate]
    @State private var acknowledged = false

    private var permanent: Bool { store.deletionMode == .removePermanently }

    private var totalBytes: Int64 {
        candidates.reduce(0) { $0 + $1.size.allocatedBytes }
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack(spacing: 8) {
                if permanent {
                    Image(systemName: "exclamationmark.triangle.fill")
                        .foregroundStyle(.red)
                }
                Text(permanent ? "Permanently delete \(candidates.count) items?" : "Move \(candidates.count) items to the Trash?")
                    .font(.title2.weight(.semibold))
                    .foregroundStyle(permanent ? .red : .primary)
            }
            Text(permanent
                ? "These items are deleted without a Trash copy. There is no undo and no Put Back. Each item is checked again right before it is removed; items that changed since the scan are skipped and reported."
                : "Each item is checked again right before it moves. Items that changed since the scan are skipped and reported. You can put items back from the Trash in Finder.")
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
                    Text(permanent ? "Delete permanently" : "Move to Trash")
                        .font(.caption)
                        .foregroundStyle(permanent ? .red : .secondary)
                    Text(ByteFormat.string(candidate.size))
                        .monospacedDigit()
                        .frame(width: 90, alignment: .trailing)
                }
            }
            .frame(minHeight: 240)
            if permanent {
                Toggle("I understand these items cannot be recovered", isOn: $acknowledged)
                    .toggleStyle(.checkbox)
            }
            HStack {
                Text("Total: \(ByteFormat.string(totalBytes))")
                    .monospacedDigit()
                Spacer()
                Button("Cancel", action: store.dismissConfirmation)
                    .keyboardShortcut(.cancelAction)
                if permanent {
                    Button("Delete Permanently", role: .destructive) {
                        store.confirmClean(acknowledged: acknowledged)
                    }
                    .tint(.red)
                    .buttonStyle(.borderedProminent)
                    .disabled(!acknowledged)
                } else {
                    Button("Move to Trash") { store.confirmClean() }
                }
            }
        }
        .padding(20)
        .frame(minWidth: 640, idealWidth: 720, minHeight: 420)
    }
}
