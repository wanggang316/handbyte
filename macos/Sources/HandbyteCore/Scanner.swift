import Foundation
import os

/// Turns the rule catalog into measured candidates.
///
/// Named `DiskScanner` because Foundation already exports a `Scanner`.
public struct DiskScanner: Sendable {
    public init() {}

    /// Events as they happen. Ending the iteration, including by cancelling
    /// the consuming task, cancels the scan.
    public func scan(_ configuration: ScanConfiguration) -> AsyncStream<ScanEvent> {
        AsyncStream { continuation in
            let task = Task {
                _ = await scan(configuration) { continuation.yield($0) }
                continuation.finish()
            }
            continuation.onTermination = { _ in task.cancel() }
        }
    }

    /// Runs in the caller's task, so cancelling that task stops the scan:
    /// no new walk starts, walks in flight stop, and their categories are
    /// reported as cancelled in the result and in the `categoryFinished` events.
    public func scan(
        _ configuration: ScanConfiguration,
        onEvent emit: @escaping @Sendable (ScanEvent) -> Void = { _ in }
    ) async -> ScanResult {
        var run = ScanRun(configuration: configuration, emit: emit)
        return await run.execute()
    }
}

/// One directory to measure, after rule expansion and before deduplication.
struct SizingTask: Sendable, Hashable {
    let rule: Rule
    let url: URL
    let identity: FileIdentity
}

extension Rule {
    /// Higher wins when two rules reach the same directory. A preset names an
    /// explicit location, so it beats a pattern match; among presets the deeper
    /// template wins, for example `~/Library/Caches/go-build` over the
    /// `~/Library/Caches` child expansion.
    var specificity: (Int, Int) {
        switch kind {
        case .preset(let template, _):
            return (1, template.split(separator: "/").count)
        case .projectArtifact:
            return (0, 0)
        }
    }

    func isMoreSpecific(than other: Rule) -> Bool {
        specificity > other.specificity
    }
}

/// Running totals shared between concurrent walks.
private struct Totals: Sendable {
    var bytes: Int64 = 0
    var files = 0
}

private struct CategoryProgress {
    var started = false
    /// At least one directory of this category exists and was queued.
    var measured = false
    var remaining = 0
    var partial = false
    var cancelled = false
}

private struct ScanRun {
    let configuration: ScanConfiguration
    let emit: @Sendable (ScanEvent) -> Void
    private let totals = OSAllocatedUnfairLock(initialState: Totals())
    private let hardLinks = HardLinkRegistry()
    private var categories: [Category: CategoryProgress] = [:]
    private var candidates: [Candidate] = []

    init(configuration: ScanConfiguration, emit: @escaping @Sendable (ScanEvent) -> Void) {
        self.configuration = configuration
        self.emit = emit
    }

    private var rules: [Rule] {
        configuration.skipProjectScan
            ? configuration.rules.filter { $0.directoryName == nil }
            : configuration.rules
    }

    mutating func execute() async -> ScanResult {
        for category in Set(rules.map(\.category)) {
            categories[category] = CategoryProgress()
        }

        var tasks = expandPresets()
        tasks += discoverProjectArtifacts()
        tasks = deduplicate(tasks)

        for task in tasks {
            categories[task.rule.category]?.remaining += 1
            categories[task.rule.category]?.measured = true
        }
        for (category, progress) in categories where progress.remaining == 0 {
            finish(category)
        }

        await measure(tasks)

        for (category, progress) in categories where progress.remaining > 0 {
            categories[category]?.cancelled = true
            categories[category]?.remaining = 0
            finish(category)
        }

        let snapshot = totals.withLock { $0 }
        let states = categories.mapValues(\.state)
        let result = ScanResult(
            candidates: candidates.sorted {
                ($0.size.allocatedBytes, $1.url.path) > ($1.size.allocatedBytes, $0.url.path)
            },
            categories: states,
            bytesSeen: snapshot.bytes,
            filesSeen: snapshot.files,
            cancelled: states.values.contains(.cancelled)
        )
        emit(.finished(result))
        return result
    }

    // MARK: Expansion

    private mutating func expandPresets() -> [SizingTask] {
        var tasks: [SizingTask] = []
        for rule in rules {
            guard let location = rule.location(in: configuration.home),
                  let status = FileStatus(lstatPath: location.fileSystemPath),
                  status.isDirectory
            else { continue }
            if rule.expandChildren {
                tasks += expandChildren(of: location, rule: rule)
            } else {
                tasks.append(SizingTask(rule: rule, url: location, identity: status.identity))
            }
        }
        return tasks
    }

    private mutating func expandChildren(of location: URL, rule: Rule) -> [SizingTask] {
        guard let names = try? FileManager.default.contentsOfDirectory(atPath: location.fileSystemPath) else {
            categories[rule.category]?.partial = true
            return []
        }
        return names.sorted().compactMap { name in
            let child = location.appending(path: name, directoryHint: .isDirectory)
            guard let status = FileStatus(lstatPath: child.fileSystemPath), status.isDirectory else { return nil }
            return SizingTask(rule: rule, url: child, identity: status.identity)
        }
    }

