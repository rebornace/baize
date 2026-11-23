#!/usr/bin/env bash
# Build and export the iOS shell for App Store Connect.
# Requires macOS + Xcode + an Apple Developer team with the bundle id registered.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

: "${DEVELOPMENT_TEAM:?set DEVELOPMENT_TEAM to your Apple Team ID}"
BUNDLE_ID="${BUNDLE_ID:-com.rebornace.baize.mobilert}"
VERSION="${MARKETING_VERSION:-1.0.0}"
BUILD_NUMBER="${CURRENT_PROJECT_VERSION:-1}"
SCHEME="BaizeMobileRT"
ARCHIVE_DIR="$ROOT/build"
ARCHIVE_PATH="$ARCHIVE_DIR/$SCHEME.xcarchive"
EXPORT_DIR="$ARCHIVE_DIR/export"
EXPORT_OPTIONS="$ARCHIVE_DIR/ExportOptions.local.plist"

mkdir -p "$ARCHIVE_DIR"
sed "s/__TEAM_ID__/$DEVELOPMENT_TEAM/" "$ROOT/ExportOptions.plist" > "$EXPORT_OPTIONS"

xcodebuild \
  -project "$ROOT/BaizeMobileRT.xcodeproj" \
  -scheme "$SCHEME" \
  -configuration Release \
  -destination "generic/platform=iOS" \
  -archivePath "$ARCHIVE_PATH" \
  DEVELOPMENT_TEAM="$DEVELOPMENT_TEAM" \
  PRODUCT_BUNDLE_IDENTIFIER="$BUNDLE_ID" \
  MARKETING_VERSION="$VERSION" \
  CURRENT_PROJECT_VERSION="$BUILD_NUMBER" \
  clean archive

rm -rf "$EXPORT_DIR"
xcodebuild -exportArchive \
  -archivePath "$ARCHIVE_PATH" \
  -exportPath "$EXPORT_DIR" \
  -exportOptionsPlist "$EXPORT_OPTIONS"

echo "Archive: $ARCHIVE_PATH"
echo "Export:  $EXPORT_DIR"
echo "Next: upload the .ipa with Transporter or: xcrun altool --upload-app -f <ipa> -t ios --apiKey <key> --apiIssuer <issuer>"
