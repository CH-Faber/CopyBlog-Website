package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "hash-password":
			hashPasswordCommand()
			return
		case "generate-vapid":
			generateVAPIDCommand()
			return
		}
	}

	cfg, err := loadConfig()
	if err != nil {
		log.Fatal(err)
	}
	store, err := openStore(cfg.DatabasePath)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	server := newServer(cfg, store)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go server.push.run(ctx)
	go runBackupLoop(ctx, store, cfg.BackupDir)

	httpServer := &http.Server{
		Addr:              net.JoinHostPort(cfg.Host, cfg.Port),
		Handler:           server.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       75 * time.Second,
		WriteTimeout:      75 * time.Second,
		IdleTimeout:       90 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()

	log.Printf("Faber organizer is listening on %s (timezone=%s, push=%t)", httpServer.Addr, cfg.Timezone, server.push.enabled())
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func hashPasswordCommand() {
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		log.Fatal("read password from stdin")
	}
	password := strings.TrimSpace(scanner.Text())
	if len(password) < 12 {
		log.Fatal("password must contain at least 12 characters")
	}
	hash, err := hashPassword(password)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(hash)
}

func generateVAPIDCommand() {
	privateKey, publicKey, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("VAPID_PUBLIC_KEY=%s\nVAPID_PRIVATE_KEY=%s\n", publicKey, privateKey)
}