    private mutating func discoverProjectArtifacts() -> [SizingTask] {
        let artifactRules = rules.filter { $0.directoryName != nil }
        guard !artifactRules.isEmpty else { return [] }
        let byName = Dictionary(artifactRules.map { ($0.directoryName!, $0) }, uniquingKeysWith: { first, _ in first })
        start(.projectBuild)

        var tasks: [SizingTask] = []
        for template in configuration.projectRoots {
            if Task.isCancelled {
                categories[.projectBuild]?.cancelled = true
                break
            }
            let root = PathTemplate.expand(template, home: configuration.home)
            emitProgress(currentPath: root.fileSystemPath)
            let found = ProjectArtifactFinder.find(
                under: root,
                directoryNames: Set(byName.keys),
                progress: { [emit, totals] path in
                    let snapshot = totals.withLock { $0 }
                    emit(.progress(ScanProgress(currentPath: path, bytesSeen: snapshot.bytes, filesSeen: snapshot.files)))
                }
            )
            if found.errorCount > 0 {
                categories[.projectBuild]?.partial = true
            }
            for match in found.matches {
                guard let rule = byName[match.lastPathComponent],
                      let status = FileStatus(lstatPath: match.fileSystemPath)
                else { continue }
                tasks.append(SizingTask(rule: rule, url: match, identity: status.identity))
            }
            if found.cancelled {
                categories[.projectBuild]?.cancelled = true
                break
            }
        }
        return tasks
    }

    /// Keeps one task per directory identity. Earlier tasks win ties, so the
    /// catalog order decides between rules of equal specificity.
    private func deduplicate(_ tasks: [SizingTask]) -> [SizingTask] {
        var chosen: [FileIdentity: Int] = [:]
        var kept: [SizingTask] = []
        for task in tasks {
            if let index = chosen[task.identity] {
                if task.rule.isMoreSpecific(than: kept[index].rule) {
                    kept[index] = task
                }
            } else {
                chosen[task.identity] = kept.count
                kept.append(task)
            }
        }
        return kept
    }

    // MARK: Sizing

    private mutating func measure(_ tasks: [SizingTask]) async {
        var pending = tasks.makeIterator()
        await withTaskGroup(of: (SizingTask, DiskUsageReport).self) { group in
            func launch(_ group: inout TaskGroup<(SizingTask, DiskUsageReport)>) -> Bool {
                guard !Task.isCancelled, let task = pending.next() else { return false }
                start(task.rule.category)
                emitProgress(currentPath: task.url.fileSystemPath)
                group.addTask { [hardLinks, totals, emit] in
                    let report = await Self.walk(task.url, hardLinks: hardLinks) { path, bytes, files in
                        let snapshot = totals.withLock { $0 }
                        emit(.progress(ScanProgress(
                            currentPath: path, bytesSeen: snapshot.bytes + bytes, filesSeen: snapshot.files + files
                        )))
                    }
                    return (task, report)
                }
                return true
            }

            for _ in 0..<max(1, configuration.workers) where launch(&group) {}
            for await (task, report) in group {
                record(task, report)
                _ = launch(&group)
            }
        }
    }

    /// fts blocks the thread, so the walk runs off the cooperative pool and
    /// learns about cancellation through a flag instead of `Task.isCancelled`.
    private static let walkQueue = DispatchQueue(label: "handbyte.scan.walk", attributes: .concurrent)

    private static func walk(
        _ url: URL,
        hardLinks: HardLinkRegistry,
        progress: @escaping DiskUsage.ProgressHandler
    ) async -> DiskUsageReport {
        let cancelled = OSAllocatedUnfairLock(initialState: false)
        return await withTaskCancellationHandler {
            await withCheckedContinuation { continuation in
                walkQueue.async {
                    continuation.resume(returning: DiskUsage.measure(
                        url,
                        hardLinks: hardLinks,
                        isCancelled: { cancelled.withLock { $0 } },
                        progress: progress
                    ))
                }
            }
        } onCancel: {
            cancelled.withLock { $0 = true }
        }
    }

    private mutating func record(_ task: SizingTask, _ report: DiskUsageReport) {
        totals.withLock {
            $0.bytes += report.allocatedBytes
            $0.files += report.fileCount
        }
        let category = task.rule.category
        if report.cancelled {
            categories[category]?.cancelled = true
        } else {
            if report.partial {
                categories[category]?.partial = true
            }
            if report.allocatedBytes >= configuration.minimumSize {
                let candidate = Candidate(
                    rule: task.rule,
                    url: task.url,
                    identity: task.identity,
                    size: report.sizeReport,
                    lastActivity: report.lastModified,
                    eligibility: .eligible
                )
                candidates.append(candidate)
                emit(.candidateFound(candidate))
            }
        }
        categories[category]?.remaining -= 1
        if categories[category]?.remaining == 0 {
            finish(category)
        }
    }

    // MARK: Events

    private mutating func start(_ category: Category) {
        guard categories[category]?.started == false else { return }
        categories[category]?.started = true
        emit(.categoryStarted(category))
    }

    private mutating func finish(_ category: Category) {
        start(category)
        emit(.categoryFinished(category, categories[category]?.state ?? .unavailable))
    }

    private func emitProgress(currentPath: String) {
        let snapshot = totals.withLock { $0 }
        emit(.progress(ScanProgress(currentPath: currentPath, bytesSeen: snapshot.bytes, filesSeen: snapshot.files)))
    }
}

extension CategoryProgress {
    /// Decided when the last task of the category is recorded.
    var state: CategoryState {
        if cancelled { return .cancelled }
        if partial { return .partial }
        return measured ? .complete : .unavailable
    }
}
