import HandbyteCore
import HandbyteUI
import SwiftUI

/// Honest numbers: one bucket per outcome, bytes only for applied actions.
struct ReportView: View {
    @Environment(AppStore.self) private var store
    let report: CleanReport

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            HStack {
                VStack(alignment: .leading, spacing: 4) {
                    Text(report.isPartial ? "Cleanup cancelled" : "Cleanup finished")
                        .font(.title2.weight(.semibold))
                    Text("\(ByteFormat.string(report.bytesClaimed)) claimed by scan for \(report.moved.count + report.removed.count) items")
                        .foregroundStyle(.secondary)
                }
                Spacer()
                Button("Show Operations Log in Finder", systemImage: "doc.text", action: store.showJournal)
                    .disabled(!store.canShowJournal)
                Button("Rescan", systemImage: "arrow.clockwise", action: store.startScan)
                    .keyboardShortcut(.defaultAction)
            }
            HStack(spacing: 20) {
                count("Moved to Trash", report.moved.count, .green)
                count("Removed", report.removed.count, .red)
                count("Previewed", report.previewed.count, .secondary)
                count("Skipped", report.skipped.count, .orange)
                count("Failed", report.failed.count, .red)
                count("Not attempted", report.notAttempted.count, .secondary)
            }
            List {
                bucket("Moved to Trash", report.moved)
                bucket("Removed", report.removed)
                bucket("Previewed", report.previewed)
                bucket("Skipped", report.skipped)
                bucket("Failed", report.failed)
                bucket("Not attempted", report.notAttempted)
            }
        }
        .padding(24)
    }

    private func count(_ title: String, _ value: Int, _ color: Color) -> some View {
        VStack(alignment: .leading) {
            Text(title).font(.caption).foregroundStyle(.secondary)
            Text("\(value)").font(.title3.monospacedDigit()).foregroundStyle(value == 0 ? .secondary : color)
        }
    }

    @ViewBuilder
    private func bucket(_ title: String, _ items: [ReportItem]) -> some View {
        if !items.isEmpty {
            Section("\(title) (\(items.count))") {
                ForEach(items) { OutcomeRow(item: $0) }
            }
        }
    }
}

struct OutcomeRow: View {
    let item: ReportItem

    var body: some View {
        HStack(alignment: .firstTextBaseline) {
            Image(systemName: symbol)
                .foregroundStyle(color)
                .accessibilityLabel(statusLabel)
            VStack(alignment: .leading, spacing: 2) {
                Text(item.candidate.url.fileSystemPath)
                    .font(.callout.monospaced())
                    .lineLimit(1)
                    .truncationMode(.middle)
                if let detail = item.detail {
                    Text("\(statusLabel): \(detail)")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .lineLimit(2)
                }
            }
            Spacer()
            Text(ByteFormat.string(item.candidate.size))
                .monospacedDigit()
                .foregroundStyle(.secondary)
        }
    }

    private var statusLabel: String {
        switch item.outcome.status {
        case .moved: "Moved to Trash"
        case .removed: "Removed"
        case .previewed: "Previewed"
        case .skipped: "Skipped"
        case .failed: "Failed"
        case .notAttempted: "Not attempted"
        }
    }

    private var symbol: String {
        switch item.outcome.status {
        case .moved: "trash.fill"
        case .removed: "xmark.bin.fill"
        case .previewed: "eye"
        case .skipped: "forward.fill"
        case .failed: "exclamationmark.triangle.fill"
        case .notAttempted: "minus.circle"
        }
    }

    private var color: Color {
        switch item.outcome.status {
        case .moved: .green
        case .removed, .failed: .red
        case .previewed, .notAttempted: .secondary
        case .skipped: .orange
        }
    }
}
