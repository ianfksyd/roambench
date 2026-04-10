#!/usr/bin/env python3

import re
import subprocess
import sys
from collections import OrderedDict
from dataclasses import dataclass
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parent.parent
MAX_BLOB_SIZE = 2_000_000


@dataclass(frozen=True)
class Pattern:
    name: str
    regex: re.Pattern[bytes]


PATTERNS = [
    Pattern("private_key", re.compile(rb"-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----")),
    Pattern("aws_access_key", re.compile(rb"\b(?:AKIA|ASIA)[0-9A-Z]{16}\b")),
    Pattern(
        "aws_secret_key",
        re.compile(rb"aws_secret_access_key\s*[=:]\s*[\"']?[A-Za-z0-9/+=]{40}"),
    ),
    Pattern("github_pat", re.compile(rb"github_pat_[A-Za-z0-9_]{20,}")),
    Pattern("github_token", re.compile(rb"\bgh[pousr]_[A-Za-z0-9]{20,}\b")),
    Pattern("slack_token", re.compile(rb"\bxox[baprs]-[A-Za-z0-9-]{10,}\b")),
    Pattern("openai_key", re.compile(rb"\bsk-[A-Za-z0-9]{20,}\b")),
    Pattern("google_api_key", re.compile(rb"\bAIza[0-9A-Za-z\-_]{35}\b")),
    Pattern("stripe_live", re.compile(rb"\b(?:sk|rk)_live_[0-9A-Za-z]{16,}\b")),
    Pattern(
        "jwt",
        re.compile(rb"\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b"),
    ),
    Pattern("bearer_token", re.compile(rb"Bearer\s+[A-Za-z0-9._\-=]{24,}")),
    Pattern("cookie_header", re.compile(rb"Cookie:\s*[^\n]{20,}")),
    Pattern(
        "session_assignment",
        re.compile(
            rb"\b(?:session|csrf|token)\s*[=:]\s*[\"'][A-Za-z0-9._\-=%]{20,}[\"']"
        ),
    ),
    Pattern(
        "bcrypt_hash",
        re.compile(rb'password_hash\s*=\s*"\$2[aby]\$[0-9]{2}\$[./A-Za-z0-9]{53}"'),
    ),
]


def run_git(*args: str, input_text: str | None = None) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        ["git", *args],
        cwd=REPO_ROOT,
        input=input_text,
        text=True,
        capture_output=True,
        check=True,
    )


def iter_reachable_blobs() -> list[tuple[str, int, str]]:
    rev_list = run_git("rev-list", "--objects", "--all").stdout
    objects: OrderedDict[str, str] = OrderedDict()
    for line in rev_list.splitlines():
        if not line.strip():
            continue
        parts = line.split(" ", 1)
        sha = parts[0]
        path = parts[1] if len(parts) > 1 else ""
        objects.setdefault(sha, path)

    batch_input = "".join(sha + "\n" for sha in objects)
    batch = run_git(
        "cat-file",
        "--batch-check=%(objectname) %(objecttype) %(objectsize)",
        input_text=batch_input,
    ).stdout

    blobs: list[tuple[str, int, str]] = []
    for line in batch.splitlines():
        sha, object_type, size_text = line.split()
        if object_type != "blob":
            continue
        size = int(size_text)
        if size > MAX_BLOB_SIZE:
            continue
        blobs.append((sha, size, objects.get(sha, "")))
    return blobs


def scan_blobs(blobs: list[tuple[str, int, str]]) -> tuple[int, list[tuple[str, str, str, str]]]:
    findings: list[tuple[str, str, str, str]] = []
    text_blob_count = 0

    proc = subprocess.Popen(
        ["git", "cat-file", "--batch"],
        cwd=REPO_ROOT,
        stdin=subprocess.PIPE,
        stdout=subprocess.PIPE,
    )
    assert proc.stdin is not None
    assert proc.stdout is not None

    for sha, _, _ in blobs:
        proc.stdin.write((sha + "\n").encode("utf-8"))
    proc.stdin.close()

    for sha, _, path in blobs:
        header = proc.stdout.readline().decode("utf-8", "replace").strip()
        if not header:
            continue
        _, object_type, size_text = header.split()
        if object_type != "blob":
            continue
        size = int(size_text)
        data = proc.stdout.read(size)
        proc.stdout.read(1)

        if b"\x00" in data:
            continue
        text_blob_count += 1

        for pattern in PATTERNS:
            match = pattern.regex.search(data)
            if not match:
                continue
            sample = match.group(0)[:120].decode("utf-8", "replace")
            findings.append((pattern.name, path, sha[:12], sample))
            break

    proc.wait()
    return text_blob_count, findings


def main() -> int:
    try:
        blobs = iter_reachable_blobs()
        text_blob_count, findings = scan_blobs(blobs)
    except subprocess.CalledProcessError as exc:
        sys.stderr.write(exc.stderr)
        return exc.returncode

    print(f"Scanned {len(blobs)} reachable blobs, {text_blob_count} text blobs.")
    if not findings:
        print("No matches for targeted secret patterns in reachable git history.")
        return 0

    print("Potential secret findings:")
    for name, path, sha, sample in findings:
        print(f"- {name}: {path} [{sha}] -> {sample}")
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
