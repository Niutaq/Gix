package utilities

import (
	// Standard libraries
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

//go:embed locales/*.json
var localesFS embed.FS

// Map for translations and once check variable
var translationsMap map[string]map[string]string
var once sync.Once

// Month names map
var monthNames = map[string][]string{
	"EN": {"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"},
	"PL": {"Sty", "Lut", "Mar", "Kwi", "Maj", "Cze", "Lip", "Sie", "Wrz", "Paź", "Lis", "Gru"},
	"DE": {"Jan", "Feb", "Mär", "Apr", "Mai", "Jun", "Jul", "Aug", "Sep", "Okt", "Nov", "Dez"},
	"FR": {"Jan", "Fév", "Mar", "Avr", "Mai", "Juin", "Juil", "Aoû", "Sep", "Oct", "Nov", "Déc"},
	"CZ": {"Led", "Úno", "Bře", "Dub", "Kvě", "Čer", "Čvc", "Srp", "Zář", "Říj", "Lis", "Pro"},
	"UA": {"Січ", "Лют", "Бer", "Кві", "Тра", "Чер", "Лип", "Сер", "Вер", "Жов", "Лис", "Гру"},
	"RO": {"Ian", "Feb", "Mar", "Apr", "Mai", "Iun", "Iul", "Aug", "Sep", "Oct", "Nov", "Dec"},
	"HU": {"Jan", "Feb", "Már", "Ápr", "Máj", "Jún", "Júl", "Aug", "Szep", "Okt", "Nov", "Dec"},
	"HR": {"Sij", "Vel", "Ožu", "Tra", "Svi", "Lip", "Srp", "Kol", "Ruj", "Lis", "Stu", "Pro"},
	"DA": {"Jan", "Feb", "Mar", "Apr", "Maj", "Jun", "Jul", "Aug", "Sep", "Okt", "Nov", "Dec"},
	"NO": {"Jan", "Feb", "Mar", "Apr", "Mai", "Jun", "Jul", "Aug", "Sep", "Okt", "Nov", "Des"},
	"SW": {"Jan", "Feb", "Mar", "Apr", "Maj", "Jun", "Jul", "Aug", "Sep", "Okt", "Nov", "Dec"},
	"TR": {"Oca", "Şub", "Mar", "Nis", "May", "Haz", "Tem", "Ağu", "Eyl", "Eki", "Kas", "Ara"},
	"AL": {"Jan", "Shk", "Mar", "Pri", "Maj", "Qer", "Kor", "Gus", "Sht", "Tet", "Nën", "Dhj"},
	"IC": {"Jan", "Feb", "Mar", "Apr", "Maí", "Jún", "Júl", "Ágú", "Sep", "Okt", "Nóv", "Des"},
	"BU": {"Яну", "Фев", "Мар", "Апр", "Май", "Юни", "Юли", "Авг", "Сеп", "Окт", "Ное", "Дек"},
}

// GetFormattedDate returns a date string localized for the given language (e.g. "24 Jan").
func GetFormattedDate(lang string, t time.Time) string {
	lang = strings.ToUpper(lang)
	months, ok := monthNames[lang]
	if !ok {
		months = monthNames["EN"]
	}

	m := t.Month() // 1-12
	monthStr := months[m-1]

	return fmt.Sprintf("%02d %s", t.Day(), monthStr)
}

// GetFormattedDateTime returns a date and time string localized (e.g. "24 Jan 15:30").
func GetFormattedDateTime(lang string, t time.Time) string {
	date := GetFormattedDate(lang, t)
	return fmt.Sprintf("%s %02d:%02d", date, t.Hour(), t.Minute())
}

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
