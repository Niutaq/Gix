#!/bin/bash

# --- CONFIGURATION ---
APP_NAME="Gix"
IDENTITY="B40DD3160F80680903E6EC8881DB4CDA3B7BFD9A"
ENTITLEMENTS_FILE="saved_entitlements.plist"
ICON_SOURCE="appicon.png"
# --------------------

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${YELLOW}🚀 Gix iOS Builder (EN)${NC}"

# 1. Find application path in DerivedData
DERIVED_DATA_PATH=$(ls -td ~/Library/Developer/Xcode/DerivedData/Gix-* 2>/dev/null | head -1)

if [ -z "$DERIVED_DATA_PATH" ]; then
    echo -e "${RED}❌ Error: Project not found in DerivedData.${NC}"
    echo "   Run an empty project in Xcode at least once (Play)."
    exit 1
fi

APP_PATH="$DERIVED_DATA_PATH/Build/Products/Debug-iphoneos/$APP_NAME.app"

if [ ! -d "$APP_PATH" ]; then
    echo -e "${RED}❌ Error: .app file not found at: $APP_PATH${NC}"
    echo "   Ensure you selected PHYSICAL DEVICE (iPhone) in Xcode, not a simulator."
    exit 1
fi

echo -e "✅ Found application at: $APP_PATH"

# 2. Entitlements Management
echo -e "${BLUE}🔑 Checking entitlements...${NC}"
codesign -d --entitlements :tmp_entitlements.plist "$APP_PATH/$APP_NAME" 2>/dev/null

if [ -s tmp_entitlements.plist ]; then
    echo -e "✅ Retrieved fresh entitlements from Xcode."
    mv tmp_entitlements.plist "$ENTITLEMENTS_FILE"
elif [ -f "$ENTITLEMENTS_FILE" ]; then
    echo -e "${GREEN}♻️  Using cached entitlements.${NC}"
else
    echo -e "${RED}❌ Error: Missing entitlements. Run Clean and Run in Xcode on an empty project.${NC}"
    exit 1
fi

PLIST_PATH="$APP_PATH/Info.plist"
if [ -f "$PLIST_PATH" ]; then
    # Check if the key already exists, if not - add it
    if ! grep -q "NSLocationWhenInUseUsageDescription" "$PLIST_PATH"; then
        echo -e "${YELLOW}⚠️  Injecting GPS key into Info.plist (better to add it in Xcode!)${NC}"
        plutil -replace NSLocationWhenInUseUsageDescription -string "Gix needs location access to find the nearest currency exchange office." "$PLIST_PATH"
    fi
fi

# Icon (Hack)
cp "$ICON_SOURCE" "$APP_PATH/Icon.png"
plutil -replace CFBundleIconFile -string "Icon.png" "$PLIST_PATH" 2>/dev/null

# 4. Building Go
SDK_PATH=$(xcrun --sdk iphoneos --show-sdk-path)
echo -e "${BLUE}🔨 Building Go code...${NC}"

rm -f "${APP_NAME}_bin"

CGO_ENABLED=1 \
GOOS=ios \
GOARCH=arm64 \
CGO_CFLAGS="-isysroot $SDK_PATH -arch arm64 -miphoneos-version-min=15.0" \
CGO_LDFLAGS="-isysroot $SDK_PATH -arch arm64 -miphoneos-version-min=15.0" \
go build -tags ios -ldflags "-s -w" -o "${APP_NAME}_bin" ./cmd/gix

if [ $? -ne 0 ]; then
    echo -e "${RED}❌ Go compilation error.${NC}"
    exit 1
fi

echo -e "${BLUE}💉 Replacing executable file...${NC}"
cp "${APP_NAME}_bin" "$APP_PATH/$APP_NAME"

# 6. Signing
echo -e "${BLUE}✍️  Digital signing...${NC}"
codesign -f -s "$IDENTITY" --entitlements "$ENTITLEMENTS_FILE" "$APP_PATH"

if [ $? -eq 0 ]; then
    echo -e "${GREEN}✅ SUCCESS!${NC}"
    echo -e ">>> Now in Xcode press: ${YELLOW}Ctrl + Cmd + R${NC} (Run Without Building)"
else
    echo -e "${RED}❌ Signing error.${NC}"
    exit 1
fi

rm "${APP_NAME}_bin"
