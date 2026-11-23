#!/usr/bin/env python3
"""Validate the iOS shell's App Store metadata and Xcode wiring."""
from __future__ import annotations

import json
import pathlib
import plistlib
import struct
import sys

ROOT = pathlib.Path(__file__).resolve().parents[1]
APP = ROOT / "BaizeMobileRT"
ERRORS: list[str] = []


def need(cond: bool, message: str) -> None:
    if not cond:
        ERRORS.append(message)


def load_plist(path: pathlib.Path) -> dict:
    with path.open("rb") as fh:
        return plistlib.load(fh)


def png_size(path: pathlib.Path) -> tuple[int, int]:
    with path.open("rb") as fh:
        head = fh.read(24)
    if head[:8] != b"\x89PNG\r\n\x1a\n":
        return 0, 0
    return struct.unpack(">II", head[16:24])


def main() -> int:
    info_path = APP / "Info.plist"
    need(info_path.exists(), "missing BaizeMobileRT/Info.plist")
    if info_path.exists():
        info = load_plist(info_path)
        need(info.get("CFBundleDisplayName") == "Baize MOBILE-RT", "unexpected CFBundleDisplayName")
        need(info.get("CFBundleShortVersionString") == "1.0.0", "CFBundleShortVersionString must be 1.0.0")
        need(info.get("ITSAppUsesNonExemptEncryption") is False, "ITSAppUsesNonExemptEncryption must be false")
        need(
            (info.get("UILaunchScreen") or {}).get("UIColorName") == "LaunchBackground",
            "UILaunchScreen must reference LaunchBackground",
        )
        need(bool(info.get("NSLocalNetworkUsageDescription")), "missing NSLocalNetworkUsageDescription")
        need("UIInterfaceOrientationPortrait" in (info.get("UISupportedInterfaceOrientations") or []), "portrait orientation missing")

    privacy = APP / "PrivacyInfo.xcprivacy"
    need(privacy.exists(), "missing PrivacyInfo.xcprivacy")
    if privacy.exists():
        values = load_plist(privacy)
        need(values.get("NSPrivacyTracking") is False, "privacy manifest must declare NSPrivacyTracking=false")
        need("NSPrivacyCollectedDataTypes" in values, "privacy manifest missing NSPrivacyCollectedDataTypes")
        need("NSPrivacyAccessedAPITypes" in values, "privacy manifest missing NSPrivacyAccessedAPITypes")

    icon_dir = APP / "Assets.xcassets" / "AppIcon.appiconset"
    icon = icon_dir / "AppIcon-1024.png"
    need(icon.exists(), "missing AppIcon-1024.png")
    if icon.exists():
        w, h = png_size(icon)
        need((w, h) == (1024, 1024), f"AppIcon must be 1024x1024, got {w}x{h}")
    need((icon_dir / "Contents.json").exists(), "missing AppIcon Contents.json")
    need((APP / "Assets.xcassets" / "Contents.json").exists(), "missing Assets.xcassets/Contents.json")
    if (icon_dir / "Contents.json").exists():
        contents = json.loads((icon_dir / "Contents.json").read_text(encoding="utf-8-sig"))
        images = contents.get("images") or []
        need(
            any(i.get("filename") == "AppIcon-1024.png" and i.get("size") == "1024x1024" for i in images),
            "AppIcon Contents.json must reference AppIcon-1024.png at 1024x1024",
        )
    need((APP / "Assets.xcassets" / "AccentColor.colorset" / "Contents.json").exists(), "missing AccentColor")
    need((APP / "Assets.xcassets" / "LaunchBackground.colorset" / "Contents.json").exists(), "missing LaunchBackground")

    project = (ROOT / "BaizeMobileRT.xcodeproj" / "project.pbxproj").read_text(encoding="utf-8")
    need("ASSETCATALOG_COMPILER_APPICON_NAME = AppIcon;" in project, "project missing AppIcon setting")
    need("ASSETCATALOG_COMPILER_GLOBAL_ACCENT_COLOR_NAME = AccentColor;" in project, "project missing accent color setting")
    need('TARGETED_DEVICE_FAMILY = "1";' in project, "project must target iPhone only")
    need("Assets.xcassets in Resources" in project, "Assets.xcassets not in Resources")
    need("PrivacyInfo.xcprivacy in Resources" in project, "PrivacyInfo.xcprivacy not in Resources")
    need("MARKETING_VERSION = 1.0.0;" in project, "project MARKETING_VERSION must be 1.0.0")

    need((ROOT / "ExportOptions.plist").exists(), "missing ExportOptions.plist")
    need((ROOT / "scripts" / "archive-ios.sh").exists(), "missing archive-ios.sh")
    need((ROOT / "AppStore" / "README.md").exists(), "missing AppStore/README.md")

    if ERRORS:
        print("iOS release checks failed:")
        for e in ERRORS:
            print(" -", e)
        return 1
    print("iOS release checks: ok")
    return 0


if __name__ == "__main__":
    sys.exit(main())
