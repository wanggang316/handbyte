import HandbyteCore
import HandbyteUI
import SwiftUI

/// Grouped candidate list with a detail pane. Selection for cleanup is the
/// checkbox; the highlighted row only drives the detail pane.
struct ReviewView: View {
    @Environment(AppStore.self) private var store
    @State private var highlighted: Candidate.ID?
    @State private var collapsed: Set<HandbyteCore.Category> = []

    var body: some View {
        @Bindable var store = store
        NavigationSplitView {
            list
                .navigationSplitViewColumnWidth(min: 420, ideal: 520)
                .toolbar { sidebarToolbar }
        } detail: {
            if let candidate = highlightedCandidate {
                CandidateDetailView(candidate: candidate)
            } else {
                ContentUnavailableView("Select an item", systemImage: "doc.text.magnifyingglass",
                                       description: Text("The detail pane shows the path, rule, activity and recovery notes."))
            }
        }
        .searchable(text: $store.reviewList.query, placement: .toolbar, prompt: "Search path or rule")
        .safeAreaInset(edge: .bottom) { footer }
        .sheet(isPresented: $store.isConfirming) {
            ConfirmSheet(candidates: store.selection.selectedCandidates)
        }
    }

    private var highlightedCandidate: Candidate? {
        guard let highlighted else { return nil }
        return store.reviewList.candidates.first { $0.id == highlighted }
    }

    private var list: some View {
        @Bindable var store = store
        return List(selection: $highlighted) {
            ForEach(store.reviewList.groups) { group in
                Section(isExpanded: expansion(group.category)) {
                    ForEach(group.candidates) { candidate in
                        CandidateRow(candidate: candidate, selection: $store.selection)
                            .tag(candidate.id)
                            .contextMenu {
                                Button("Reveal in Finder") { store.reveal(candidate) }
                            }
                    }
                } header: {
                    GroupHeader(group: group, selection: $store.selection)
                }
            }
        }
        .listStyle(.sidebar)
        .onKeyPress(.space) {
            guard let highlighted else { return .ignored }
            store.selection.toggle(highlighted)
            return .handled
        }
        .overlay {
            if store.reviewList.totalCount == 0 {
                ContentUnavailableView("Nothing to clean", systemImage: "checkmark.seal",
                                       description: Text("No candidate reached the minimum size."))
            } else if store.reviewList.visibleCount == 0 {
                ContentUnavailableView.search(text: store.reviewList.query)
            }
        }
    }

    private func expansion(_ category: HandbyteCore.Category) -> Binding<Bool> {
        Binding(
            get: { !collapsed.contains(category) },
            set: { expanded in
                if expanded { collapsed.remove(category) } else { collapsed.insert(category) }
            }
        )
    }

    /// Sort and rescan sit above the list; the detail column keeps the search field.
    @ToolbarContentBuilder
    private var sidebarToolbar: some ToolbarContent {
        ToolbarItemGroup {
            @Bindable var store = store
            Picker("Sort", selection: $store.reviewList.sortOrder) {
                Text("Size").tag(ReviewList.SortOrder.size)
                Text("Name").tag(ReviewList.SortOrder.name)
            }
            .pickerStyle(.segmented)
            .help("Sort candidates by size or name")
            Button("Rescan", systemImage: "arrow.clockwise", action: store.startScan)
                .help("Scan again")
        }
    }

    private var footer: some View {
        HStack {
            Button("Select All Safe") { store.selection.selectAllSafe() }
                .help("Select every eligible candidate whose rule is marked safe")
            Button("Clear") { store.selection.clear() }
                .disabled(store.selection.selectedIDs.isEmpty)
            if case .reviewing(let result) = store.phase, result.cancelled {
                Label("Scan cancelled: the list is incomplete", systemImage: "exclamationmark.triangle.fill")
                    .foregroundStyle(.orange)
            }
            Spacer()
            Text("\(store.selection.selectedIDs.count) selected, \(ByteFormat.string(store.selection.selectedBytes))")
                .monospacedDigit()
                .foregroundStyle(.secondary)
            Button("Move to Trash…", action: store.requestConfirmation)
                .disabled(store.selection.selectedIDs.isEmpty)
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 10)
        .background(.bar)
    }
}

private struct GroupHeader: View {
    let group: CandidateGroup
    @Binding var selection: Selection

