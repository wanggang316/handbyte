import Foundation
import Testing
import HandbyteCore
@testable import HandbyteUI

@Suite("Selection")
struct SelectionTests {
    let safe = Fixtures.candidate(path: "/Users/alice/Library/Caches/safe", bytes: 100)
    let review = Fixtures.candidate(
        path: "/Users/alice/dev/app/node_modules", rule: Fixtures.rule(id: "artifact:node_modules", category: .projectBuild, safety: .review), bytes: 200
    )
    let hold = Fixtures.candidate(
        path: "/Users/alice/Downloads", rule: Fixtures.rule(id: "downloads", category: .trash, safety: .caution), bytes: 300,
        eligibility: .reviewOnly(reason: "Needs deliberate confirmation")
    )
    let blocked = Fixtures.candidate(
        path: "/Users/alice/Library/Caches/blocked", bytes: 400, eligibility: .blocked(reason: "Symbolic link")
    )

    @Test func toggleSelectsAndDeselectsAnEligibleCandidate() {
        var selection = Selection(candidates: [safe])

        selection.toggle(safe.id)
        #expect(selection.isSelected(safe.id))
        #expect(selection.selectedCandidates == [safe])
        #expect(selection.selectedBytes == 100)

        selection.toggle(safe.id)
        #expect(!selection.isSelected(safe.id))
        #expect(selection.selectedCandidates.isEmpty)
    }

    @Test func blockedCandidatesCannotBeSelected() {
        var selection = Selection(candidates: [blocked])
        #expect(!selection.canSelect(blocked))
        selection.toggle(blocked.id)
        #expect(selection.selectedCandidates.isEmpty)
    }

    @Test func reviewOnlyCandidatesAreSelectableOneByOne() {
        var selection = Selection(candidates: [hold])
        #expect(selection.canSelect(hold))
        selection.toggle(hold.id)
        #expect(selection.selectedCandidates == [hold])
    }

    @Test func selectAllSafeSkipsReviewHoldAndBlocked() {
        var selection = Selection(candidates: [safe, review, hold, blocked])
        selection.selectAllSafe()
        #expect(selection.selectedCandidates == [safe])
    }

    @Test func selectAllSafeKeepsAlreadySelectedReviewItems() {
        var selection = Selection(candidates: [safe, review])
        selection.toggle(review.id)
        selection.selectAllSafe()
        #expect(Set(selection.selectedIDs) == [safe.id, review.id])
    }

    @Test func groupToggleSelectsOnlySafeEligibleMembersThenClearsTheGroup() {
        let otherGroup = Fixtures.candidate(path: "/Users/alice/go", rule: Fixtures.rule(id: "go-build", category: .devCache))
        var selection = Selection(candidates: [safe, blocked, otherGroup, review])
        selection.toggle(otherGroup.id)

        selection.toggleGroup(.systemCache)
        #expect(Set(selection.selectedIDs) == [safe.id, otherGroup.id])
        #expect(selection.groupState(.systemCache) == .all)
        #expect(selection.groupState(.projectBuild) == .none)

        selection.toggleGroup(.systemCache)
        #expect(Set(selection.selectedIDs) == [otherGroup.id])
        #expect(selection.groupState(.systemCache) == .none)
    }

    @Test func groupToggleDeselectsHandPickedMembersWhenEverySafeMemberIsSelected() {
        let safeB = Fixtures.candidate(path: "/Users/alice/Library/Caches/safe-b", bytes: 10)
        let reviewSameGroup = Fixtures.candidate(
            path: "/Users/alice/Library/Caches/review", rule: Fixtures.rule(id: "pnpm-store", safety: .review)
        )
        var selection = Selection(candidates: [safe, safeB, reviewSameGroup])
        selection.toggle(reviewSameGroup.id)
        #expect(selection.groupState(.systemCache) == .some)

        selection.toggleGroup(.systemCache)
        #expect(Set(selection.selectedIDs) == [safe.id, safeB.id, reviewSameGroup.id])
        #expect(selection.groupState(.systemCache) == .all)

        selection.toggleGroup(.systemCache)
        #expect(selection.selectedIDs.isEmpty)
    }

    @Test func groupWithoutSafeMembersHasNoBulkToggle() {
        let selection = Selection(candidates: [review, hold])
        #expect(!selection.hasBulkSelectable(.projectBuild))
        #expect(!selection.hasBulkSelectable(.trash))
        #expect(selection.hasBulkSelectable(.systemCache) == false)
    }

    @Test func selectingAChildDeselectsItsParentAndViceVersa() {
        let parent = Fixtures.candidate(path: "/Users/alice/dev/app/node_modules", rule: review.rule)
        let child = Fixtures.candidate(path: "/Users/alice/dev/app/node_modules/pkg/node_modules", rule: review.rule)
        let sibling = Fixtures.candidate(path: "/Users/alice/dev/app/node_modules-old", rule: review.rule)
        var selection = Selection(candidates: [parent, child, sibling])

        selection.toggle(parent.id)
        selection.toggle(sibling.id)
        #expect(selection.overlapping(child).map(\.id) == [parent.id])
        selection.toggle(child.id)
        #expect(Set(selection.selectedIDs) == [child.id, sibling.id])

        selection.toggle(parent.id)
        #expect(Set(selection.selectedIDs) == [parent.id, sibling.id])
    }

    @Test func bulkSelectionPrefersTheOutermostOfNestedSafeCandidates() {
        let parent = Fixtures.candidate(path: "/Users/alice/Library/Caches/outer", bytes: 500)
        let child = Fixtures.candidate(path: "/Users/alice/Library/Caches/outer/inner", bytes: 50)
        var selection = Selection(candidates: [child, parent])
        selection.toggle(child.id)

        selection.selectAllSafe()
        #expect(selection.selectedCandidates == [parent])
    }

    @Test func clearRemovesEverySelection() {
        var selection = Selection(candidates: [safe, review])
        selection.selectAllSafe()
        selection.toggle(review.id)
        selection.clear()
        #expect(selection.selectedIDs.isEmpty)
        #expect(selection.selectedBytes == 0)
    }

    @Test func selectedCandidatesFollowTheCandidateOrder() {
        var selection = Selection(candidates: [safe, review, hold])
        selection.toggle(hold.id)
        selection.toggle(safe.id)
        #expect(selection.selectedCandidates == [safe, hold])
    }
}
