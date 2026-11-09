import HandbyteCore
import SwiftUI

/// The tier the user reads next to a candidate: safe, review or hold.
struct SafetyBadge: View {
    let safety: Safety

    var body: some View {
        Text(label)
            .font(.caption2.weight(.semibold))
            .padding(.horizontal, 6)
            .padding(.vertical, 2)
            .background(color.opacity(0.18), in: Capsule())
            .foregroundStyle(color)
            .accessibilityLabel("Safety: \(label)")
    }

    private var label: String {
        switch safety {
        case .auto: "Safe"
        case .review: "Review"
        case .caution: "Hold"
        }
    }

    private var color: Color {
        switch safety {
        case .auto: .green
        case .review: .orange
        case .caution: .red
        }
    }
}

/// Shown only when the policy refused the candidate.
struct BlockedBadge: View {
    let reason: String

    var body: some View {
        Label(reason, systemImage: "lock.fill")
            .font(.caption2.weight(.semibold))
            .padding(.horizontal, 6)
            .padding(.vertical, 2)
            .background(Color.secondary.opacity(0.15), in: Capsule())
            .foregroundStyle(.secondary)
            .accessibilityLabel("Blocked: \(reason)")
    }
}

extension Eligibility {
    var blockedReason: String? {
        if case .blocked(let reason) = self { return reason }
        return nil
    }

    var reviewOnlyReason: String? {
        if case .reviewOnly(let reason) = self { return reason }
        return nil
    }
}

extension CategoryState {
    var label: String {
        switch self {
        case .complete: "Complete"
        case .partial: "Partial"
        case .unavailable: "Not present"
        case .cancelled: "Cancelled"
        }
    }

    var systemImage: String {
        switch self {
        case .complete: "checkmark.circle.fill"
        case .partial: "exclamationmark.triangle.fill"
        case .unavailable: "minus.circle"
        case .cancelled: "xmark.circle"
        }
    }

    var color: Color {
        switch self {
        case .complete: .green
        case .partial: .orange
        case .unavailable: .secondary
        case .cancelled: .red
        }
    }
}
