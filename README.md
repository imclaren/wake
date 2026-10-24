# wake

A Mac menu bar app that keeps a MacBook awake after its lid is closed, and
turns that on and off with a switch. It is a menu bar version of
[caffeinate+](https://github.com/clashman/caffeinate-plus), a command line
tool by clashman, built on [vero](https://github.com/imclaren/vero).

```
cmd/worker/        the worker: the state, the two requests, and the helper that runs as root
macos/             the SwiftUI menu bar app, a Swift package, with Sparkle for updates
site/, cmd/server/ wake.calmapps.com: the page and the release folder
scripts/release.sh builds a release and puts it on the releases branch
vero-app.toml      describes the app to vero's packaging and tests
```

## How it works

The setting that stops a closing lid putting a Mac to sleep,
`SleepDisabled`, may be changed only by an administrator. When the switch
is turned on, the worker runs a second copy of itself as root through
osascript's "with administrator privileges", which shows the system's own
password dialog once. That copy, the helper, disables sleep while the lid
is open and enables it again the moment the lid closes, which leaves the
Mac awake, because it is closing the lid that puts a Mac to sleep. The
worker also holds caffeinate's assertion against idle sleep while the
switch is on.

The helper ends when the file the worker made for it goes away, which is
the switch being turned off, or when the worker has gone. If the lid is
closed when the switch is turned off, it puts the Mac to sleep, as
caffeinate+ does when it is stopped. Quitting the app does not.

## Build and run

```bash
cd macos && ./build.sh                       # the worker and the app
VERO_PROFILE=dev ./.build/debug/wake     # run it, apart from the copy you use
go test ./... && go vet ./...                # the worker and the site
vero gen --check                             # the Swift binding is current
```

`AGENTS.md` is vero's guide for coding agents working on the app.

## Release

```bash
scripts/release.sh --notes "What is new."
```

It builds the installer and the disk image, signs and notarises them,
makes the release folder with the Sparkle appcast, and replaces the
repository's `releases` branch with it. `scripts/release.sh --help` has
the one-time setup. The signing key is in `~/.config/wake/vero-key`;
back it up, since a lost key means that installed copies never update
again.

## The site

`cmd/server` is wake.calmapps.com (package `site`): the page, and the
release folder under `/releases/`, served through vero's `kit/site`. It
knows nobody and keeps nothing. `~/src/builder` deploys it on the tasks
server as the service `wake`, which clones the `releases` branch into
`releases/`. After a release, run `bash bootstrap.sh update` there.

```bash
go run ./cmd/server                          # http://127.0.0.1:8011, serving ./releases
```
