package main

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"glpi-painel/internal/config"
	"glpi-painel/internal/glpi"
	"glpi-painel/internal/handler"
	"glpi-painel/internal/logger"
	"glpi-painel/web"
)

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func securityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrappedWriter := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(wrappedWriter, r)

		if r.URL.Path == "/health" {
			return
		}

		duration := time.Since(start)
		logLevel := slog.LevelInfo
		if wrappedWriter.status >= 500 {
			logLevel = slog.LevelError
		} else if wrappedWriter.status >= 400 {
			logLevel = slog.LevelWarn
		}

		slog.Log(r.Context(), logLevel, "HTTP Request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", wrappedWriter.status),
			slog.String("duration", duration.String()),
			slog.String("ip", r.RemoteAddr),
		)
	})
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("Erro fatal ao carregar configurações: %v\n", err)
		panic(err)
	}

	logger.Init(cfg)
	slog.Info("Configurações carregadas", "glpi_url", cfg.GlpiURL, "tech_group", cfg.TechnicalGroup)

	glpiClient := glpi.NewClient(cfg)

	apiHandler := &handler.APIHandler{
		Client: glpiClient,
		Config: cfg,
	}

	mux := http.NewServeMux()

	// Endpoints da API
	mux.HandleFunc("/api/tickets", apiHandler.HandleTickets)
	mux.HandleFunc("/health", apiHandler.HandleHealth)

	// Servir frontend estático embutido
	sub, err := fs.Sub(web.StaticFS, "static")
	if err != nil {
		slog.Error("Erro ao preparar sistema de arquivos estáticos", "error", err)
		panic(err)
	}
	fileServer := http.FileServer(http.FS(sub))
	mux.Handle("/", fileServer)

	finalHandler := loggingMiddleware(securityMiddleware(mux))

	serverAddr := ":" + cfg.Port
	srv := &http.Server{
		Addr:    serverAddr,
		Handler: finalHandler,
	}

	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		slog.Info("Painel GLPI iniciado com sucesso", "porta", cfg.Port, "url", "http://localhost:"+cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Erro fatal no servidor HTTP", "error", err)
			os.Exit(1)
		}
	}()

	sig := <-stopChan
	slog.Info("Sinal de encerramento recebido", "signal", sig)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	slog.Info("Encerrando servidor graciosamente...")
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("Erro ao desligar servidor forçadamente", "error", err)
	}
	slog.Info("Servidor desligado com sucesso.")
}
