import Foundation
import Testing
import HandbyteCore
@testable import HandbyteUI

@Suite("Byte format")
struct ByteFormatTests {
    @Test func usesDecimalUnitsLikeFinder() {
        #expect(ByteFormat.string(1_000_000) == "1 MB")
        #expect(ByteFormat.string(52_428_800) == "52.4 MB")
        #expect(ByteFormat.string(0) == "Zero KB")
    }

    @Test func partialSizesReadAsLowerBounds() {
        let exact = SizeReport(allocatedBytes: 1_000_000, fileCount: 1, partial: false, errorCount: 0)
        let partial = SizeReport(allocatedBytes: 1_000_000, fileCount: 1, partial: true, errorCount: 2)
        #expect(ByteFormat.string(exact) == "1 MB")
        #expect(ByteFormat.string(partial) == "≥ 1 MB")
    }
}
