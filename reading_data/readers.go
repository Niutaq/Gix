package reading_data

import (
	"embed"
	"fmt"
)

//go:embed fonts/*
var font embed.FS

func GetFont(path string) ([]byte, error) {
	data, err := font.ReadFile(path)

	if err != nil {
		return nil, fmt.Errorf("\nFailed to read font file at path: '%s': %w", path, err)
	}

	return data, nil
}
