# wake for macOS

A SwiftUI app in the menu bar, on the worker in `../cmd/worker`. It is a
Swift package, which `swift build` builds. `scripts/release.sh` makes it an
app with its icon and the worker inside, signs it, and puts it in the
release, where it updates itself with Sparkle.

    ./build.sh                                   builds the worker and the app
    VERO_PROFILE=dev ./.build/debug/wake     runs it
