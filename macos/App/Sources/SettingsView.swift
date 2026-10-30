import AppKit
import HandbyteCore
import HandbyteUI
import SwiftUI

/// The AppSettings window. Every control writes straight into `store.settings`,
/// which persists on change; the next scan picks the values up.
struct SettingsView: View {
    @Environment(AppStore.self) private var store

    var body: some View {
        @Bindable var store = store
        Form {
            if let error = store.settingsError {
                Section {
                    Label(error, systemImage: "exclamationmark.triangle.fill")
                        .foregroundStyle(.orange)
                }
            }
            DeletionSection(settings: $store.settings)
            MinimumSizeSection(settings: $store.settings, overridden: store.overriddenFields.contains(.minimumSize))
            ProjectRootsSection(settings: $store.settings, overridden: store.overriddenFields.contains(.projectRoots))
            ExclusionsSection()
            Section {
                Button("Reset to Defaults") { store.settings = AppSettings() }
            }
        }
        .formStyle(.grouped)
        .frame(width: 560)
        .frame(minHeight: 520)
    }
}

private struct DeletionSection: View {
    @Binding var settings: AppSettings

    var body: some View {
        Section {
            Picker("Deletion", selection: $settings.deletionMode) {
                Text("Move to Trash").tag(AppSettings.DeletionMode.trash)
                Text("Delete permanently").tag(AppSettings.DeletionMode.removePermanently)
            }
            .pickerStyle(.radioGroup)
            .labelsHidden()
            if settings.deletionMode == .removePermanently {
                Label("Items are removed without a Trash copy and cannot be put back. Each cleanup asks for a separate acknowledgement.",
                      systemImage: "exclamationmark.triangle.fill")
                    .foregroundStyle(.red)
                    .font(.callout)
            } else {
                Text("Items move to the Trash with Finder \"Put Back\" information.")
                    .foregroundStyle(.secondary)
                    .font(.callout)
            }
        } header: {
            Text("Deletion")
        }
    }
}

/// Minimum size in the CLI's syntax (`50M`), committed when valid.
private struct MinimumSizeSection: View {
    @Binding var settings: AppSettings
    let overridden: Bool
    @State private var text = ""
    @State private var invalid = false

    var body: some View {
        Section {
            HStack {
                TextField("Minimum size", text: $text, prompt: Text("50M"))
                    .labelsHidden()
                    .textFieldStyle(.roundedBorder)
                    .frame(width: 120)
                    .onSubmit(commit)
                    .onChange(of: text) { _, _ in invalid = false }
                Button("Apply", action: commit)
                    .disabled(ByteCount.parse(text) == nil)
                Spacer()
                Text(ByteFormat.string(settings.minimumSize))
                    .monospacedDigit()
                    .foregroundStyle(.secondary)
            }
            if invalid {
                Text("Use an integer with an optional K, M, G or T suffix, for example 50M.")
                    .font(.callout)
                    .foregroundStyle(.red)
            }
            if overridden {
                OverrideNote(variable: "HANDBYTE_MIN_SIZE")
            }
        } header: {
            Text("Minimum size")
        } footer: {
            Text("Candidates smaller than this are not listed.")
        }
        .onAppear { text = Self.text(for: settings.minimumSize) }
        .onChange(of: settings.minimumSize) { _, size in text = Self.text(for: size) }
    }

    private func commit() {
        guard let size = ByteCount.parse(text) else {
            invalid = true
            return
        }
        settings.minimumSize = size
        text = Self.text(for: size)
    }

    /// The largest power-of-1024 unit that divides the value exactly.
    static func text(for bytes: Int64) -> String {
        let units: [(Int64, String)] = [(1 << 40, "T"), (1 << 30, "G"), (1 << 20, "M"), (1 << 10, "K")]
        for (multiplier, suffix) in units where bytes > 0 && bytes % multiplier == 0 {
            return "\(bytes / multiplier)\(suffix)"
        }
        return String(bytes)
    }
}

