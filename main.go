package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"sqlrouter/db"
	"sqlrouter/handler"
	"sqlrouter/router"
	"sqlrouter/stats"
)

type Config struct {
	MasterDSN     string   `json:"master_dsn"`
	SlaveDSNs     []string `json:"slave_dsns"`
	MaxOpen       int      `json:"max_open_conns"`
	MaxIdle       int      `json:"max_idle_conns"`
	MaxLife       int      `json:"max_conn_life_seconds"`
	Port          int      `json:"port"`
	SlowThreshold int      `json:"slow_threshold_ms"`
}

func main() {
	cfgPath := flag.String("config", "config.json", "path to config file")
	flag.Parse()

	cfg, err := loadConfig(*cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	pool, err := db.NewPool(db.Config{
		MasterDSN: cfg.MasterDSN,
		SlaveDSNs: cfg.SlaveDSNs,
		MaxOpen:   cfg.MaxOpen,
		MaxIdle:   cfg.MaxIdle,
		MaxLife:   cfg.MaxLife,
	})
	if err != nil {
		log.Fatalf("init db pool: %v", err)
	}
	defer pool.Close()

	coll := stats.NewCollector()
	if cfg.SlowThreshold > 0 {
		coll.SetSlowThreshold(time.Duration(cfg.SlowThreshold) * time.Millisecond)
	}

	rtr := router.New(pool, coll)

	mux := http.NewServeMux()
	mux.HandleFunc("/query", handler.Query(rtr))
	mux.HandleFunc("/query/tx", handler.Tx(rtr))
	mux.HandleFunc("/stats", handler.Stats(coll))

	addr := fmt.Sprintf(":%d", cfg.Port)
	log.Printf("sql-router listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file %s: %w", path, err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if cfg.MasterDSN == "" {
		return nil, fmt.Errorf("master_dsn is required")
	}
	if cfg.Port == 0 {
		cfg.Port = 8080
	}
	if cfg.MaxOpen == 0 {
		cfg.MaxOpen = 10
	}
	if cfg.MaxIdle == 0 {
		cfg.MaxIdle = 5
	}
	if cfg.MaxLife == 0 {
		cfg.MaxLife = 300
	}
	return &cfg, nil
}
