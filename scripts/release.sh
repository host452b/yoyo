#!/usr/bin/env bash
#
# scripts/release.sh — manually publish a yoyo release.
#
# Usage:
#   ./scripts/release.sh v2.3.0
#
# What it does:
#   1. Validates the tag format and the working state.
#   2. Cross-compiles yoyo for linux/darwin × amd64/arm64.
#   3. Writes SHA-256 checksums of all four binaries to checksums.txt.
#   4. Extracts the matching section from CHANGELOG.md as release notes.
#   5. Publishes a GitHub release (via `gh`) with the four binaries,
#      checksums.txt, and install.sh attached.
#
# Requirements: go, gh (authenticated), awk, shasum (or sha256sum).
#
# The tag must already exist locally AND on origin. Create it first:
#   git tag -a v2.3.0 -m "..." && git push origin v2.3.0
#
# Then run this script from the repo root.

set -euo pipefail

# ---- args + validation ------------------------------------------------------

if [[ $# -ne 1 ]]; then
  echo "usage: $0 <tag>   (e.g. v2.3.0)" >&2
  exit 1
fi

TAG="$1"
if [[ ! "$TAG" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "error: tag must match vMAJOR.MINOR.PATCH (got: $TAG)" >&2
  exit 1
fi
VERSION="${TAG#v}"

# Must be in the repo root (go.mod present).
if [[ ! -f go.mod ]]; then
  echo "error: run from the repo root (go.mod not found)" >&2
  exit 1
fi

# Repo slug for `gh` (allow override via REPO env var).
REPO="${REPO:-host452b/yoyo}"

# ---- preflight checks -------------------------------------------------------

for cmd in go gh awk; do
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "error: required command not found: $cmd" >&2
    exit 1
  fi
done

# SHA-256 tool varies by OS.
if command -v sha256sum >/dev/null 2>&1; then
  SHA_CMD="sha256sum"
elif command -v shasum >/dev/null 2>&1; then
  SHA_CMD="shasum -a 256"
else
  echo "error: need sha256sum or shasum" >&2
  exit 1
fi

# gh auth.
if ! gh auth status >/dev/null 2>&1; then
  echo "error: gh not authenticated (run: gh auth login)" >&2
  exit 1
fi

# Tag must exist locally.
if ! git rev-parse --verify --quiet "refs/tags/$TAG" >/dev/null; then
  echo "error: local tag $TAG not found. Create it first:" >&2
  echo "   git tag -a $TAG -m \"...\"" >&2
  echo "   git push origin main && git push origin $TAG" >&2
  exit 1
fi

# Tag must exist on origin (best-effort check).
if ! git ls-remote --tags origin "$TAG" 2>/dev/null | grep -q "refs/tags/$TAG"; then
  echo "warning: tag $TAG not pushed to origin — release will still publish but the tag won't be fetchable by users." >&2
  echo "hint: git push origin $TAG" >&2
fi

# Working tree must match the tag exactly.
if ! git diff --quiet "$TAG" HEAD -- ':!scripts' 2>/dev/null; then
  echo "warning: HEAD differs from $TAG (outside ./scripts). Artifacts will be built from current HEAD, not the tag's tree." >&2
  read -r -p "proceed anyway? [y/N] " ans
  [[ "$ans" == "y" || "$ans" == "Y" ]] || exit 1
fi

# ---- build ------------------------------------------------------------------

DIST="$(mktemp -d)"
trap 'rm -rf "$DIST"' EXIT

echo "building into $DIST ..."
for OS in linux darwin; do
  for ARCH in amd64 arm64; do
    OUT="yoyo-${OS}-${ARCH}"
    echo "  → $OUT"
    GOOS=$OS GOARCH=$ARCH CGO_ENABLED=0 go build \
      -trimpath \
      -ldflags "-s -w -X main.version=${TAG}" \
      -o "${DIST}/${OUT}" \
      ./cmd/yoyo
  done
done

# ---- checksums --------------------------------------------------------------

(
  cd "$DIST"
  $SHA_CMD yoyo-linux-amd64 yoyo-linux-arm64 yoyo-darwin-amd64 yoyo-darwin-arm64 > checksums.txt
)
echo "---- checksums.txt ----"
cat "${DIST}/checksums.txt"
echo

# ---- release notes ----------------------------------------------------------

NOTES="${DIST}/release-notes.md"
awk -v v="$VERSION" '
  $0 ~ "^## \\["v"\\]" { flag=1; next }
  flag && /^## \[/      { exit }
  flag                   { print }
' CHANGELOG.md > "$NOTES"

if [[ ! -s "$NOTES" ]]; then
  echo "warning: no CHANGELOG section for [$VERSION] — using a placeholder." >&2
  printf 'See [CHANGELOG.md](https://github.com/%s/blob/main/CHANGELOG.md) for details.\n' "$REPO" > "$NOTES"
fi

echo "---- release notes ----"
cat "$NOTES"
echo

# ---- copy install.sh alongside assets --------------------------------------

if [[ ! -f install.sh ]]; then
  echo "error: install.sh not found in repo root" >&2
  exit 1
fi
cp install.sh "$DIST/"

# ---- publish ----------------------------------------------------------------

echo "publishing release $TAG to $REPO ..."

# If a release already exists (e.g. prior attempt), delete it so we can re-upload.
if gh release view "$TAG" --repo "$REPO" >/dev/null 2>&1; then
  echo "existing release $TAG found — deleting before re-publishing ..."
  gh release delete "$TAG" --repo "$REPO" --yes --cleanup-tag=false
fi

gh release create "$TAG" \
  --repo "$REPO" \
  --title "$TAG" \
  --notes-file "$NOTES" \
  "${DIST}/yoyo-linux-amd64" \
  "${DIST}/yoyo-linux-arm64" \
  "${DIST}/yoyo-darwin-amd64" \
  "${DIST}/yoyo-darwin-arm64" \
  "${DIST}/checksums.txt" \
  "${DIST}/install.sh"

echo
echo "✓ released $TAG (GitHub)"
echo "  https://github.com/${REPO}/releases/tag/${TAG}"

# ── optional: PyPI wheels ────────────────────────────────────────────────────
#
# Build per-platform Python wheels for every published Go binary. Default is
# build-only — the wheels are written to python/dist/ and the script prints a
# twine command. Set UPLOAD_PYPI=1 to actually upload.

if command -v python3 >/dev/null 2>&1 && [[ -f python/build_wheels.py ]]; then
  echo
  echo "building PyPI wheels ..."
  python3 python/build_wheels.py "$TAG"

  if [[ "${UPLOAD_PYPI:-0}" == "1" ]]; then
    if ! command -v twine >/dev/null 2>&1; then
      echo "UPLOAD_PYPI=1 set but twine not installed — run: pip install twine" >&2
      exit 1
    fi
    echo
    echo "uploading wheels to PyPI ..."
    twine upload python/dist/yoyo-${TAG#v}-*.whl
    echo "✓ uploaded to PyPI"
    echo "  https://pypi.org/project/yoyo/${TAG#v}/"
  else
    echo
    echo "Wheels built but NOT uploaded. To publish:"
    echo "  UPLOAD_PYPI=1 ./scripts/release.sh $TAG"
    echo "  # or: twine upload python/dist/yoyo-${TAG#v}-*.whl"
  fi
fi
