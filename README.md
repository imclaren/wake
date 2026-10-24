# wake

A Mac menu bar app that keeps a MacBook awake after its lid is closed, and
turns that on and off with a switch. It is a menu bar version of
[caffeinate+](https://github.com/clashman/caffeinate-plus), a command line
tool by clashman, and it is built on [vero](https://github.com/imclaren/vero).

Turn the switch on and the Mac does not go to sleep when you close the lid,
so that a download, a build or a backup runs on until you turn the switch
off. Turn it off while the lid is closed and the Mac goes to sleep at once.

## Install

Download the disk image or the installer package from
[wake.calmapps.com](https://wake.calmapps.com). The disk image is dragged to
Applications. The package installs into Applications itself. Either way the
app checks its own site for updates, through
[Sparkle](https://sparkle-project.org), and offers each new version when it
is next opened. It needs macOS 13 or newer.

The first time the switch is turned on, the app asks for an administrator's
password, once, and never again. The section below says why.

## Privacy

wake keeps nothing about you. It connects to one address, its own site, to
check for a new version, and to nothing else. The only things it writes are
the rule described below, and a small settings file and a marker file in
its own folder under `~/Library/Application Support`.

## How it works

The setting that stops a closing lid putting a Mac to sleep,
`SleepDisabled`, may be changed only by an administrator. The first time
the switch is turned on, the worker writes a rule for sudo through
osascript's "with administrator privileges", which shows the system's own
password dialog once. The rule, in `/private/etc/sudoers.d/wake-<uid>`,
lets that account run the three `pmset` commands that change the setting
and put the Mac to sleep, and nothing else, without a password. It stays
until the file is removed, so the password is never asked for again, on
that launch or any other. Removing the file undoes it.

Then the worker runs a second copy of itself, the helper, which runs those
commands through `sudo -n`. It disables sleep while the lid is open and
enables it again the moment the lid closes, which leaves the Mac awake,
because it is closing the lid that puts a Mac to sleep. The worker also
holds caffeinate's assertion against idle sleep while the switch is on.

The panel also has "Remember across logins", which keeps where the switch
was left in `settings.json` in the app's folder and turns it on again
when the worker starts, with a notice that leaving both on means the Mac
never sleeps; and "Open at login", which asks macOS itself, through
SMAppService, so that the app shows under Login Items in System Settings.

The helper ends when the file the worker made for it goes away, which is
the switch being turned off, or when the worker has gone. If the lid is
closed when the switch is turned off, it puts the Mac to sleep, as
caffeinate+ does when it is stopped. Quitting the app does not.

## Uninstall

Turn the switch off, quit the app, and move it from Applications to the
bin. Then remove what it wrote, if you want nothing of it left:

```bash
sudo rm /private/etc/sudoers.d/wake-$(id -u)          # the rule for sudo
rm -r ~/Library/Application\ Support/wake              # its settings
```

If "Open at login" was on, the app also appears under Login Items in
System Settings until it is removed from Applications.

## The code

```
cmd/worker/        the worker: the state, the requests, and the helper that runs as root
macos/             the SwiftUI menu bar app, a Swift package, with Sparkle for updates
site/, cmd/server/ the site: the page and the release folder
scripts/release.sh builds a release and puts it on the releases branch
vero-app.toml      describes the app to vero's packaging and tests
```

The worker is a Go program that holds the app's state and does its work.
The Swift app starts it and talks to it in JSON over a pipe, which is how
every vero app is put together. `AGENTS.md` is vero's guide for coding
agents working on the app, and
[vero's recipes](https://github.com/imclaren/vero/blob/main/docs/README.md)
explain the shape in full.

## Build and run

You need a Mac with Apple silicon, Go, Xcode and vero's command, which is
installed with `go install github.com/imclaren/vero/cmd/vero@latest`.

```bash
cd macos && ./build.sh                       # the worker and the app
VERO_PROFILE=dev ./.build/debug/wake         # run it, apart from the copy you use
go test ./... && go vet ./...                # the worker and the site
vero gen --check                             # the Swift binding is current
```

`VERO_PROFILE` gives a development copy settings of its own, so that it
cannot touch the settings of the copy you use.

## Release

```bash
scripts/release.sh --notes "What is new."
scripts/release.sh --dry-run /tmp/wake-dry   # builds without signing or publishing
```

The script builds the installer and the disk image, signs and notarises
them, makes the release folder with the Sparkle appcast, and replaces the
repository's `releases` branch with that folder as a single commit, so
that the repository's history does not grow by every installer.
`scripts/release.sh --help` has the one-time setup.

A release needs a vero checkout beside this repository, a Developer ID
certificate, a notarytool profile, and the signing key folder that vero
makes on the first release. The key signs every update, and
`sparkle_public_key` in `vero-app.toml` is its public half. Neither may
change once a copy is installed, or that copy stops updating, so back the
key folder up. A fork of wake needs a key, a site and a bundle identifier
of its own, or its copies will look to wake.calmapps.com for updates.

## The site

`cmd/server` serves the app's page and the release folder under
`/releases/`, through vero's `kit/site`. It keeps nothing and has no
accounts. It reads three settings from its environment, or from a `.env`
file beside the binary: `PORT`, which is 8011 unless set, `BASE_URL`, the
site's address, and `RELEASES_DIR`, a folder holding the `releases`
branch. To publish a release, clone or pull that branch into
`RELEASES_DIR` and restart the server.

```bash
go run ./cmd/server                          # http://127.0.0.1:8011, serving ./releases
```

## Licence

MIT.
