#!/usr/bin/env python3
"""Build per-platform Python wheels bundling the yoyo Go binary.

Usage
-----
    python python/build_wheels.py v2.2.3

Behaviour
---------
For each supported (GOOS, GOARCH) pair, this script:

1. Cross-compiles the Go binary with the same flags the release uses
   (``-trimpath -ldflags "-s -w -X main.version=<tag>"``).
2. Packs the binary into a wheel with the matching platform tag. The
   wheel contains no Python code — only ``yoyo-<ver>.data/scripts/yoyo``
   (the binary) plus standard ``dist-info/`` metadata.
3. Writes the wheel to ``python/dist/``.

Supported platforms (no Windows — yoyo's Windows support is partial):

- linux/amd64  → manylinux2014_x86_64
- linux/arm64  → manylinux2014_aarch64
- darwin/amd64 → macosx_10_12_x86_64
- darwin/arm64 → macosx_11_0_arm64

Upload (manual, after `scripts/release.sh` finishes the GitHub release):

    twine upload python/dist/*.whl

Requires only Python 3 stdlib (zipfile, hashlib, base64). No external
packaging libraries needed.
"""

from __future__ import annotations

import base64
import hashlib
import io
import os
import subprocess
import sys
import zipfile
from pathlib import Path
from typing import List, Tuple

# ── configuration ───────────────────────────────────────────────────────────

PLATFORMS: List[Tuple[str, str, str]] = [
    # (GOOS, GOARCH, wheel platform tag)
    ("linux",  "amd64", "manylinux2014_x86_64"),
    ("linux",  "arm64", "manylinux2014_aarch64"),
    ("darwin", "amd64", "macosx_10_12_x86_64"),
    ("darwin", "arm64", "macosx_11_0_arm64"),
]

# ── helpers ─────────────────────────────────────────────────────────────────

def hash_for_record(data: bytes) -> str:
    """Return the sha256 hash for a RECORD entry (urlsafe-b64, no padding)."""
    digest = hashlib.sha256(data).digest()
    return "sha256=" + base64.urlsafe_b64encode(digest).rstrip(b"=").decode()


def wheel_metadata(version: str, readme_path: Path) -> str:
    """Return the METADATA contents for every wheel (platform-agnostic)."""
    long_description = readme_path.read_text(encoding="utf-8")
    return (
        "Metadata-Version: 2.1\n"
        "Name: yoyo\n"
        f"Version: {version}\n"
        "Summary: PTY proxy that auto-approves AI agent permission prompts\n"
        "Home-page: https://github.com/host452b/yoyo\n"
        "Author: host452b\n"
        "License: MIT\n"
        "Project-URL: Source, https://github.com/host452b/yoyo\n"
        "Project-URL: Changelog, https://github.com/host452b/yoyo/blob/main/CHANGELOG.md\n"
        "Keywords: cli,pty,proxy,ai,agent,claude,codex,cursor,auto-approve\n"
        "Classifier: Environment :: Console\n"
        "Classifier: Intended Audience :: Developers\n"
        "Classifier: License :: OSI Approved :: MIT License\n"
        "Classifier: Operating System :: POSIX :: Linux\n"
        "Classifier: Operating System :: MacOS :: MacOS X\n"
        "Classifier: Topic :: Utilities\n"
        "Classifier: Topic :: Terminals\n"
        "Requires-Python: >=3.8\n"
        "Description-Content-Type: text/markdown\n"
        "\n"
        f"{long_description}"
    )


def wheel_file_content(platform_tag: str) -> str:
    """Return the WHEEL file contents. Platform-specific."""
    return (
        "Wheel-Version: 1.0\n"
        "Generator: yoyo build_wheels.py\n"
        "Root-Is-Purelib: false\n"
        f"Tag: py3-none-{platform_tag}\n"
    )


def build_go_binary(goos: str, goarch: str, tag: str, out_path: Path,
                    repo_root: Path) -> None:
    """Cross-compile the yoyo binary for the given platform."""
    env = os.environ.copy()
    env["GOOS"] = goos
    env["GOARCH"] = goarch
    env["CGO_ENABLED"] = "0"
    subprocess.run(
        [
            "go", "build",
            "-trimpath",
            "-ldflags", f"-s -w -X main.version={tag}",
            "-o", str(out_path),
            "./cmd/yoyo",
        ],
        cwd=str(repo_root),
        env=env,
        check=True,
    )


