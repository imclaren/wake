#!/bin/bash
#
# Builds a wake release for the Mac with vero's packager, and the
# release folder that the site serves at wake.calmapps.com/releases/.
#
#   scripts/release.sh                      the next patch version
#   scripts/release.sh 1.2.3                a version you choose
#   scripts/release.sh --notes "..."        what Sparkle shows in the update prompt
#   scripts/release.sh --notes-file FILE    the same, from a file
#   scripts/release.sh --dry-run DIR        builds into DIR, signed ad hoc and not notarised:
#                                           NOT shippable, and nothing in releases/ changes
#
# It runs vero's package.sh, which builds the installers from
# vero-app.toml, then vero-repo build, which makes the release folder and
# checks it: every signature, every hash, every link, and that the version
# is newer than the one before.
#
# The release folder lives on the repository's releases branch, not on
# main. Each release replaces the branch with a single commit holding the
# newest version, and force-pushes it, so that the repository's history
# does not grow by every installer of every release. The builder clones
# that branch into releases/ when it deploys (RELEASES_BRANCH in its
# service-wake.conf). After this script, redeploy the service on the
# tasks server, with bash bootstrap.sh update, to publish.
#
#
# ONE-TIME SETUP
# ==============
#
# 1. vero beside this repository (../vero), or VERO set to it.
#
# 2. The signing key in ~/.config/wake/vero-key, which vero key made
#    for the first release. It holds the Sparkle key that signs the Mac's
#    updates, and its public half is sparkle_public_key in vero-app.toml.
#    Every release has to be signed with it, or installed copies stop
#    updating. Back the folder up.
#
# 3. ~/.config/wake/release.env, outside the repository, is optional:
#
#        WAKE_MAINTAINER="calmapps <email>"     who the packages name
#        VERO_MAC_IDENTITY, VERO_MAC_INSTALLER      the Developer ID certificates
#        VERO_NOTARY_PROFILE                        the notarytool profile
#        WAKE_KEY                               the signing key's folder
#
# 4. The Developer ID certificates and the notarytool profile; Xcode and Go.
#    vero credentials says what is set up.
#
#
# WHAT IT PRODUCES
# ================
#
# The releases branch, which the server serves at
# wake.calmapps.com/releases/:
#
#   appcast.xml          the Mac's updates, where every installed copy looks
#   macos/               the .dmg Sparkle updates from, and the .pkg the
#                        page offers
#   latest.json          the newest version, which the page shows
#   index.html           the release folder's own install page

set -euo pipefail

VERSION=""
NOTES_TEXT=""
NOTES_FILE=""
DRY_RUN=""
while [ $# -gt 0 ]; do
    case "$1" in
        --notes)      NOTES_TEXT="$2"; shift ;;
        --notes-file) NOTES_FILE="$2"; shift ;;
        --dry-run)    DRY_RUN="$2"; shift ;;
        -h|--help)    awk 'NR>1 && /^#/ { sub(/^# ?/, ""); print; next } NR>1 { exit }' "$0"; exit 0 ;;
        -*)           echo "unknown option: $1" >&2; exit 1 ;;
        *)            VERSION="$1" ;;
    esac
    shift
done

say()  { printf '\n\033[1m==> %s\033[0m\n' "$*"; }
fail() { printf '\033[31merror: %s\033[0m\n' "$*" >&2; exit 1; }

REPO=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
VERO=${VERO:-$(cd "$REPO/../vero" 2>/dev/null && pwd || true)}
[ -x "$VERO/scripts/package.sh" ] || fail "vero is not at ${VERO:-../vero}: set VERO"

[ -f "$HOME/.config/wake/release.env" ] && set -a && . "$HOME/.config/wake/release.env" && set +a
# Who the packages name: the publisher in vero-app.toml with your git email,
# unless WAKE_MAINTAINER="Name <email>" in release.env says otherwise.
PUBLISHER=$(sed -n 's/^publisher = "\([^<"]*\).*/\1/p' "$REPO/vero-app.toml" | sed 's/ *$//')
EMAIL=$(git -C "$REPO" config user.email || true)
[ -n "${WAKE_MAINTAINER:-}" ] || [ -n "$EMAIL" ] || fail "no email for the packages: set git's user.email, or WAKE_MAINTAINER in ~/.config/wake/release.env"
WAKE_MAINTAINER=${WAKE_MAINTAINER:-"$PUBLISHER <$EMAIL>"}

URL="https://wake.calmapps.com/releases"
if [ -n "$DRY_RUN" ]; then
    SITE="$DRY_RUN/releases"
    # Signed by nothing but its own hash, and not sent to Apple.
    unset VERO_MAC_IDENTITY VERO_MAC_INSTALLER VERO_NOTARY_PROFILE
    KEY="${WAKE_KEY:-$DRY_RUN/key}"
    mkdir -p "$SITE"
