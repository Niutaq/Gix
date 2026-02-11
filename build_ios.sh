#!/bin/bash

# --- KONFIGURACJA ---
APP_NAME="Gix"
IDENTITY="B40DD3160F80680903E6EC8881DB4CDA3B7BFD9A"
ENTITLEMENTS_FILE="saved_entitlements.plist"
ICON_SOURCE="appicon2.png"
# --------------------

# Kolory
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${YELLOW}🚀 Gix iOS Builder 4.0 (PL)${NC}"

# 1. Znajdź ścieżkę do aplikacji w DerivedData
DERIVED_DATA_PATH=$(ls -td ~/Library/Developer/Xcode/DerivedData/Gix-* 2>/dev/null | head -1)

if [ -z "$DERIVED_DATA_PATH" ]; then
    echo -e "${RED}❌ Błąd: Nie znaleziono projektu w DerivedData.${NC}"
    echo "   Uruchom pusty projekt w Xcode chociaż raz (Play)."
    exit 1
fi

APP_PATH="$DERIVED_DATA_PATH/Build/Products/Debug-iphoneos/$APP_NAME.app"

if [ ! -d "$APP_PATH" ]; then
    echo -e "${RED}❌ Błąd: Nie znaleziono pliku .app w: $APP_PATH${NC}"
    echo "   Upewnij się, że w Xcode wybrałeś FIZYCZNE URZĄDZENIE (iPhone), a nie symulator."
    exit 1
fi

echo -e "✅ Znaleziono aplikację w: $APP_PATH"

# 2. Zarządzanie Uprawnieniami
echo -e "${BLUE}🔑 Sprawdzanie uprawnień...${NC}"
codesign -d --entitlements :tmp_entitlements.plist "$APP_PATH/$APP_NAME" 2>/dev/null

if [ -s tmp_entitlements.plist ]; then
    echo -e "✅ Pobrano świeże uprawnienia z Xcode."
    mv tmp_entitlements.plist "$ENTITLEMENTS_FILE"
elif [ -f "$ENTITLEMENTS_FILE" ]; then
    echo -e "${GREEN}♻️  Używam zapisanych uprawnień z cache.${NC}"
else
    echo -e "${RED}❌ Błąd: Brak entitlements. Zrób Clean i Run w Xcode na pustym projekcie.${NC}"
    exit 1
fi

# 3. Logo i GPS (Injection)
# Próbujemy wstrzyknąć Info.plist key, jeśli użytkownik zapomniał w Xcode
PLIST_PATH="$APP_PATH/Info.plist"
if [ -f "$PLIST_PATH" ]; then
    # Sprawdź czy klucz już istnieje, jeśli nie - dodaj
    if ! grep -q "NSLocationWhenInUseUsageDescription" "$PLIST_PATH"; then
        echo -e "${YELLOW}⚠️  Wstrzykuję klucz GPS do Info.plist (lepiej dodaj go w Xcode!)${NC}"
        plutil -replace NSLocationWhenInUseUsageDescription -string "Gix potrzebuje lokalizacji, aby znaleźć najbliższy kantor." "$PLIST_PATH"
    fi
fi

# Ikonka (Hack)
cp "$ICON_SOURCE" "$APP_PATH/Icon.png"
plutil -replace CFBundleIconFile -string "Icon.png" "$PLIST_PATH" 2>/dev/null

# 4. Budowanie Go
SDK_PATH=$(xcrun --sdk iphoneos --show-sdk-path)
echo -e "${BLUE}🔨 Budowanie kodu Go...${NC}"

# Ważne: Usuwamy stary plik binarny przed budowaniem, by uniknąć błędów linkera
rm -f "${APP_NAME}_bin"

CGO_ENABLED=1 \
GOOS=ios \
GOARCH=arm64 \
CGO_CFLAGS="-isysroot $SDK_PATH -arch arm64 -miphoneos-version-min=15.0" \
CGO_LDFLAGS="-isysroot $SDK_PATH -arch arm64 -miphoneos-version-min=15.0" \
go build -tags ios -ldflags "-s -w" -o "${APP_NAME}_bin" ./cmd/gix

if [ $? -ne 0 ]; then
    echo -e "${RED}❌ Błąd kompilacji Go.${NC}"
    exit 1
fi

# 5. Iniekcja
echo -e "${BLUE}💉 Podmienianie pliku wykonywalnego...${NC}"
cp "${APP_NAME}_bin" "$APP_PATH/$APP_NAME"

# 6. Podpisywanie
echo -e "${BLUE}✍️  Podpisywanie cyfrowe...${NC}"
codesign -f -s "$IDENTITY" --entitlements "$ENTITLEMENTS_FILE" "$APP_PATH"

if [ $? -eq 0 ]; then
    echo -e "${GREEN}✅ SUKCES!${NC}"
    echo -e ">>> Teraz w Xcode naciśnij: ${YELLOW}Ctrl + Cmd + R${NC} (Run Without Building)"
else
    echo -e "${RED}❌ Błąd podpisywania.${NC}"
    exit 1
fi

rm "${APP_NAME}_bin"