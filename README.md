# wake

A Mac menu bar app that keeps a MacBook awake after its lid is closed, and
turns that on and off with a switch. It is a menu bar version of
[caffeinate+](https://github.com/clashman/caffeinate-plus), a command line
tool by clashman.

Turn the switch on and the Mac does not go to sleep when you close the lid,
so that a download, a build or a backup runs on until you turn the switch
off. Turn it off while the lid is closed and the Mac goes to sleep at once.

## Install

Download the disk image or the installer package from
[wake.calmapps.com](https://wake.calmapps.com). 

The first time the switch is turned on, the app asks for an administrator's
password, once, and never again. 

## Build and release

You need a Mac with Apple silicon, Go, Xcode and vero's command, which is
installed with `go install github.com/imclaren/vero/cmd/vero@latest`.

```bash
scripts/release.sh --notes "What is new."
scripts/release.sh --dry-run /tmp/wake-dry   # builds without signing or publishing
```

The script builds the installer and the disk image, signs and notarises
them, makes the release folder with the Sparkle appcast, and replaces the
repository's `releases` branch with that folder as a single commit, so
that the repository's history does not grow as we create every new installer.

A release needs a vero checkout beside this repository, a Developer ID
certificate, a notarytool profile, and the signing key folder that vero
makes on the first release. The key signs every update, and
`sparkle_public_key` in `vero-app.toml` is its public half. Neither may
change once a copy is installed, or that copy stops updating, so back the
key folder up. A fork of wake needs a key, a site and a bundle identifier
of its own, or its copies will look to wake.calmapps.com for updates.

## The site

`cmd/server` serves the app's page and the release folder under
`/releases/`, through vero's `kit/site`. To publish a new release, clone or pull that branch into
`RELEASES_DIR` and restart the server.

```bash
go run ./cmd/server                          # http://127.0.0.1:8011, serving ./releases
```

## Licence

MIT.
