// swift-tools-version: 6.2
import PackageDescription

let package = Package(
    name: "HandbyteCore",
    platforms: [.macOS(.v14)],
    products: [
        .library(name: "HandbyteCore", targets: ["HandbyteCore"]),
        .executable(name: "hbscan", targets: ["hbscan"]),
    ],
    targets: [
        .target(name: "HandbyteCore"),
        .executableTarget(name: "hbscan", dependencies: ["HandbyteCore"]),
        .testTarget(name: "HandbyteCoreTests", dependencies: ["HandbyteCore"]),
    ],
    swiftLanguageModes: [.v6]
)
