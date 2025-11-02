package reading_data

import (
	"embed"
	"fmt"

	"gioui.org/font"
	"gioui.org/font/opentype"
)

//go:embed fonts/*
var fontFiles embed.FS

func LoadAndParseFont(fontPath string) (font.FontFace, error) {
	fontData, err := GetFont(fontPath)
	if err != nil {
		return font.FontFace{}, fmt.Errorf("error reading font %s: %w", fontPath, err)
	}

	parsedFont, err := opentype.Parse(fontData)
	if err != nil {
		return font.FontFace{}, fmt.Errorf("error parsing font %s: %w", fontPath, err)
	}

	return font.FontFace{Font: font.Font{Weight: font.Normal}, Face: parsedFont}, nil
}

func GetFont(path string) ([]byte, error) {
	data, err := fontFiles.ReadFile(path)

	if err != nil {
		return nil, fmt.Errorf("\nFailed to read font file at path: '%s': %w", path, err)
	}

	return data, nil
}
