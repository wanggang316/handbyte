import Foundation
import Testing
import HandbyteCore
@testable import HandbyteUI

@Suite("Review list")
struct ReviewListTests {
    let xcode = Fixtures.candidate(
        path: "/Users/alice/Library/Developer/Xcode/DerivedData",
        rule: Fixtures.rule(id: "xcode-derived", category: .xcode, summary: "Xcode build intermediates."), bytes: 900
    )
    let brew = Fixtures.candidate(
        path: "/Users/alice/Library/Caches/Homebrew",
        rule: Fixtures.rule(id: "brew-cache", category: .devCache, summary: "Homebrew download cache."), bytes: 300, partial: true
    )
    let go = Fixtures.candidate(
        path: "/Users/alice/Library/Caches/go-build",
        rule: Fixtures.rule(id: "go-build", category: .devCache, summary: "Go build cache."), bytes: 500
    )

    @Test func groupsFollowCatalogCategoryOrderAndSkipEmptyCategories() {
        let groups = ReviewList(candidates: [brew, xcode, go]).groups
        #expect(groups.map(\.category) == [.devCache, .xcode])
        #expect(groups[0].candidates == [go, brew])
        #expect(groups[0].totalBytes == 800)
        #expect(groups[0].partial)
        #expect(!groups[1].partial)
    }

    @Test func sortsBySizeDescendingOrNameAscending() {
        let alpha = Fixtures.candidate(path: "/Users/alice/Library/Caches/alpha", bytes: 10)
        let zulu = Fixtures.candidate(path: "/Users/alice/Library/Caches/Zulu", bytes: 100)
        var list = ReviewList(candidates: [alpha, zulu])
        #expect(list.groups[0].candidates.map(\.id) == [zulu.id, alpha.id])
        list.sortOrder = .name
        #expect(list.groups[0].candidates.map(\.id) == [alpha.id, zulu.id])
    }

    @Test func searchMatchesPathRuleIDOrSummaryCaseInsensitively() {
        var list = ReviewList(candidates: [brew, xcode, go])
        list.query = "HOMEBREW"
        #expect(list.groups.flatMap(\.candidates) == [brew])
        list.query = "intermediates"
        #expect(list.groups.flatMap(\.candidates) == [xcode])
        list.query = "go-build"
        #expect(list.groups.flatMap(\.candidates) == [go])
        list.query = "   "
        #expect(list.groups.flatMap(\.candidates).count == 3)
    }

    @Test func totalsCoverEveryCandidateNotOnlyTheVisibleOnes() {
        var list = ReviewList(candidates: [brew, xcode, go])
        list.query = "xcode"
        #expect(list.totalBytes == 1700)
        #expect(list.totalCount == 3)
        #expect(list.visibleCount == 1)
    }
}