else
    SITE="$REPO/.releases"
    export VERO_MAC_IDENTITY="${VERO_MAC_IDENTITY:-Developer ID Application: Iain McLaren (TCVS8DN8UA)}"
    export VERO_MAC_INSTALLER="${VERO_MAC_INSTALLER:-Developer ID Installer: Iain McLaren (TCVS8DN8UA)}"
    export VERO_NOTARY_PROFILE="${VERO_NOTARY_PROFILE:-calmdocs}"
    KEY="${WAKE_KEY:-$HOME/.config/wake/vero-key}"
    [ -f "$KEY/key.asc" ] || fail "no signing key in $KEY: see ONE-TIME SETUP in $0"
    # Without one, vero-repo would make a new Sparkle key and sign this
    # release with it, and no copy already installed would accept the update.
    [ -f "$KEY/sparkle.sec" ] || fail "no Sparkle key in $KEY (sparkle.sec): it has to be the one every release was signed with"
fi

# What is published: the releases branch as it is on GitHub. A real release
# builds into a checkout of it, so that vero-repo keeps the version before
# and checks that this one is newer; a dry run copies its latest.json for
# the same check.
git -C "$REPO" fetch --quiet origin "+refs/heads/releases:refs/remotes/origin/releases" 2>/dev/null || true
published() { git -C "$REPO" show "origin/releases:$1" 2>/dev/null; }
if [ -n "$DRY_RUN" ]; then
    published latest.json > "$SITE/latest.json" || rm -f "$SITE/latest.json"
fi

# The version: the one asked for, or one more than the newest published,
# or 0.1.0 before the first release.
if [ -z "$VERSION" ]; then
    CURRENT=$( (published latest.json || true) | sed -n 's/.*"version": *"\([0-9.]*\)".*/\1/p' | head -1)
    if [ -n "$CURRENT" ]; then
        VERSION=$(echo "$CURRENT" | awk -F. '{print $1"."$2"."$3+1}')
    else
        VERSION="0.1.0"
    fi
fi
[[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || fail "a version looks like 1.2.3, not '$VERSION'"
say "wake $VERSION${DRY_RUN:+ (dry run, into $DRY_RUN)}"

# vero-app.toml with who publishes it, which is not in the repository. It
# has to sit beside the real one, since its paths are relative to it.
APP="$REPO/.release.vero-app.toml"
trap 'rm -f "$APP"' EXIT
sed "s|^publisher = .*|publisher = \"$WAKE_MAINTAINER\"|" "$REPO/vero-app.toml" > "$APP"

PACKAGES=$(mktemp -d "${TMPDIR:-/tmp}/wake-release.XXXXXX")
trap 'rm -f "$APP"; rm -rf "$PACKAGES"' EXIT

# A real release builds into .releases, a checkout of the published
# releases branch, or an empty folder before the first.
if [ -z "$DRY_RUN" ]; then
    rm -rf "$SITE"
    git -C "$REPO" worktree prune
    if git -C "$REPO" rev-parse --verify --quiet origin/releases >/dev/null; then
        git -C "$REPO" worktree add --quiet --detach "$SITE" origin/releases
    else
        git -C "$REPO" worktree add --quiet --detach "$SITE"
        git -C "$SITE" rm -rq --cached . && find "$SITE" -mindepth 1 -maxdepth 1 ! -name .git -exec rm -rf {} +
    fi
    trap 'rm -f "$APP"; rm -rf "$PACKAGES"; git -C "$REPO" worktree remove --force "$SITE" 2>/dev/null' EXIT
fi

say "Building the installers"
"$VERO/scripts/package.sh" --app "$APP" --version "$VERSION" --out "$PACKAGES" --key "$KEY"

say "Building and checking the release folder"
[ -n "$DRY_RUN" ] && [ ! -f "$KEY/key.asc" ] && \
    go run -C "$VERO/cmd/vero-repo" . key --dir "$KEY" --name "wake dry run" --email dry-run@example.com
# Only the newest version is kept (--keep 1): the branch is pushed whole
# each time, since its commit has no parent, and everything the folder
# offers points at the newest anyway.
NOTES_ARGS=()
[ -n "$NOTES_TEXT" ] && NOTES_ARGS=(--notes "$NOTES_TEXT")
[ -n "$NOTES_FILE" ] && NOTES_ARGS=(--notes-file "$NOTES_FILE")
go run -C "$VERO/cmd/vero-repo" . build --app "$APP" --packages "$PACKAGES" --key "$KEY" \
    --url "$URL" --out "$SITE" --keep 1 ${NOTES_ARGS[@]+"${NOTES_ARGS[@]}"}

if [ -z "$DRY_RUN" ]; then
    # The new folder as the branch's only commit, so that the version
    # before leaves the repository too.
    say "Publishing the releases branch"
    git -C "$SITE" add -A
    COMMIT=$(git -C "$SITE" commit-tree "$(git -C "$SITE" write-tree)" -m "wake $VERSION")
    # The commit has no parent, so git sends every file, which past its
    # HTTP buffer goes in chunks that GitHub refuses: so a buffer of 1 GiB.
    git -C "$REPO" -c http.postBuffer=1073741824 push --quiet --force origin "$COMMIT:refs/heads/releases"
    say "Done"
    echo "  wake $VERSION is on the releases branch."
    echo "  Now redeploy the service to publish it: bash bootstrap.sh update on the tasks server."
else
    say "Done"
    echo "  $SITE"
fi
