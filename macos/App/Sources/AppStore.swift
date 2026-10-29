import AppKit
import HandbyteCore
import HandbyteUI
import Observation

/// Progress of a cleanup run: what the cleaner reported so far.
struct CleanRun: Equatable {
    let total: Int
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

    let configuration: ScanConfiguration
    let journalURL: URL

    private let scanner = DiskScanner()
    private let cleaner = Cleaner()
    private var scanTask: Task<Void, Never>?
    private var cleanTask: Task<Void, Never>?

    init(configuration: ScanConfiguration? = nil) {
        var configuration = configuration ?? ScanConfiguration()
        LaunchOverrides.apply(to: &configuration)
        self.configuration = configuration
        journalURL = OperationsJournal.defaultURL(home: configuration.home)
    }

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

    private var policy: EligibilityPolicy {
        EligibilityPolicy(context: PolicyContext(
            home: configuration.home,
            projectRoots: configuration.projectRoots.map { PathTemplate.expand($0, home: configuration.home) },
            exclusions: [],
            runningApps: WorkspaceRunningAppsProbe()
        ))
    }

    // MARK: Scan

    func startScan() {
        guard scanTask == nil else { return }
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
        let policy = policy
        scanTask = Task { [weak self] in
            let result = await scanner.collect(configuration) { continuation.yield($0) }
            continuation.finish()
            await consumer.value
            let labelled = ScanResult(
                candidates: policy.apply(to: result.candidates),
                categories: result.categories,
                bytesSeen: result.bytesSeen,
                filesSeen: result.filesSeen,
                cancelled: result.cancelled
            )
            self?.finishScan(labelled)
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

    private func finishScan(_ result: ScanResult) {
        scanTask = nil
        reviewList = ReviewList(candidates: result.candidates)
        selection = Selection(candidates: result.candidates)
        phase = .reviewing(result)
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

    func confirmClean() {
        guard case .confirming = phase, cleanTask == nil else { return }
        let targets = selection.selectedCandidates
        phase = .cleaning(CleanRun(total: targets.count))

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
            let outcomes = await cleaner.clean(targets, action: .trash, policy: policy, journal: journal) {
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
