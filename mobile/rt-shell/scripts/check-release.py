#!/usr/bin/env python3
"""Validate the Android shell's Play release wiring and store metadata."""

from __future__ import annotations

import pathlib
import struct
import sys
import xml.etree.ElementTree as ET

ROOT = pathlib.Path(__file__).resolve().parents[1]
APP = ROOT / "app" / "src" / "main"
RES = APP / "res"
ANDROID = "{http://schemas.android.com/apk/res/android}"

MIPMAP_SIZES = {
    "mdpi": 48,
    "hdpi": 72,
    "xhdpi": 96,
    "xxhdpi": 144,
    "xxxhdpi": 192,
}
ERRORS: list[str] = []


def need(cond: bool, message: str) -> None:
    if not cond:
        ERRORS.append(message)


def png_size(path: pathlib.Path) -> tuple[int, int]:
    with path.open("rb") as fh:
        head = fh.read(24)
    if head[:8] != b"\x89PNG\r\n\x1a\n":
        return 0, 0
    return struct.unpack(">II", head[16:24])


def parse_xml(path: pathlib.Path) -> ET.Element | None:
    if not path.exists():
        return None
    try:
        return ET.parse(path).getroot()
    except ET.ParseError as exc:
        ERRORS.append(f"{path.relative_to(ROOT)}: invalid XML: {exc}")
        return None


