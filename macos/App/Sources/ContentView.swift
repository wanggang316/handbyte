import SwiftUI

struct ContentView: View {
    @Environment(AppStore.self) private var store

    var body: some View {
        Group {
            switch store.phase {
            case .idle, .scanning:
                ScanView()
            case .reviewing, .confirming:
                ReviewView()
            case .cleaning(let run):
                CleaningView(run: run)
            case .report(let report):
                ReportView(report: report)
            }
        }
        .frame(minWidth: 900, minHeight: 560)
    }
}
