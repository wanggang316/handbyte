import SwiftUI

struct ContentView: View {
    @Environment(AppStore.self) private var store

    var body: some View {
        // A plain stack, not a safe-area inset: the split view's sidebar would
        // scroll under an inset and the banner would cover the first rows.
        VStack(spacing: 0) {
            FullDiskAccessBanner()
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