def main() -> int:
    manifest = parse_xml(APP / "AndroidManifest.xml")
    need(manifest is not None, "missing app/src/main/AndroidManifest.xml")
    if manifest is not None:
        permissions = {
            node.get(ANDROID + "name")
            for node in manifest.findall("uses-permission")
        }
        need("android.permission.INTERNET" in permissions, "Manifest must keep INTERNET permission")
        app = manifest.find("application")
        need(app is not None, "Manifest missing <application>")
        if app is not None:
            need(app.get(ANDROID + "label") == "@string/app_name", "application label must reference app_name")
            need(app.get(ANDROID + "icon") == "@mipmap/ic_launcher", "application icon must be @mipmap/ic_launcher")
            need(app.get(ANDROID + "roundIcon") == "@mipmap/ic_launcher_round", "application roundIcon must be @mipmap/ic_launcher_round")
            need(app.get(ANDROID + "allowBackup") == "false", "allowBackup must be false")
            need(app.get(ANDROID + "fullBackupContent") == "false", "fullBackupContent must be false")
            need(app.get(ANDROID + "usesCleartextTraffic") == "true", "usesCleartextTraffic must stay true for local endpoints")
            activity = app.find("activity")
            need(activity is not None, "Manifest missing MainActivity")
            if activity is not None:
                need(activity.get(ANDROID + "name") == ".MainActivity", "MainActivity name mismatch")
                need(activity.get(ANDROID + "exported") == "true", "MainActivity must be exported")
                actions = {
                    node.get(ANDROID + "name")
                    for node in activity.findall("intent-filter/action")
                }
                need("android.intent.action.MAIN" in actions, "MainActivity missing MAIN action")

    strings = parse_xml(RES / "values" / "strings.xml")
    need(strings is not None, "missing values/strings.xml")
    if strings is not None:
        values = {node.get("name"): (node.text or "") for node in strings.findall("string")}
        need(values.get("app_name") == "Baize MOBILE-RT", "app_name must be Baize MOBILE-RT")

    colors = parse_xml(RES / "values" / "colors.xml")
    need(colors is not None, "missing values/colors.xml")
    if colors is not None:
        names = {node.get("name") for node in colors.findall("color")}
        need("ic_launcher_background" in names, "colors.xml missing ic_launcher_background")

    theme = parse_xml(RES / "values" / "themes.xml")
    need(theme is not None, "missing values/themes.xml")
    if theme is not None:
        styles = {node.get("name"): node for node in theme.findall("style")}
        style = styles.get("Theme.BaizeMobileRT")
        need(style is not None, "missing Theme.BaizeMobileRT")
        if style is not None:
            items = {node.get("name"): (node.text or "") for node in style.findall("item")}
            need(items.get("android:windowBackground") == "@drawable/launch_background", "theme windowBackground must use launch_background")
    need((RES / "drawable" / "launch_background.xml").exists(), "missing drawable/launch_background.xml")

    for name in ("ic_launcher", "ic_launcher_round"):
        adaptive = parse_xml(RES / "mipmap-anydpi-v26" / f"{name}.xml")
        need(adaptive is not None, f"missing mipmap-anydpi-v26/{name}.xml")
        if adaptive is not None:
            need(adaptive.tag == "adaptive-icon", f"{name} is not an adaptive-icon")
            background = adaptive.find("background")
            foreground = adaptive.find("foreground")
            need(background is not None and background.get(ANDROID + "drawable") == "@color/ic_launcher_background", f"{name} background mismatch")
            need(foreground is not None and foreground.get(ANDROID + "drawable") == "@drawable/ic_launcher_foreground", f"{name} foreground mismatch")

    foreground = RES / "drawable-nodpi" / "ic_launcher_foreground.png"
    need(foreground.exists(), "missing drawable-nodpi/ic_launcher_foreground.png")
    if foreground.exists():
        need(png_size(foreground) == (432, 432), f"adaptive foreground must be 432x432, got {png_size(foreground)}")

    for density, size in MIPMAP_SIZES.items():
        for name in ("ic_launcher", "ic_launcher_round"):
            path = RES / f"mipmap-{density}" / f"{name}.png"
            need(path.exists(), f"missing {path.relative_to(ROOT)}")
            if path.exists():
                got = png_size(path)
                need(got == (size, size), f"{path.relative_to(ROOT)} must be {size}x{size}, got {got}")

    gradle = ROOT / "app" / "build.gradle.kts"
    need(gradle.exists(), "missing app/build.gradle.kts")
    if gradle.exists():
        text = gradle.read_text(encoding="utf-8")
        for needle in (
            'applicationId = "com.rebornace.baize.mobilert"',
            '"1.0.0"',
            "releaseVersionCode",
            "versionName = releaseVersionName",
            "versionCode = releaseVersionCode",
            "targetSdk = 35",
            "keystore.properties",
            "signingConfigs",
            'findProperty("versionName")',
            'findProperty("versionCode")',
        ):
            need(needle in text, f"app/build.gradle.kts missing: {needle}")

    wrapper_props = ROOT / "gradle" / "wrapper" / "gradle-wrapper.properties"
    need((ROOT / "gradlew").exists(), "missing gradlew")
    need((ROOT / "gradlew.bat").exists(), "missing gradlew.bat")
    need((ROOT / "gradle" / "wrapper" / "gradle-wrapper.jar").exists(), "missing gradle-wrapper.jar")
    need(wrapper_props.exists(), "missing gradle-wrapper.properties")
    if wrapper_props.exists():
        need("gradle-8.10.2-bin.zip" in wrapper_props.read_text(encoding="utf-8"), "wrapper must pin gradle 8.10.2")

    keystore = ROOT / "keystore.properties.example"
    need(keystore.exists(), "missing keystore.properties.example")
    if keystore.exists():
        text = keystore.read_text(encoding="utf-8")
        for key in ("storeFile", "storePassword", "keyAlias", "keyPassword"):
            need(f"{key}=" in text, f"keystore.properties.example missing {key}")

    archive = ROOT / "scripts" / "archive-android.ps1"
    need(archive.exists(), "missing scripts/archive-android.ps1")
    if archive.exists():
        text = archive.read_text(encoding="utf-8")
        need("bundleRelease" in text, "archive script must run bundleRelease")
        need("keystore.properties" in text, "archive script must check keystore.properties")

    store = ROOT / "PlayStore" / "README.md"
    need(store.exists(), "missing PlayStore/README.md")
    if store.exists():
        text = store.read_text(encoding="utf-8")
        for needle in ("Data safety", "截图", "keystore", "AAB", "隐私政策"):
            need(needle in text, f"PlayStore/README.md missing: {needle}")

    if ERRORS:
        print("Android release checks failed:")
        for error in ERRORS:
            print(" -", error)
        return 1
    print("Android release checks: ok")
    return 0


if __name__ == "__main__":
    sys.exit(main())