def build_wheel(version: str, platform_tag: str, binary_path: Path,
                readme_path: Path, out_dir: Path) -> Path:
    """Pack one binary into a platform-specific wheel."""
    dist = f"yoyo-{version}"
    wheel_name = f"{dist}-py3-none-{platform_tag}.whl"
    out_path = out_dir / wheel_name

    binary_bytes = binary_path.read_bytes()
    metadata_bytes = wheel_metadata(version, readme_path).encode("utf-8")
    wheel_bytes = wheel_file_content(platform_tag).encode("utf-8")

    # Build the list of wheel entries. Each entry is (arcname, bytes, mode).
    # Binaries go in `<dist>.data/scripts/yoyo` so pip places them on PATH.
    # The dist-info/ files are metadata.
    entries: List[Tuple[str, bytes, int]] = [
        (f"{dist}.data/scripts/yoyo",       binary_bytes, 0o755),
        (f"{dist}.dist-info/METADATA",      metadata_bytes, 0o644),
        (f"{dist}.dist-info/WHEEL",         wheel_bytes, 0o644),
    ]

    # RECORD lists every file above with its hash and size.
    record_lines: List[str] = []
    for arcname, data, _mode in entries:
        record_lines.append(f"{arcname},{hash_for_record(data)},{len(data)}")
    # RECORD references itself with empty hash/size — per spec.
    record_lines.append(f"{dist}.dist-info/RECORD,,")
    record_bytes = ("\n".join(record_lines) + "\n").encode("utf-8")
    entries.append((f"{dist}.dist-info/RECORD", record_bytes, 0o644))

    # Write zip. Use STORED for the binary (already compressed-resistant
    # ELF/Mach-O, gzip gains < 1%), DEFLATE for text metadata.
    #
    # external_attr encodes Unix mode in the upper 16 bits. Must include
    # S_IFREG (0o100000) so pip's wheel installer recognises it as a
    # regular file and preserves the executable bit — without this flag
    # the installed binary ends up mode 0644 and fails to execute.
    S_IFREG = 0o100000
    with zipfile.ZipFile(out_path, "w", zipfile.ZIP_DEFLATED) as zf:
        for arcname, data, mode in entries:
            info = zipfile.ZipInfo(arcname)
            info.external_attr = (S_IFREG | mode) << 16
            compress = (zipfile.ZIP_STORED if mode == 0o755
                        else zipfile.ZIP_DEFLATED)
            zf.writestr(info, data, compress_type=compress)

    return out_path


# ── main ────────────────────────────────────────────────────────────────────

def main(argv: List[str]) -> int:
    if len(argv) != 2:
        print("usage: build_wheels.py <tag>   (e.g. v2.2.3)", file=sys.stderr)
        return 1
    tag = argv[1]
    if not tag.startswith("v") or tag.count(".") < 2:
        print(f"error: tag must look like vMAJOR.MINOR.PATCH (got {tag})",
              file=sys.stderr)
        return 1
    version = tag[1:]

    # Paths
    script_path = Path(__file__).resolve()
    python_dir = script_path.parent
    repo_root = python_dir.parent
    out_dir = python_dir / "dist"
    out_dir.mkdir(exist_ok=True)
    readme_path = python_dir / "README.md"
    if not readme_path.is_file():
        print(f"error: {readme_path} not found", file=sys.stderr)
        return 1

    # Clear prior wheels for this version to avoid pile-up.
    for old in out_dir.glob(f"yoyo-{version}-*.whl"):
        old.unlink()

    # For each platform: build binary, pack into wheel.
    tmp = out_dir / "_tmp"
    tmp.mkdir(exist_ok=True)
    built = []
    for goos, goarch, plat_tag in PLATFORMS:
        print(f"[{goos}/{goarch}] building Go binary …")
        binary_out = tmp / f"yoyo-{goos}-{goarch}"
        build_go_binary(goos, goarch, tag, binary_out, repo_root)

        print(f"[{goos}/{goarch}] packing wheel …")
        wheel_path = build_wheel(version, plat_tag, binary_out, readme_path,
                                 out_dir)
        built.append(wheel_path)
        print(f"    → {wheel_path.relative_to(repo_root)}")

    # Clean up temp binaries.
    for f in tmp.iterdir():
        f.unlink()
    tmp.rmdir()

    print()
    print("Done. Built wheels:")
    for path in built:
        print(f"  {path.relative_to(repo_root)}")
    print()
    print("Upload (after inspecting):")
    print("  twine upload python/dist/*.whl")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
