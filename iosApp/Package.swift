// swift-tools-version: 5.9
import PackageDescription

// AnpFuel iOS shell package (P12-T04). Requires macOS with Xcode 26.4:
// iosArm64/iosSimulatorArm64 compilation, `swift test` and simulator runs
// are BLOCKED until Mac access is available. See iosApp/README.md.
let package = Package(
    name: "AnpFuel",
    defaultLocalization: "en",
    platforms: [.iOS(.v17), .macOS(.v14)],
    products: [
        .library(name: "AnpFuelCore", targets: ["AnpFuelCore"]),
    ],
    targets: [
        .target(name: "AnpFuelCore", resources: [.process("Resources")]),
        .target(
            name: "AnpFuelShell",
            dependencies: ["AnpFuelCore"],
            resources: [.process("Localizable.xcstrings")]
        ),
        .testTarget(name: "AnpFuelCoreTests", dependencies: ["AnpFuelCore"]),
    ]
)
