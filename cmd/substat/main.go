package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"bulubula/substat/internal/config"
	"bulubula/substat/internal/scheduler"
	"bulubula/substat/internal/server"
	"bulubula/substat/internal/store"
)

func main() {
	configPath := flag.String("config", "config.yaml", "Path to config file")
	flag.Parse()

	log.Printf("[INFO] Loading configuration from %s...", *configPath)
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("[FATAL] Failed to load config: %v", err)
	}

	log.Printf("[INFO] Initializing store at %s...", cfg.Storage.Path)
	st, err := store.NewStore(cfg.Storage.Path, cfg.Storage.RingBufferSize)
	if err != nil {
		log.Fatalf("[FATAL] Store init failed: %v", err)
	}
	defer st.Close()

	var monitorNames []string
	for _, m := range cfg.Monitors {
		monitorNames = append(monitorNames, m.Name)
	}
	if err := st.LoadHistoryFromFile(monitorNames); err != nil {
		log.Printf("[WARN] Failed to load history from file: %v", err)
	}

	log.Printf("[INFO] Initializing probe scheduler with %d monitors...", len(cfg.Monitors))
	sched, err := scheduler.NewScheduler(cfg, st)
	if err != nil {
		log.Fatalf("[FATAL] Scheduler init failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv, err := server.NewServer(cfg, sched, st)
	if err != nil {
		log.Fatalf("[FATAL] Server init failed: %v", err)
	}

	// Start HTTP server before worker ticks
	go func() {
		log.Printf("[INFO] Substat server listening on http://%s%s", cfg.Server.Listen, cfg.Server.BasePath)
		if err := srv.ListenAndServe(); err != nil {
			log.Printf("[INFO] HTTP server stopped: %v", err)
		}
	}()

	time.Sleep(100 * time.Millisecond)

	sched.Start(ctx)
	defer sched.Stop()

	// Graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("[INFO] Shutting down gracefully...")
	cancel()
	sched.Stop()
	time.Sleep(500 * time.Millisecond)
	fmt.Println("Bye!")
}
