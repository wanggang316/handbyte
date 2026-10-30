import AppKit
import HandbyteCore
import HandbyteUI
import Observation

/// Progress of a cleanup run: what the cleaner reported so far.
struct CleanRun: Equatable {
    let total: Int
    let action: CleanAction
    var outcomes: [CleanOutcome] = []
    var bytesClaimed: Int64 = 0

    var completed: Int { outcomes.count }
}

/// Drives the scanner and the cleaner and holds what the views show. Safety
/// decisions come from Core: the store applies `EligibilityPolicy` to the
/// scan result and hands the user's selection to `Cleaner` unchanged.
@MainActor
@Observable
final class AppStore {
    enum Phase: Equatable {
        case idle
        case scanning(ScanMonitor)
        case reviewing(ScanResult)
        /// The review view with the confirmation sheet on top.
        case confirming(ScanResult)
        case cleaning(CleanRun)
        case report(CleanReport)
    }

    private(set) var phase: Phase = .idle
    var reviewList = ReviewList(candidates: [])
    var selection = Selection(candidates: [])

    /// Saved on every change. A file that cannot be read starts from defaults
    /// and `settingsError` tells the settings window why. A change to a policy
    /// input relabels the current result, whichever control made it.
    var settings: AppSettings {
        didSet {
            persistSettings()
            if settings.exclusions != oldValue.exclusions || settings.projectRoots != oldValue.projectRoots {
                relabel()
            }
        }
    }
    private(set) var settingsError: String?
    let overriddenFields: Set<LaunchOverrides.Field>

    /// Probed at launch and again at the start of every scan.
    private(set) var fullDiskAccess: FullDiskAccess.Status
    /// The probe result when the current result set was scanned.
    private(set) var scanLackedFullDiskAccess = false

    let home: URL
    let journalURL: URL
    private let settingsStore: SettingsStore
    private let environment: [String: String]

    private let scanner = DiskScanner()
    private let cleaner = Cleaner()
    private var scanTask: Task<Void, Never>?
    private var cleanTask: Task<Void, Never>?

    init(environment: [String: String] = ProcessInfo.processInfo.environment) {
        home = LaunchOverrides.home(environment: environment)
        self.environment = environment
        journalURL = OperationsJournal.defaultURL(home: home)
        settingsStore = SettingsStore(fileURL: SettingsStore.defaultURL(home: home))
        overriddenFields = LaunchOverrides.overridden(environment: environment)
        fullDiskAccess = FullDiskAccess.probe(home: home)
        do {
            settings = try settingsStore.load()
        } catch {
            settings = AppSettings()
            settingsError = "Settings file could not be read, defaults are in use: \(error.localizedDescription)"
        }
    }

    /// AppSettings plus the launch overrides, which win when set.
    var configuration: ScanConfiguration {
        var configuration = settings.scanConfiguration(home: home)
        LaunchOverrides.apply(to: &configuration, environment: environment)
        return configuration
    }

    var deletionMode: AppSettings.DeletionMode { settings.deletionMode }

    var isScanning: Bool {
        if case .scanning = phase { return true }
        return false
    }

    var isCleaning: Bool {
        if case .cleaning = phase { return true }
        return false
    }

    var isConfirming: Bool {
        get {
            if case .confirming = phase { return true }
            return false
        }
        set {
            if !newValue { dismissConfirmation() }
        }
    }

    /// Built from `configuration`, not the settings alone, so overridden
    /// project roots are protected roots as well.
    private var policy: EligibilityPolicy {
        EligibilityPolicy(context: settings.policyContext(for: configuration, runningApps: WorkspaceRunningAppsProbe()))
    }

    private func persistSettings() {
        do {
            try settingsStore.save(settings)
            settingsError = nil
        } catch {
            settingsError = "Settings could not be saved: \(error.localizedDescription)"
        }
    }

    // MARK: Full Disk Access

    func probeFullDiskAccess() {
        fullDiskAccess = FullDiskAccess.probe(home: home)
    }

    func openFullDiskAccessSettings() {
        NSWorkspace.shared.open(FullDiskAccess.settingsURL)
    }

    // MARK: Scan

    func startScan() {
        guard scanTask == nil else { return }
        probeFullDiskAccess()
        scanLackedFullDiskAccess = fullDiskAccess == .denied
        phase = .scanning(ScanMonitor(rules: configuration.rules))

        // Events arrive on scanner threads; the stream keeps their order and
        // the consumer applies them on the main actor. Only `scanTask` is
        // cancelled: `collect` then returns the partial result.
        let (events, continuation) = AsyncStream.makeStream(of: ScanEvent.self)
        let consumer = Task { [weak self] in
            for await event in events {
                self?.applyScanEvent(event)
            }
        }
        let configuration = configuration
        let scanner = scanner
        scanTask = Task { [weak self] in
            let result = await scanner.collect(configuration) { continuation.yield($0) }
            continuation.finish()
            await consumer.value
            self?.finishScan(result)
        }
    }

