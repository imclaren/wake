# wake

A Mac menu bar app that keeps a Mac awake, even with its lid closed, and
turns that on and off with a switch. It is a menu bar version of
[caffeinate+](https://github.com/clashman/caffeinate-plus), a command line
tool by clashman.

The app provides options to keep your Mac awake, in one of two ways:

- **While the lid is open.** The Mac stays awake, and sleeps as usual when
  the lid is closed. This needs no password.
- **Even when the lid is closed.** A MacBook stays awake even after its lid is
  closed, so that a download, a build or a backup continues to run.

## Install

Download the disk image or the installer package from
[wake.calmapps.com](https://wake.calmapps.com), or install it with
[Homebrew](https://brew.sh):

```bash
brew install --cask imclaren/tap/wake
```

The first time the Mac is kept awake with its lid closed, the app explains
why it needs an administrator's password, and then asks for it, once, and
never again.

## Build and run

You need a Mac with Apple silicon, Xcode and Go.

1. Get the code.

   ```bash
   git clone https://github.com/imclaren/wake && cd wake
   ```

2. Build the worker and the app. The script builds the Go worker, builds
   the Swift package, and puts the worker beside the app.

   ```bash
   cd macos && ./build.sh
   ```

3. Run it. `VERO_PROFILE` gives this copy settings of its own, so that it
   cannot touch the settings of a copy you use.

   ```bash
   VERO_PROFILE=dev ./.build/debug/wake
   ```

4. Test the worker and the site.

   ```bash
   cd .. && go test ./... && go vet ./...
   ```

The debug build is a bare executable rather than an app bundle, which is
enough to work on the panel and the switch. 

## Open at login and automated updates (Sparkle) functionality

"Open at login" and "Check
for Updates" need a real bundle. For one, install [vero](https://github.com/imclaren/vero)'s command and run
`vero install`, which builds the bundle, signs it with the Developer ID
certificate in your keychain, or ad hoc without one, and puts it in
Applications.

```bash
go install github.com/imclaren/vero/cmd/vero@latest
vero install
```

## Release

A release is signed, notarised and published by `scripts/release.sh`. It
builds the installer and the disk image, makes the release folder with the
Sparkle appcast, and replaces the repository's `releases` branch with that
folder as a single commit, so that the repository's history does not grow
by every installer.

### Before the first release

1. Clone [vero](https://github.com/imclaren/vero) beside this repository, or set `VERO` to a clone.

   ```bash
   git clone https://github.com/imclaren/vero ../vero
   ```

2. Have a Developer ID Application certificate and a Developer ID
   Installer certificate in your keychain, from your Apple Developer
   account.

3. Store a notarytool profile. It asks for an app-specific password,
   which is made at appleid.apple.com.

   ```bash
   xcrun notarytool store-credentials wake --apple-id you@example.com --team-id TEAMID
   ```

4. Make the signing key. It signs every update, and a lost key means that
   installed copies never update again, so back the folder up.

   ```bash
   go run -C ../vero/cmd/vero-repo . key --dir ~/.config/wake/vero-key --name "Your name" --email you@example.com
   ```

5. If this is a fork, give it a site, a key and a bundle identifier of
   its own, or its copies will look to wake.calmapps.com for updates. In
   `vero-app.toml`, change `id`, `homepage`, `site` and `feed`, and set
   `sparkle_public_key` to the public key that step 4 printed.

6. Tell the script your certificates and profile in
   `~/.config/wake/release.env`, which is outside the repository. Without
   it, the script uses the defaults at the top of its `ONE-TIME SETUP`
   section, which are calmapps's.

   ```bash
   VERO_MAC_IDENTITY="Developer ID Application: Your Name (TEAMID)"
   VERO_MAC_INSTALLER="Developer ID Installer: Your Name (TEAMID)"
   VERO_NOTARY_PROFILE="wake"
   WAKE_MAINTAINER="Your Name <you@example.com>"
   ```

`vero credentials` says what is set up.

### Each release

1. Check that everything builds, with a dry run. It signs ad hoc, sends
   nothing to Apple, and publishes nothing.

   ```bash
   scripts/release.sh --dry-run /tmp/wake-dry
   ```

2. Release. The version is the next patch after the newest published,
   unless one is given, and the notes are what Sparkle shows in the
   update prompt.

   ```bash
   scripts/release.sh --notes "What is new."
   scripts/release.sh 1.2.0 --notes "What is new."
   ```

3. Publish it on the site, as the next section says.

4. Once the site serves it, put the new cask in the Homebrew tap. The
   release folder is a checkout of the `releases` branch.

   ```bash
   git worktree add /tmp/wake-releases origin/releases
   go run -C ../vero/cmd/vero-repo . publish --homebrew --app "$PWD/vero-app.toml" --site /tmp/wake-releases
   git worktree remove /tmp/wake-releases
   ```

`scripts/release.sh --help` prints the full setup and every option.

## Host the site

`cmd/server` serves the app's page and the release folder under
`/releases/`. It keeps nothing, has no accounts and no database, and
listens on localhost only, so a web server in front of it terminates TLS.

1. Build the server, on the host or for it.

   ```bash
   GOOS=linux GOARCH=amd64 go build -o wake-site ./cmd/server
   ```

2. Clone the `releases` branch on the host. The server serves this folder.

   ```bash
   git clone --branch releases --single-branch --depth 1 https://github.com/imclaren/wake /srv/wake/releases
   ```

3. Put a `.env` file beside the binary, or set the same in its
   environment. `BASE_URL` has to be the address that `site` and `feed` in
   `vero-app.toml` point at, since every installed copy looks there for
   updates.

   ```
   PORT=8011
   BASE_URL=https://wake.example.com
   RELEASES_DIR=/srv/wake/releases
   ```

4. Run the binary as a service, with systemd or whatever the host uses,
   and proxy the site's domain to `127.0.0.1:8011` with TLS in front, from
   nginx or Caddy.

5. After each release, bring the clone up to date and restart the
   service. The branch is force-pushed as a single commit each time, so a
   plain pull does not apply.

   ```bash
   git -C /srv/wake/releases fetch --depth 1 origin releases
   git -C /srv/wake/releases reset --hard origin/releases
   ```

To try the site on your own Mac, with the release folder in `./releases`:

```bash
go run ./cmd/server                          # http://127.0.0.1:8011
```

## Licence

MIT.
