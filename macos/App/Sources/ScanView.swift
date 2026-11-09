import HandbyteCore
import HandbyteUI
import SwiftUI

/// Idle start screen and the live scan.
struct ScanView: View {
    @Environment(AppStore.self) private var store

    var body: some View {
        VStack(spacing: 0) {
            switch store.phase {
            case .scanning(let monitor):
                scanning(monitor)
            default:
                idle
            }
        }
        .padding(24)
    }

    private var idle: some View {
        VStack(spacing: 16) {
            Spacer()
            Image(systemName: "internaldrive")
                .font(.system(size: 56))
                .foregroundStyle(.secondary)
            Text("Handbyte")
                .font(.largeTitle.weight(.semibold))
            Text("Finds caches, build artifacts and other regenerable data under your home folder and shows you what each item is before anything is moved to the Trash.")
                .multilineTextAlignment(.center)
                .foregroundStyle(.secondary)
                .frame(maxWidth: 480)
            Text("Project roots: \(store.configuration.projectRoots.joined(separator: ", "))")
                .font(.caption)
                .foregroundStyle(.tertiary)
            Button("Start Scan", action: store.startScan)
                .keyboardShortcut(.defaultAction)
                .controlSize(.large)
            Spacer()
        }
        .frame(maxWidth: .infinity)
    }

    private func scanning(_ monitor: ScanMonitor) -> some View {
        VStack(alignment: .leading, spacing: 16) {
            HStack {
                ProgressView()
                    .controlSize(.small)
                Text("Scanning…")
                    .font(.title2.weight(.semibold))
                Spacer()
                Button("Cancel", action: store.cancelScan)
                    .keyboardShortcut(.cancelAction)
            }
            HStack(spacing: 24) {
                counter("Candidates", value: "\(monitor.found.count)")
                counter("Measured", value: ByteFormat.string(monitor.bytesSeen))
                counter("Files", value: monitor.filesSeen.formatted())
            }
            Text(monitor.currentPath)
                .font(.caption.monospaced())
                .foregroundStyle(.secondary)
                .lineLimit(1)
                .truncationMode(.middle)
                .frame(maxWidth: .infinity, alignment: .leading)
            List(monitor.rows) { row in
                HStack {
                    Text(row.category.rawValue)
                    Spacer()
                    rowState(row.state)
                }
            }
        }
    }

    private func counter(_ title: String, value: String) -> some View {
        VStack(alignment: .leading) {
            Text(title).font(.caption).foregroundStyle(.secondary)
            Text(value).font(.title3.monospacedDigit())
        }
    }

    @ViewBuilder
    private func rowState(_ state: ScanMonitor.RowState) -> some View {
        switch state {
        case .pending:
            Text("Waiting").foregroundStyle(.secondary)
        case .running:
            HStack(spacing: 6) {
                ProgressView().controlSize(.mini)
                Text("Scanning")
            }
        case .finished(let categoryState):
            Label(categoryState.label, systemImage: categoryState.systemImage)
                .foregroundStyle(categoryState.color)
        }
    }
}
