#!/usr/bin/env bash
# Creates a local release keystore for signed APK/AAB builds (Phase 10.5).
# Output is gitignored (*.keystore). Use strong passwords for production releases.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
KEYSTORE_DIR="$ROOT/.local"
KEYSTORE_FILE="$KEYSTORE_DIR/anpfuel-release.keystore"
KEY_ALIAS="anpfuel"

if [[ -f "$KEYSTORE_FILE" ]]; then
    echo "Keystore already exists: $KEYSTORE_FILE"
    echo "Delete it first if you need a new one."
    exit 0
fi

mkdir -p "$KEYSTORE_DIR"

# Default password for local dev only — override in signing.properties for production.
STORE_PASSWORD="${ANPFUEL_DEV_KEYSTORE_PASSWORD:-anpfuel-dev-keystore}"

keytool -genkeypair \
    -keystore "$KEYSTORE_FILE" \
    -alias "$KEY_ALIAS" \
    -keyalg RSA \
    -keysize 2048 \
    -validity 10000 \
    -storepass "$STORE_PASSWORD" \
    -keypass "$STORE_PASSWORD" \
    -dname "CN=ANP Fuel Prices, OU=Development, O=ANP Fuel Prices OSS, L=Local, ST=NA, C=BR"

echo "Created keystore: $KEYSTORE_FILE"
echo "Alias: $KEY_ALIAS"
echo ""
echo "Next steps:"
echo "  cp signing.properties.example signing.properties"
echo "  # set storePassword/keyPassword to your chosen password"
echo "  ./gradlew validateReleaseBuild"
