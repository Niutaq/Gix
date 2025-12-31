package utilities

import (
	"embed"
	"encoding/json"
	"log"
	"strings"
	"sync"
)

//go:embed locales/*.json
var localesFS embed.FS

var translationsMap map[string]map[string]string
var once sync.Once

// InitTranslations loads the translations from the embedded filesystem and initializes the global translations map.
func InitTranslations() {
	once.Do(func() {
		translationsMap = make(map[string]map[string]string)

		langs := []string{
			"en", "pl", "de", "da", "no", "fr", "sw", "cz",
			"hr", "hu", "ua", "bu", "ro", "al", "tr", "ic",
		}

		for _, lang := range langs {
			filename := "locales/" + lang + ".json"
			data, err := localesFS.ReadFile(filename)
			if err != nil {
				if lang == "en" || lang == "pl" {
					log.Printf("Warning: Could not load core language file %s: %v", filename, err)
				}
				continue
			}

			var content map[string]string
			if err := json.Unmarshal(data, &content); err != nil {
				log.Printf("Error parsing JSON for %s: %v", lang, err)
				continue
			}

			translationsMap[strings.ToUpper(lang)] = content
		}
		log.Println("Translations loaded successfully.")
	})
}

// GetTranslation retrieves the translation for a given language and key. Defaults to English or returns the key if unavailable.
func GetTranslation(lang, key string) string {
	if translationsMap == nil {
		InitTranslations()
	}

	lang = strings.ToUpper(lang)

	if langMap, ok := translationsMap[lang]; ok {
		if val, ok := langMap[key]; ok {
			return val
		}
	}

	if lang != "EN" {
		if enMap, ok := translationsMap["EN"]; ok {
			if val, ok := enMap[key]; ok {
				return val
			}
		}
	}

	return key
}
