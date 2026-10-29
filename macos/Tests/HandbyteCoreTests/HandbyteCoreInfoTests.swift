import Testing
@testable import HandbyteCore

@Test func coreReportsItsVersion() {
    #expect(HandbyteCoreInfo.version == "0.1.0")
}
