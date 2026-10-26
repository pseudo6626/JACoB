#!/usr/bin/env python3
from __future__ import annotations

import argparse
import base64
import hashlib
import json
import re
import sys
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path

MAX_HTML_BYTES = 4 * 1024 * 1024
PACKAGE_RE = re.compile(r"^community\.[a-z0-9][a-z0-9-]{0,47}\.[a-f0-9]{6}$")
SEMVER_RE = re.compile(r"^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$")
ALLOWED_ACTIONS = {"publish", "update", "remix"}
ALLOWED_CATEGORIES = {"accessibility", "bgs", "combat", "engineering", "exploration", "fleet-carrier", "hud", "mining", "navigation", "racing", "trading", "utility", "other"}
CAPABILITY_PATTERNS = {
    "clipboard": (r"\bElite\.clipboard\b",),
    "control": (r"\bElite\.(?:input|bindings|macros)\b",),
    "network": (r"\bElite\.(?:net|catalog)\b",),
    "recorder": (r"\bElite\.recorder\b",),
    "overlay": (r"\bElite\.(?:overlay|hud)\b",),
    "video": (r"\bElite\.video\b",),
    "vision": (r"\bElite\.(?:vision|spatial)\b",),
    "intertab": (r"\bElite\.actions\.invoke\b",),
    "storage": (r"\bElite\.storage\b",),
}

class SubmissionError(Exception):
    pass


def load_json(path: Path):
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise SubmissionError(f"could not read JSON: {exc}") from exc


def semver_tuple(value: str):
    m = SEMVER_RE.fullmatch(value or "")
    if not m:
        raise SubmissionError(f"invalid version: {value!r}")
    return tuple(int(x) for x in m.groups())


def detect_capabilities(html: str):
    found = []
    for capability, patterns in CAPABILITY_PATTERNS.items():
        if any(re.search(pattern, html) for pattern in patterns):
            found.append(capability)
    return sorted(found)


def normalized_tags(raw):
    if not isinstance(raw, list):
        return []
    out = []
    seen = set()
    for item in raw:
        tag = re.sub(r"[^a-z0-9-]+", "-", str(item).strip().lower()).strip("-")[:32]
        if tag and tag not in seen:
            out.append(tag)
            seen.add(tag)
        if len(out) >= 12:
            break
    return out


def decode_html(source):
    if not isinstance(source, dict) or source.get("encoding") != "base64":
        raise SubmissionError("source.encoding must be base64")
    encoded = source.get("html", "")
    if not isinstance(encoded, str) or not encoded:
        raise SubmissionError("submission does not contain tab HTML")
    try:
        raw = base64.b64decode(encoded, validate=True)
    except Exception as exc:
        raise SubmissionError("tab HTML is not valid base64") from exc
    if not raw or len(raw) > MAX_HTML_BYTES:
        raise SubmissionError(f"tab HTML must be between 1 byte and {MAX_HTML_BYTES} bytes")
    try:
        html = raw.decode("utf-8")
    except UnicodeDecodeError as exc:
        raise SubmissionError("tab HTML must be UTF-8") from exc
    if "\x00" in html:
        raise SubmissionError("tab HTML contains NUL bytes")
    actual_hash = hashlib.sha256(raw).hexdigest()
    declared_hash = str(source.get("sha256", "")).lower()
    declared_size = source.get("sizeBytes")
    if declared_hash != actual_hash:
        raise SubmissionError("SHA-256 does not match the submitted HTML")
    if declared_size != len(raw):
        raise SubmissionError("sizeBytes does not match the submitted HTML")
    return raw, html, actual_hash


def manifest_path(root: Path, package_id: str):
    return root / "registry" / "tabs" / f"{package_id}.json"


def read_existing_manifest(root: Path, package_id: str):
    path = manifest_path(root, package_id)
    if not path.exists():
        return None
    data = load_json(path)
    if data.get("id") != package_id:
        raise SubmissionError(f"existing manifest {path} has the wrong id")
    return data


