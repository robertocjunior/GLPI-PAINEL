package sound

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	mu            sync.RWMutex
	customSound   []byte
	criticalSound []byte
	customFile    = filepath.Join("data", "custom_sound.mp3")
	criticalFile  = filepath.Join("data", "critical_sound.mp3")
)

// Init inicializa os sons customizados: baixa caso haja URLs configuradas.
func Init(defaultSoundURL, critSoundURL string) {
	if defaultSoundURL != "" {
		data := downloadSound("padrão", defaultSoundURL, customFile)
		if len(data) > 0 {
			mu.Lock()
			customSound = data
			mu.Unlock()
		}
	} else {
		slog.Info("Nenhum som de notificação configurado (NOTIFICATION_SOUND_URL vazio). Usando som sintetizado padrão.")
	}

	if critSoundURL != "" {
		data := downloadSound("crítico", critSoundURL, criticalFile)
		if len(data) > 0 {
			mu.Lock()
			criticalSound = data
			mu.Unlock()
		}
	} else {
		slog.Info("Nenhum som crítico configurado (CRITICAL_SOUND_URL vazio). Usando som sintetizado de alerta crítico.")
	}
}

func downloadSound(soundType, soundURL, cachePath string) []byte {
	slog.Info(fmt.Sprintf("Baixando arquivo de som %s...", soundType), "url", soundURL)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, soundURL, nil)
	if err != nil {
		slog.Error(fmt.Sprintf("URL de som %s inválida", soundType), "url", soundURL, "error", err)
		return nil
	}

	req.Header.Set("User-Agent", "GLPI-Painel/1.0")

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		slog.Error(fmt.Sprintf("Falha ao baixar som %s", soundType), "url", soundURL, "error", err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Error(fmt.Sprintf("Servidor retornou status não OK ao baixar som %s", soundType), "status", resp.Status)
		return nil
	}

	lr := io.LimitReader(resp.Body, 15*1024*1024)
	data, err := io.ReadAll(lr)
	if err != nil {
		slog.Error(fmt.Sprintf("Erro ao ler conteúdo do som %s", soundType), "error", err)
		return nil
	}

	if len(data) == 0 {
		slog.Warn(fmt.Sprintf("Arquivo de som %s baixado está vazio", soundType))
		return nil
	}

	if err := os.MkdirAll("data", 0755); err == nil {
		if err := os.WriteFile(cachePath, data, 0644); err != nil {
			slog.Warn(fmt.Sprintf("Não foi possível salvar cache do som %s", soundType), "error", err)
		}
	}

	slog.Info(fmt.Sprintf("Som %s carregado com sucesso!", soundType), "tamanho_bytes", len(data))
	return data
}

// Handler HTTP para servir o arquivo .mp3 padrão
func Handler(w http.ResponseWriter, r *http.Request) {
	mu.RLock()
	data := customSound
	mu.RUnlock()

	if len(data) == 0 {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// CriticalHandler HTTP para servir o arquivo .mp3 de chamados críticos
func CriticalHandler(w http.ResponseWriter, r *http.Request) {
	mu.RLock()
	data := criticalSound
	mu.RUnlock()

	if len(data) == 0 {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
