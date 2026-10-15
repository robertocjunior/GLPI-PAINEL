package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	Port         string
	GlpiURL      string
	GlpiAppToken string
	GlpiUserToken string
	TechnicalGroup string // ID ou nome do grupo técnico (padrão "1" para Sistemas)
	RefreshIntervalSec int // Intervalo de polling no frontend
	NotificationSoundURL string // URL opcional de arquivo de som .mp3 para novos chamados
	CriticalSoundURL     string // URL opcional de arquivo de som .mp3 para chamados críticos/muito alta
	CategorySounds       []CategorySoundRule // Regras de som por categoria

	// Logs
	LogMaxSize int
	LogMaxAge  int
}

type CategorySoundRule struct {
	Index    int    `json:"index"`
	Category string `json:"category"`
	SoundURL string `json:"sound_url"`
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	glpiURL := os.Getenv("GLPI_URL")
	if glpiURL == "" {
		glpiURL = "https://grupolipetral.verdanadesk.com"
	}
	glpiURL = strings.TrimRight(glpiURL, "/")
	glpiURL = strings.TrimSuffix(glpiURL, "/front") // sanitiza caso termine em /front

	appToken := os.Getenv("GLPI_APP_TOKEN")
	userToken := os.Getenv("GLPI_USER_TOKEN")

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	group := os.Getenv("GLPI_TECH_GROUP")
	if group == "" {
		group = "1"
	}

	refreshSec, _ := strconv.Atoi(os.Getenv("REFRESH_INTERVAL_SEC"))
	if refreshSec <= 0 {
		refreshSec = 30
	}

	logSize, _ := strconv.Atoi(os.Getenv("LOG_MAX_SIZE_MB"))
	logAge, _ := strconv.Atoi(os.Getenv("LOG_MAX_AGE_DAYS"))

	soundURL := strings.TrimSpace(os.Getenv("NOTIFICATION_SOUND_URL"))
	critSoundURL := strings.TrimSpace(os.Getenv("CRITICAL_SOUND_URL"))
	if critSoundURL == "" {
		critSoundURL = "https://www.myinstants.com/media/sounds/airport-announcement-starting.mp3"
	}
	catSoundsRaw := strings.TrimSpace(os.Getenv("CATEGORY_SOUNDS"))
	catSounds := parseCategorySounds(catSoundsRaw)

	cfg := &Config{
		Port:                 port,
		GlpiURL:              glpiURL,
		GlpiAppToken:         appToken,
		GlpiUserToken:        userToken,
		TechnicalGroup:       group,
		RefreshIntervalSec:   refreshSec,
		NotificationSoundURL: soundURL,
		CriticalSoundURL:     critSoundURL,
		CategorySounds:       catSounds,
		LogMaxSize:           logSize,
		LogMaxAge:            logAge,
	}

	if cfg.GlpiAppToken == "" || cfg.GlpiUserToken == "" {
		return nil, fmt.Errorf("GLPI_APP_TOKEN e GLPI_USER_TOKEN são obrigatórios nas variáveis de ambiente ou .env")
	}

	return cfg, nil
}

// parseCategorySounds suporta formatos como:
// (Categoria, URL) | (Categoria2, URL2)
// Categoria, URL ; Categoria2, URL2
// [ "Categoria", "URL" ]
func parseCategorySounds(raw string) []CategorySoundRule {
	if raw == "" {
		return nil
	}

	var rules []CategorySoundRule
	// Normaliza separadores de pares
	// Primeiro verifica se usa formato com parênteses: (cat, url) ou (cat, url);(cat2, url2)
	cleaned := strings.TrimSpace(raw)
	
	// Divide por parênteses fechando ou quebras de linha ou ponto e vírgula
	var rawPairs []string
	if strings.Contains(cleaned, "(") && strings.Contains(cleaned, ")") {
		start := -1
		for i, r := range cleaned {
			if r == '(' {
				start = i + 1
			} else if r == ')' && start != -1 {
				rawPairs = append(rawPairs, cleaned[start:i])
				start = -1
			}
		}
	} else {
		// Divide por ponto e vírgula ou nova linha
		lines := strings.FieldsFunc(cleaned, func(r rune) bool {
			return r == ';' || r == '\n'
		})
		rawPairs = append(rawPairs, lines...)
	}

	idx := 0
	for _, pair := range rawPairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}

		// Divide por vírgula ou pipe
		var parts []string
		if strings.Contains(pair, "http://") || strings.Contains(pair, "https://") {
			// Localiza onde começa http:// ou https://
			httpIdx := strings.Index(pair, "http://")
			if httpIdx == -1 {
				httpIdx = strings.Index(pair, "https://")
			}
			if httpIdx > 0 {
				catPart := strings.TrimRight(strings.TrimSpace(pair[:httpIdx]), ",|; ")
				urlPart := strings.TrimSpace(pair[httpIdx:])
				parts = []string{catPart, urlPart}
			}
		}

		if len(parts) < 2 {
			p := strings.Split(pair, ",")
			if len(p) >= 2 {
				parts = []string{strings.TrimSpace(p[0]), strings.TrimSpace(p[1])}
			}
		}

		if len(parts) >= 2 {
			cat := strings.Trim(strings.TrimSpace(parts[0]), `"'`)
			url := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
			if cat != "" && url != "" {
				rules = append(rules, CategorySoundRule{
					Index:    idx,
					Category: cat,
					SoundURL: url,
				})
				idx++
			}
		}
	}

	return rules
}