    func cancelScan() {
        scanTask?.cancel()
    }

    private func applyScanEvent(_ event: ScanEvent) {
        guard case .scanning(var monitor) = phase else { return }
        monitor.apply(event)
        phase = .scanning(monitor)
    }

    /// Labels with the policy as it is now, so an exclusion added during the
    /// scan applies to this result.
    private func finishScan(_ result: ScanResult) {
        scanTask = nil
        let labelled = labelled(result)
        reviewList = ReviewList(candidates: labelled.candidates)
        selection = Selection(candidates: labelled.candidates)
        phase = .reviewing(labelled)
    }

    private func labelled(_ result: ScanResult) -> ScanResult {
        ScanResult(
            candidates: policy.apply(to: result.candidates),
            categories: result.categories,
            bytesSeen: result.bytesSeen,
            filesSeen: result.filesSeen,
            cancelled: result.cancelled
        )
    }

    // MARK: Exclusions

    /// "Never clean this": the path joins the exclusion list and the policy
    /// runs again on the current result, so the row turns blocked at once.
    func exclude(_ candidate: Candidate) {
        settings.exclude(candidate)
    }

    func removeExclusion(_ entry: String) {
        settings.removeExclusion(entry)
    }

    func isExcluded(_ candidate: Candidate) -> Bool {
        settings.excludes(candidate)
    }

    private func relabel() {
        guard case .reviewing(let result) = phase else { return }
        let relabelled = labelled(result)
        var list = ReviewList(candidates: relabelled.candidates)
        list.query = reviewList.query
        list.sortOrder = reviewList.sortOrder
        reviewList = list
        selection = Selection(candidates: relabelled.candidates, keeping: selection)
        phase = .reviewing(relabelled)
    }

    // MARK: Confirm and clean

    func requestConfirmation() {
        guard case .reviewing(let result) = phase, !selection.selectedIDs.isEmpty else { return }
        phase = .confirming(result)
    }

    func dismissConfirmation() {
        guard case .confirming(let result) = phase else { return }
        phase = .reviewing(result)
    }

    /// `acknowledged` is the distinct confirmation for permanent deletion.
    /// Without it, permanent deletion is refused and nothing happens.
    func confirmClean(acknowledged: Bool = false) {
        guard case .confirming = phase, cleanTask == nil,
              let action = CleanConfirmation.action(for: deletionMode, acknowledged: acknowledged)
        else { return }
        let targets = selection.selectedCandidates
        phase = .cleaning(CleanRun(total: targets.count, action: action))

        let (updates, continuation) = AsyncStream.makeStream(of: CleanProgress.self)
        let consumer = Task { [weak self] in
            for await progress in updates {
                self?.applyCleanProgress(progress)
            }
        }
        let cleaner = cleaner
        let policy = policy
        let journal = OperationsJournal(fileURL: journalURL)
        cleanTask = Task { [weak self] in
            let outcomes = await cleaner.clean(targets, action: action, policy: policy, journal: journal) {
                continuation.yield($0)
            }
            continuation.finish()
            await consumer.value
            self?.finishClean(outcomes)
        }
    }

    func cancelClean() {
        cleanTask?.cancel()
    }

    private func applyCleanProgress(_ progress: CleanProgress) {
        guard case .cleaning(var run) = phase else { return }
        run.outcomes.append(progress.latest)
        run.bytesClaimed = progress.bytesClaimed
        phase = .cleaning(run)
    }

    private func finishClean(_ outcomes: [CleanOutcome]) {
        cleanTask = nil
        phase = .report(CleanReport(outcomes: outcomes))
    }

    // MARK: Finder

    func reveal(_ candidate: Candidate) {
        NSWorkspace.shared.activateFileViewerSelecting([candidate.url])
    }

    /// The log exists only after the first applied action; fall back to its folder.
    var canShowJournal: Bool {
        FileManager.default.fileExists(atPath: journalURL.path(percentEncoded: false))
            || FileManager.default.fileExists(atPath: journalURL.deletingLastPathComponent().path(percentEncoded: false))
    }

    func showJournal() {
        if FileManager.default.fileExists(atPath: journalURL.path(percentEncoded: false)) {
            NSWorkspace.shared.activateFileViewerSelecting([journalURL])
        } else {
            NSWorkspace.shared.open(journalURL.deletingLastPathComponent())
        }
    }
}
