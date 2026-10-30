import AppKit
import HandbyteCore

/// Answers from the live process list. `NSWorkspace` is not main-actor bound
/// and `NSRunningApplication` is Sendable, so the policy can call this from
/// any task.
struct WorkspaceRunningAppsProbe: RunningAppsProbe {
    func isRunning(bundleID: String) -> Bool {
        !NSRunningApplication.runningApplications(withBundleIdentifier: bundleID).isEmpty
    }
}
