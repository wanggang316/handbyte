import Testing
@testable import HandbyteCore

@Test func byteCountParsesTheGoSizeSyntax() {
    #expect(ByteCount.parse("50M") == Int64(52_428_800))
    #expect(ByteCount.parse("1g") == Int64(1_073_741_824))
    #expect(ByteCount.parse("2T") == Int64(2_199_023_255_552))
    #expect(ByteCount.parse("512k") == Int64(524_288))
    #expect(ByteCount.parse("4096") == Int64(4096))
    #expect(ByteCount.parse("0") == Int64(0))
    #expect(ByteCount.parse(" 7M ") == Int64(7_340_032))
}

@Test func byteCountRejectsMalformedSizes() {
    #expect(ByteCount.parse("") == nil)
    #expect(ByteCount.parse("M") == nil)
    #expect(ByteCount.parse("-1") == nil)
    #expect(ByteCount.parse("1.5G") == nil)
    #expect(ByteCount.parse("10X") == nil)
    #expect(ByteCount.parse("99999999999999999999T") == nil)
}
