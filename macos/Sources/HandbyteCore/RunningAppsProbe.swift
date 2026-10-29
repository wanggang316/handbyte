import Foundation

/// Answers whether an app is running. The AppKit implementation lives in the
/// app target; Core stays free of AppKit.
public protocol RunningAppsProbe: Sendable {
    func isRunning(bundleID: String) -> Bool
}

/// A fixed answer set, for tests and for previews.
public struct StaticRunningAppsProbe: RunningAppsProbe, Hashable {
    public var running: Set<String>

    public init(running: Set<String> = []) {
        self.running = running
    }

    public func isRunning(bundleID: String) -> Bool {
        running.contains(bundleID)
    }
}