private struct ProjectRootsSection: View {
    @Binding var settings: AppSettings
    let overridden: Bool
    @State private var newRoot = ""

    var body: some View {
        Section {
            if settings.projectRoots.isEmpty {
                Text("No project roots. Project build artifacts are not scanned.")
                    .foregroundStyle(.secondary)
            }
            ForEach(settings.projectRoots, id: \.self) { root in
                HStack {
                    Text(root).font(.body.monospaced())
                    Spacer()
                    Button("Remove", systemImage: "minus.circle") {
                        settings.projectRoots.removeAll { $0 == root }
                    }
                    .labelStyle(.iconOnly)
                    .buttonStyle(.borderless)
                    .accessibilityLabel("Remove project root \(root)")
                }
            }
            HStack {
                TextField("Add a project root", text: $newRoot, prompt: Text("~/src"))
                    .textFieldStyle(.roundedBorder)
                    .onSubmit(addTyped)
                Button("Add", action: addTyped)
                    .disabled(newRoot.trimmingCharacters(in: .whitespaces).isEmpty)
                Button("Choose…", action: choose)
            }
            if overridden {
                OverrideNote(variable: "HANDBYTE_PROJECT_ROOTS")
            }
        } header: {
            Text("Project roots")
        } footer: {
            Text("Build directories such as node_modules and target are searched below these folders. `~` stands for your home folder.")
        }
    }

    private func addTyped() {
        add(newRoot)
        newRoot = ""
    }

    private func add(_ root: String) {
        let root = root.trimmingCharacters(in: .whitespaces)
        guard !root.isEmpty, !settings.projectRoots.contains(root) else { return }
        settings.projectRoots.append(root)
    }

    private func choose() {
        let panel = NSOpenPanel()
        panel.canChooseDirectories = true
        panel.canChooseFiles = false
        panel.allowsMultipleSelection = true
        panel.prompt = "Add"
        guard panel.runModal() == .OK else { return }
        for url in panel.urls {
            add(url.standardizedFileURL.path(percentEncoded: false))
        }
    }
}

private struct ExclusionsSection: View {
    @Environment(AppStore.self) private var store
    @State private var newEntry = ""

    var body: some View {
        Section {
            if store.settings.exclusions.isEmpty {
                Text("Nothing is excluded. Use \"Never Clean This\" in the review list or add a rule id or path here.")
                    .foregroundStyle(.secondary)
            }
            ForEach(store.settings.exclusions, id: \.self) { entry in
                HStack {
                    Image(systemName: entry.hasPrefix("/") ? "folder" : "tag")
                        .foregroundStyle(.secondary)
                    Text(entry)
                        .font(.body.monospaced())
                        .lineLimit(1)
                        .truncationMode(.middle)
                    Spacer()
                    Button("Remove", systemImage: "minus.circle") { store.removeExclusion(entry) }
                        .labelStyle(.iconOnly)
                        .buttonStyle(.borderless)
                        .accessibilityLabel("Remove exclusion \(entry)")
                }
            }
            HStack {
                TextField("Add an exclusion", text: $newEntry, prompt: Text("xcode-archives or /Users/you/path"))
                    .textFieldStyle(.roundedBorder)
                    .onSubmit(add)
                Button("Add", action: add)
                    .disabled(newEntry.trimmingCharacters(in: .whitespaces).isEmpty)
            }
        } header: {
            Text("Never clean")
        } footer: {
            Text("A rule id excludes every item the rule finds; an absolute path excludes that directory. Excluded items stay in the list as blocked.")
        }
    }

    private func add() {
        store.settings.addExclusion(newEntry)
        newEntry = ""
    }
}

private struct OverrideNote: View {
    let variable: String

    var body: some View {
        Label("Replaced for this launch by the \(variable) environment variable.", systemImage: "terminal")
            .font(.callout)
            .foregroundStyle(.orange)
    }
}
