package main

import (
	"embed"
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/violinbg/violin.school/internal/ai"
	"github.com/violinbg/violin.school/internal/database"
	"github.com/violinbg/violin.school/internal/server"
)

//go:embed all:ui/dist/browser
var uiFS embed.FS

//go:embed migrations/*.sql
var migrationsFS embed.FS

func main() {
	addr := flag.String("addr", "", "HTTP listen address")  // do not set default here to allow env var override in resolveAddr
	dbPath := flag.String("db", "", "SQLite database path") // directly use the flag value without normalization to allow env var override in resolveDBPath
	flag.Parse()

	finalAddr := resolveAddr(*addr)
	finalDBPath := resolveDBPath(*dbPath)
	validateAIEncryptionKey()

	if dir := filepath.Dir(finalDBPath); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Fatalf("failed to create database directory: %v", err)
		}
	}

	db, err := database.Open(finalDBPath)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	if err := database.Migrate(db, migrationsFS); err != nil {
		log.Fatalf("failed to run migrations: %v", err)
	}

	srv := server.New(db, uiFS)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("server listening on %s", finalAddr)
		if err := srv.Run(finalAddr); err != nil {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-quit
	log.Println("shutting down...")
}

func validateAIEncryptionKey() {
	rawKey := strings.TrimSpace(os.Getenv("XAI_TOKEN_ENCRYPTION_KEY"))
	if rawKey == "" {
		log.Printf("warning: XAI_TOKEN_ENCRYPTION_KEY is not set; xAI token storage endpoints will return an error until configured")
		return
	}

	if _, err := ai.ParseEncryptionKey(rawKey); err != nil {
		log.Fatalf("invalid XAI_TOKEN_ENCRYPTION_KEY: %v", err)
	}
}

func resolveAddr(addrFlag string) string {
	if trimmed := strings.TrimSpace(addrFlag); trimmed != "" {
		return normalizeAddr(trimmed)
	}

	if port := strings.TrimSpace(os.Getenv("PORT")); port != "" {
		return normalizeAddr(port)
	}

	return ":8080"
}

func resolveDBPath(dbFlag string) string {
	if trimmed := strings.TrimSpace(dbFlag); trimmed != "" {
		return trimmed
	}

	if envDBPath := strings.TrimSpace(os.Getenv("DB_PATH")); envDBPath != "" {
		return envDBPath
	}

	return "violin.school.db"
}

func normalizeAddr(addr string) string {
	if _, err := strconv.Atoi(addr); err == nil {
		return ":" + addr
	}

	return addr
}
