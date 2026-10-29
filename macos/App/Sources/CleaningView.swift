import HandbyteCore
import HandbyteUI
import SwiftUI

struct CleaningView: View {
    @Environment(AppStore.self) private var store
    let run: CleanRun

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            HStack {
                Text("Moving to Trash…")
                    .font(.title2.weight(.semibold))
                Spacer()
                Button("Cancel", action: store.cancelClean)
                    .keyboardShortcut(.cancelAction)
                    .help("Finish the current item, then stop. The rest is reported as not attempted.")
            }
            ProgressView(value: Double(run.completed), total: Double(max(run.total, 1)))
            Text("\(run.completed) of \(run.total) items, \(ByteFormat.string(run.bytesClaimed)) claimed")
                .monospacedDigit()
                .foregroundStyle(.secondary)
            List(run.outcomes, id: \.candidate.id) { outcome in
                OutcomeRow(item: ReportItem(outcome))
            }
        }
        .padding(24)
    }
}
