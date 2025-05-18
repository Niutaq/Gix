package utilities

// Function for getting translations
func GetTranslation(lang, key string, params ...interface{}) string {
	if langMap, ok := translations[lang]; ok {
		if val, ok := langMap[key]; ok {
			// if len(params) > 0 {
			// 	return fmt.Sprintf(val, params...)
			// }
			return val
		}
	}

	if lang != "EN" {
		if langMapEN, ok := translations["EN"]; ok {
			if val, ok := langMapEN[key]; ok {
				return val
			}
		}
	}
	return key
}

// Language options
// It is used for internationalization (i18n) of the application's UI text.
var translations = map[string]map[string]string{
	"EN": {
		"info":                 "Select the language and the currency",
		"title":                "Gix",
		"languageLabel":        "Language",
		"buyLabel":             "Buy",
		"sellLabel":            "Sell",
		"loadingText":          "Loading...",
		"errorPrefix":          "Error",
		"cantor_tadek_name":    "Tadek Exchange (Stalowa Wola / Rzeszów)",
		"cantor_kwadrat_name":  "Kwadrat Exchange (Rzeszów)",
		"cantor_supersam_name": "SuperSam Exchange (Rzeszów)",
	},
	"PL": {
		"info":                 "Wybierz język oraz walutę",
		"title":                "Gix",
		"languageLabel":        "Język",
		"buyLabel":             "Kupno",
		"sellLabel":            "Sprzedaż",
		"loadingText":          "Ładowanie...",
		"errorPrefix":          "Błąd",
		"cantor_tadek_name":    "Kantor Tadek (Stalowa Wola / Rzeszów)",
		"cantor_kwadrat_name":  "Kantor Kwadrat (Rzeszów)",
		"cantor_supersam_name": "Kantor SuperSam (Rzeszów)",
	},
	"DE": {
		"info":                 "Wählen Sie die Sprache und die Währung",
		"title":                "Gix",
		"languageLabel":        "Sprache",
		"buyLabel":             "Kaufen",
		"sellLabel":            "Verkaufen",
		"loadingText":          "Laden...",
		"errorPrefix":          "Fehler",
		"cantor_tadek_name":    "Tadek Wechselstube (Stalowa Wola / Rzeszów)",
		"cantor_kwadrat_name":  "Kwadrat Wechselstube (Rzeszów)",
		"cantor_supersam_name": "SuperSam Wechselstube (Rzeszów)",
	},
	"DA": {
		"info":                 "Vælg sprog og valuta",
		"title":                "Gix",
		"languageLabel":        "Sprog",
		"buyLabel":             "Køb",
		"sellLabel":            "Sælg",
		"loadingText":          "Indlæser...",
		"errorPrefix":          "Fejl",
		"cantor_tadek_name":    "Tadek Valutaveksling (Stalowa Wola / Rzeszów)",
		"cantor_kwadrat_name":  "Kwadrat Valutaveksling (Rzeszów)",
		"cantor_supersam_name": "SuperSam Valutaveksling (Rzeszów)",
	},
	"NO": {
		"info":                 "Velg språk og valuta",
		"title":                "Gix",
		"languageLabel":        "Språk",
		"buyLabel":             "Kjøp",
		"sellLabel":            "Selg",
		"loadingText":          "Laster...",
		"errorPrefix":          "Feil",
		"cantor_tadek_name":    "Tadek Valutaveksling (Stalowa Wola / Rzeszów)",
		"cantor_kwadrat_name":  "Kwadrat Valutaveksling (Rzeszów)",
		"cantor_supersam_name": "SuperSam Valutaveksling (Rzeszów)",
	},
	"FR": {
		"info":                 "Sélectionnez la langue et la devise",
		"title":                "Gix",
		"languageLabel":        "Langue",
		"buyLabel":             "Acheter",
		"sellLabel":            "Vendre",
		"loadingText":          "Chargement...",
		"errorPrefix":          "Erreur",
		"cantor_tadek_name":    "Bureau de change Tadek (Stalowa Wola / Rzeszów)",
		"cantor_kwadrat_name":  "Bureau de change Kwadrat (Rzeszów)",
		"cantor_supersam_name": "Bureau de change SuperSam (Rzeszów)",
	},
	"SW": {
		"info":                 "Välj språk och valuta",
		"title":                "Gix",
		"languageLabel":        "Språk",
		"buyLabel":             "Köp",
		"sellLabel":            "Sälj",
		"loadingText":          "Laddar...",
		"errorPrefix":          "Fel",
		"cantor_tadek_name":    "Tadek Växlingskontor (Stalowa Wola / Rzeszów)",
		"cantor_kwadrat_name":  "Kwadrat Växlingskontor (Rzeszów)",
		"cantor_supersam_name": "SuperSam Växlingskontor (Rzeszów)",
	},
	"CZ": {
		"info":                 "Vyberte jazyk a měnu",
		"title":                "Gix",
		"languageLabel":        "Jazyk",
		"buyLabel":             "Koupit",
		"sellLabel":            "Prodat",
		"loadingText":          "Načítání...",
		"errorPrefix":          "Chyba",
		"cantor_tadek_name":    "Směnárna Tadek (Stalowa Wola / Rzeszów)",
		"cantor_kwadrat_name":  "Směnárna Kwadrat (Rzeszów)",
		"cantor_supersam_name": "Směnárna SuperSam (Rzeszów)",
	},
	"HR": {
		"info":                 "Odaberite jezik i valutu",
		"title":                "Gix",
		"languageLabel":        "Jezik",
		"buyLabel":             "Kupi",
		"sellLabel":            "Prodaja",
		"loadingText":          "Učitavanje...",
		"errorPrefix":          "Greška",
		"cantor_tadek_name":    "Mjenjačnica Tadek (Stalowa Wola / Rzeszów)",
		"cantor_kwadrat_name":  "Mjenjačnica Kwadrat (Rzeszów)",
		"cantor_supersam_name": "Mjenjačnica SuperSam (Rzeszów)",
	},
	"HU": {
		"info":                 "Válasszon nyelvet és valutát",
		"title":                "Gix",
		"languageLabel":        "Nyelv",
		"buyLabel":             "Vásárlás",
		"sellLabel":            "Eladás",
		"loadingText":          "Betöltés...",
		"errorPrefix":          "Hiba",
		"cantor_tadek_name":    "Tadek Pénzváltó (Stalowa Wola / Rzeszów)",
		"cantor_kwadrat_name":  "Kwadrat Pénzváltó (Rzeszów)",
		"cantor_supersam_name": "SuperSam Pénzváltó (Rzeszów)",
	},
	"UA": {
		"info":                 "Виберіть мову та валюту",
		"title":                "Gix",
		"languageLabel":        "Мова",
		"buyLabel":             "Купити",
		"sellLabel":            "Продати",
		"loadingText":          "Завантаження...",
		"errorPrefix":          "Помилка",
		"cantor_tadek_name":    "Обмінний пункт Tadek (Stalowa Wola / Rzeszów)",
		"cantor_kwadrat_name":  "Обмінний пункт Kwadrat (Rzeszów)",
		"cantor_supersam_name": "Обмінний пункт SuperSam (Rzeszów)",
	},
	"BU": {
		"info":                 "Изберете език и валута",
		"title":                "Gix",
		"languageLabel":        "Език",
		"buyLabel":             "Купете",
		"sellLabel":            "Продавам",
		"loadingText":          "Зареждане...",
		"errorPrefix":          "Грешка",
		"cantor_tadek_name":    "Обменно бюро Tadek (Stalowa Wola / Rzeszów)",
		"cantor_kwadrat_name":  "Обменно бюро Kwadrat (Rzeszów)",
		"cantor_supersam_name": "Обменно бюро SuperSam (Rzeszów)",
	},
	"RO": {
		"info":                 "Selectați limba și moneda",
		"title":                "Gix",
		"languageLabel":        "Limba",
		"buyLabel":             "Cumpără",
		"sellLabel":            "Vând",
		"loadingText":          "Se încarcă...",
		"errorPrefix":          "Eroare",
		"cantor_tadek_name":    "Casa de schimb Tadek (Stalowa Wola / Rzeszów)",
		"cantor_kwadrat_name":  "Casa de schimb Kwadrat (Rzeszów)",
		"cantor_supersam_name": "Casa de schimb SuperSam (Rzeszów)",
	},
	"AL": {
		"info":                 "Zgjidh gjuhën dhe monedhën",
		"title":                "Gix",
		"languageLabel":        "Gjuha",
		"buyLabel":             "Bli",
		"sellLabel":            "Shitet",
		"loadingText":          "Duke u ngarkuar...",
		"errorPrefix":          "Gabim",
		"cantor_tadek_name":    "Këmbimore Tadek (Stalowa Wola / Rzeszów)",
		"cantor_kwadrat_name":  "Këmbimore Kwadrat (Rzeszów)",
		"cantor_supersam_name": "Këmbimore SuperSam (Rzeszów)",
	},
	"TR": {
		"info":                 "Dil ve para birimini seçin",
		"title":                "Gix",
		"languageLabel":        "Dil",
		"buyLabel":             "Satın Al",
		"sellLabel":            "Sat",
		"loadingText":          "Yükleniyor...",
		"errorPrefix":          "Hata",
		"cantor_tadek_name":    "Tadek Döviz Bürosu (Stalowa Wola / Rzeszów)",
		"cantor_kwadrat_name":  "Kwadrat Döviz Bürosu (Rzeszów)",
		"cantor_supersam_name": "SuperSam Döviz Bürosu (Rzeszów)",
	},
	"IC": {
		"info":                 "Veldu tungumál og gjaldmiðil",
		"title":                "Gix",
		"languageLabel":        "Tungumál",
		"buyLabel":             "Kaupa",
		"sellLabel":            "Selja",
		"loadingText":          "Hleð...",
		"errorPrefix":          "Villa",
		"cantor_tadek_name":    "Tadek Gjaldeyrisskipti (Stalowa Wola / Rzeszów)",
		"cantor_kwadrat_name":  "Kwadrat Gjaldeyrisskipti (Rzeszów)",
		"cantor_supersam_name": "SuperSam Gjaldeyrisskipti (Rzeszów)",
	},
}
