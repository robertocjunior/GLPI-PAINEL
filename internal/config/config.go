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

	// Logs
	LogMaxSize int
	LogMaxAge  int
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

	cfg := &Config{
		Port:                 port,
		GlpiURL:              glpiURL,
		GlpiAppToken:         appToken,
		GlpiUserToken:        userToken,
		TechnicalGroup:       group,
		RefreshIntervalSec:   refreshSec,
		NotificationSoundURL: soundURL,
		LogMaxSize:           logSize,
		LogMaxAge:            logAge,
	}

	if cfg.GlpiAppToken == "" || cfg.GlpiUserToken == "" {
		return nil, fmt.Errorf("GLPI_APP_TOKEN e GLPI_USER_TOKEN são obrigatórios nas variáveis de ambiente ou .env")
	}

	return cfg, nil
}
