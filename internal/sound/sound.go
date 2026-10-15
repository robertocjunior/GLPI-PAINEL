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
	mu          sync.RWMutex
	customSound []byte
	customFile  = filepath.Join("data", "custom_sound.mp3")
)

// Init inicializa o som customizado: baixa caso haja URL configurada,
// ou carrega do cache local se existir.
func Init(soundURL string) {
	if soundURL == "" {
		slog.Info("Nenhum som customizado configurado (NOTIFICATION_SOUND_URL vazio). Usando som sintetizado padrão.")
		return
	}

	slog.Info("Baixando arquivo de som customizado...", "url", soundURL)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, soundURL, nil)
	if err != nil {
		slog.Error("URL de som inválida", "url", soundURL, "error", err)
		return
	}

	req.Header.Set("User-Agent", "GLPI-Painel/1.0")

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		slog.Error("Falha ao baixar som customizado", "url", soundURL, "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Error("Servidor retornou status não OK ao baixar som", "status", resp.Status)
		return
	}

	// Limita leitura em 15MB para segurança
	lr := io.LimitReader(resp.Body, 15*1024*1024)
	data, err := io.ReadAll(lr)
	if err != nil {
		slog.Error("Erro ao ler conteúdo do arquivo de som", "error", err)
		return
	}

	if len(data) == 0 {
		slog.Warn("Arquivo de som baixado está vazio")
		return
	}

	mu.Lock()
	customSound = data
	mu.Unlock()

	// Tenta persistir no diretório data local
	if err := os.MkdirAll("data", 0755); err == nil {
		if err := os.WriteFile(customFile, data, 0644); err != nil {
			slog.Warn("Não foi possível salvar cache local do som", "error", err)
		}
	}

	slog.Info("Som customizado carregado com sucesso!", "tamanho_bytes", len(data))
}

// HasCustomSound verifica se há som customizado disponível
func HasCustomSound() bool {
	mu.RLock()
	defer mu.RUnlock()
	return len(customSound) > 0
}

// Handler HTTP para servir o arquivo .mp3
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