    var body: some View {
        HStack {
            Button {
                selection.toggleGroup(group.category)
            } label: {
                Image(systemName: checkbox)
            }
            .buttonStyle(.plain)
            .disabled(!selection.hasBulkSelectable(group.category))
            .accessibilityLabel("Select all safe items in \(group.category.rawValue)")
            .help("Select every safe item in this group; review and hold items are skipped")
            Text(group.category.rawValue)
            Text("(\(group.candidates.count))")
                .foregroundStyle(.secondary)
            Spacer()
            Text(group.partial ? "≥ \(ByteFormat.string(group.totalBytes))" : ByteFormat.string(group.totalBytes))
                .monospacedDigit()
                .foregroundStyle(.secondary)
        }
    }

    private var checkbox: String {
        switch selection.groupState(group.category) {
        case .none: "square"
        case .some: "minus.square"
        case .all: "checkmark.square.fill"
        }
    }
}

private struct CandidateRow: View {
    let candidate: Candidate
    @Binding var selection: Selection

    var body: some View {
        HStack(spacing: 8) {
            Toggle(isOn: Binding(
                get: { selection.isSelected(candidate.id) },
                set: { _ in selection.toggle(candidate.id) }
            )) {
                EmptyView()
            }
            .toggleStyle(.checkbox)
            .labelsHidden()
            .disabled(!selection.canSelect(candidate))
            .accessibilityLabel("Select \(candidate.url.lastPathComponent)")
            VStack(alignment: .leading, spacing: 2) {
                HStack(spacing: 6) {
                    Text(candidate.url.lastPathComponent)
                        .lineLimit(1)
                    SafetyBadge(safety: candidate.rule.safety)
                    if let reason = candidate.eligibility.blockedReason {
                        BlockedBadge(reason: reason)
                    }
                }
                Text(candidate.url.deletingLastPathComponent().fileSystemPath)
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
                    .truncationMode(.middle)
            }
            Spacer()
            Text(ByteFormat.string(candidate.size))
                .monospacedDigit()
                .foregroundStyle(candidate.eligibility.blockedReason == nil ? .primary : .secondary)
        }
        .opacity(candidate.eligibility.blockedReason == nil ? 1 : 0.6)
    }
}

struct CandidateDetailView: View {
    @Environment(AppStore.self) private var store
    let candidate: Candidate

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 18) {
                VStack(alignment: .leading, spacing: 6) {
                    HStack(spacing: 8) {
                        Text(candidate.url.lastPathComponent)
                            .font(.title2.weight(.semibold))
                        SafetyBadge(safety: candidate.rule.safety)
                    }
                    Text(candidate.url.fileSystemPath)
                        .font(.callout.monospaced())
                        .textSelection(.enabled)
                        .foregroundStyle(.secondary)
                    Button("Reveal in Finder", systemImage: "folder") { store.reveal(candidate) }
                }
                if let reason = candidate.eligibility.blockedReason {
                    Label("Blocked: \(reason). This item cannot be selected.", systemImage: "lock.fill")
                        .foregroundStyle(.secondary)
                } else if let reason = candidate.eligibility.reviewOnlyReason {
                    Label("Hold: \(reason). Excluded from bulk selection; select it one by one.", systemImage: "hand.raised.fill")
                        .foregroundStyle(.red)
                }
                Grid(alignment: .leadingFirstTextBaseline, horizontalSpacing: 16, verticalSpacing: 8) {
                    fact("Size", ByteFormat.string(candidate.size)
                        + (candidate.size.partial ? " (lower bound, \(candidate.size.errorCount) unreadable entries)" : ""))
                    fact("Files", candidate.size.fileCount.formatted())
                    fact("Last activity", lastActivity)
                    fact("Rule", candidate.rule.id)
                    fact("Category", candidate.rule.category.rawValue)
                }
                VStack(alignment: .leading, spacing: 4) {
                    Text("What it is").font(.headline)
                    Text(candidate.rule.summary)
                }
                VStack(alignment: .leading, spacing: 4) {
                    Text("After removal").font(.headline)
                    Text(candidate.rule.recovery)
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(24)
        }
    }

    private var lastActivity: String {
        guard let date = candidate.lastActivity else { return "Unknown" }
        let relative = date.formatted(.relative(presentation: .named))
        return "\(relative) (\(date.formatted(date: .abbreviated, time: .shortened)))"
    }

    private func fact(_ label: String, _ value: String) -> some View {
        GridRow {
            Text(label).foregroundStyle(.secondary).gridColumnAlignment(.trailing)
            Text(value).textSelection(.enabled)
        }
    }
}
