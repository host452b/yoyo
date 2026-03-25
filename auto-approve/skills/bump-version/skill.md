---
description: Bump the project version across all files, update changelog, and optionally tag/push.
user_invocable: true
---

# Bump Version

Bump the project version number across all locations, update the changelog, and optionally create a git tag and push.

## Steps

1. **Determine current version**: Read `Cargo.toml` for the current version.

2. **Determine target version**: If the user specifies a version, use it. Otherwise, increment the patch version (e.g. 0.0.4 -> 0.0.5).

3. **Update version in all files**:
   - `Cargo.toml` — `version = "X.Y.Z"`
   - `scripts/install.sh` — `VERSION="${AAA_VERSION:-X.Y.Z}"`
   - `scripts/install.ps1` — `$Version = if ($env:AAA_VERSION) { $env:AAA_VERSION } else { "X.Y.Z" }`
   - `README.md` — `AAA_VERSION=X.Y.Z`

4. **Update Cargo.lock**: Run `cargo check` to regenerate the lock file with the new version.

5. **Update CHANGELOG.md**: Add a new `## X.Y.Z` section at the top (below the `# Changelog` heading) with bullet points summarizing changes since the last release. Look at `git log` since the last tag to determine what changed.

6. **Commit**: Stage all modified files and commit with message: `Bump version to X.Y.Z and add changelog`

7. **Tag and push** (only if the user asks):
   - Create tag: `git tag X.Y.Z` (no `v` prefix — this project uses bare version tags)
   - Push: `git push && git push origin X.Y.Z`
