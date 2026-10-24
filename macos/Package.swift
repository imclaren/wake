// swift-tools-version:5.9
import PackageDescription

let package = Package(
    name: "wake",
    platforms: [.macOS(.v13)],
    dependencies: [.package(url: "https://github.com/imclaren/vero", from: "0.18.1")],
    targets: [
        .executableTarget(
            name: "wake",
            dependencies: [
                .product(name: "Vero", package: "vero"),
                // Sparkle, set up for a vero app: the app updates itself
                // from the appcast on the site.
                .product(name: "VeroUpdates", package: "vero"),
            ])
    ]
)