def validate_submission(data, actor: str, root: Path):
    if not isinstance(data, dict) or data.get("kind") != "jacob-community-submission" or data.get("schema") != 1:
        raise SubmissionError("not a JACoB community submission v1 file")
    action = str(data.get("action", ""))
    if action not in ALLOWED_ACTIONS:
        raise SubmissionError("action must be publish, update, or remix")

    tab = data.get("tab")
    if not isinstance(tab, dict):
        raise SubmissionError("missing tab metadata")
    package_id = str(tab.get("packageId", "")).strip().lower()
    if not PACKAGE_RE.fullmatch(package_id):
        raise SubmissionError("packageId is invalid")
    name = str(tab.get("name", "")).strip()
    description = str(tab.get("description", "")).strip()
    author = str(tab.get("author", "")).strip()
    version = str(tab.get("version", "")).strip()
    if not name or len(name) > 80:
        raise SubmissionError("tab name must be 1-80 characters")
    if not description or len(description) > 500:
        raise SubmissionError("description must be 1-500 characters")
    if not author or len(author) > 80:
        raise SubmissionError("author/display name must be 1-80 characters")
    version_tuple = semver_tuple(version)
    category = str(tab.get("category", "utility")).strip().lower() or "utility"
    if category not in ALLOWED_CATEGORIES:
        raise SubmissionError(f"unknown category: {category}")
    tags = normalized_tags(tab.get("tags", []))
    notes = str(tab.get("notes", "")).strip()[:2000]
    sdk = int(tab.get("sdk", 0) or 0)
    if sdk < 1 or sdk > 999:
        raise SubmissionError("sdk must be a positive JACoB SDK version")

    raw, html, sha256 = decode_html(data.get("source"))
    capabilities = detect_capabilities(html)
    submitted_caps = sorted(set(str(x) for x in data.get("detectedCapabilities", []) if isinstance(x, str)))
    capability_mismatch = capabilities != submitted_caps

    existing = read_existing_manifest(root, package_id)
    based_on = None
    if action == "publish":
        if existing is not None:
            raise SubmissionError("that package ID is already published; use Publish update")
        if version != "1.0.0":
            raise SubmissionError("new community tabs must start at version 1.0.0")
        maintainers = [actor]
    elif action == "update":
        if existing is None:
            raise SubmissionError("cannot update a package that is not in the registry")
        maintainers = existing.get("maintainers") or []
        if actor.lower() not in {str(x).lower() for x in maintainers}:
            raise SubmissionError(f"GitHub user @{actor} is not a maintainer of {package_id}")
        base_version = str(tab.get("baseVersion", "")).strip()
        current_version = str(existing.get("version", "")).strip()
        if base_version != current_version:
            raise SubmissionError(f"update was based on {base_version or 'an unknown version'}, but registry currently has {current_version}")
        if version_tuple <= semver_tuple(current_version):
            raise SubmissionError("update version must be newer than the published version")
        based_on = existing.get("derivedFrom")
    else:  # remix
        if existing is not None:
            raise SubmissionError("new remix package ID already exists")
        if version != "1.0.0":
            raise SubmissionError("new remixes must start at version 1.0.0")
        origin = tab.get("basedOn")
        if not isinstance(origin, dict):
            raise SubmissionError("remix is missing its upstream package")
        origin_id = str(origin.get("packageId", "")).strip().lower()
        origin_version = str(origin.get("version", "")).strip()
        origin_manifest = read_existing_manifest(root, origin_id)
        if origin_manifest is None:
            raise SubmissionError("remix upstream package is not in the registry")
        if origin_version != str(origin_manifest.get("version", "")):
            raise SubmissionError("remix must identify the current upstream version")
        based_on = {"id": origin_id, "version": origin_version}
        maintainers = [actor]

    return {
        "action": action,
        "packageId": package_id,
        "name": name,
        "description": description,
        "author": author,
        "version": version,
        "category": category,
        "tags": tags,
        "notes": notes,
        "sdk": sdk,
        "capabilities": capabilities,
        "capabilityMismatch": capability_mismatch,
        "maintainers": maintainers,
        "derivedFrom": based_on,
        "raw": raw,
        "sha256": sha256,
        "sizeBytes": len(raw),
    }


def write_registry(root: Path, info, issue_number: int):
    pkg = info["packageId"]
    version = info["version"]
    package_dir = root / "registry" / "packages" / pkg
    package_dir.mkdir(parents=True, exist_ok=True)
    artifact_path = package_dir / f"{version}.html"
    if artifact_path.exists():
        raise SubmissionError(f"artifact already exists for {pkg} {version}")
    artifact_path.write_bytes(info["raw"])

    relative_artifact = artifact_path.relative_to(root).as_posix()
    now = datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")
    manifest = {
        "schema": 1,
        "id": pkg,
        "name": info["name"],
        "description": info["description"],
        "author": info["author"],
        "maintainers": info["maintainers"],
        "version": version,
        "jacobSdk": f">={info['sdk']}",
        "capabilities": info["capabilities"],
        "category": info["category"],
        "tags": info["tags"],
        "artifact": {
            "path": relative_artifact,
            "sha256": info["sha256"],
            "sizeBytes": info["sizeBytes"],
        },
        "updatedAt": now,
        "sourceIssue": issue_number,
    }
    if info.get("derivedFrom"):
        manifest["derivedFrom"] = info["derivedFrom"]
    path = manifest_path(root, pkg)
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(manifest, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    rebuild_index(root)
    return manifest


def rebuild_index(root: Path):
    tabs_dir = root / "registry" / "tabs"
    tabs_dir.mkdir(parents=True, exist_ok=True)
    entries = []
    for path in sorted(tabs_dir.glob("*.json")):
        entry = load_json(path)
        entries.append(entry)
    index = {"schema": 1, "tabs": entries}
    index_path = root / "registry" / "index.json"
    index_path.parent.mkdir(parents=True, exist_ok=True)
    index_path.write_text(json.dumps(index, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("submission", type=Path)
    parser.add_argument("--actor", required=True)
    parser.add_argument("--issue", required=True, type=int)
    parser.add_argument("--root", type=Path, default=Path("."))
    parser.add_argument("--summary", type=Path)
    args = parser.parse_args()

    try:
        data = load_json(args.submission)
        info = validate_submission(data, args.actor, args.root)
        manifest = write_registry(args.root, info, args.issue)
    except SubmissionError as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        return 2

    summary = {
        "ok": True,
        "action": info["action"],
        "id": info["packageId"],
        "name": info["name"],
        "version": info["version"],
        "capabilities": info["capabilities"],
        "capabilityMismatch": info["capabilityMismatch"],
        "sha256": info["sha256"],
        "sizeBytes": info["sizeBytes"],
        "manifest": manifest,
    }
    text = json.dumps(summary, indent=2)
    print(text)
    if args.summary:
        args.summary.write_text(text + "\n", encoding="utf-8")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
