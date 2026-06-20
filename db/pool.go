package db

import (
	"database/sql"
	"fmt"
	"math/rand"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type Config struct {
	MasterDSN string   `json:"master_dsn"`
	SlaveDSNs []string `json:"slave_dsns"`
	MaxOpen   int      `json:"max_open"`
	MaxIdle   int      `json:"max_idle"`
	MaxLife   int      `json:"max_life_seconds"`
}

type Pool struct {
	master *sql.DB
	slaves []*sql.DB
	mu     sync.RWMutex
	rnd    *rand.Rand
}

func NewPool(cfg Config) (*Pool, error) {
	master, err := openDB(cfg.MasterDSN, cfg.MaxOpen, cfg.MaxIdle, cfg.MaxLife)
	if err != nil {
		return nil, fmt.Errorf("connect master failed: %w", err)
	}

	var slaves []*sql.DB
	for i, dsn := range cfg.SlaveDSNs {
		slave, err := openDB(dsn, cfg.MaxOpen, cfg.MaxIdle, cfg.MaxLife)
		if err != nil {
			return nil, fmt.Errorf("connect slave[%d] failed: %w", i, err)
		}
		slaves = append(slaves, slave)
	}

	return &Pool{
		master: master,
		slaves: slaves,
		rnd:    rand.New(rand.NewSource(time.Now().UnixNano())),
	}, nil
}

func (p *Pool) Master() *sql.DB {
	return p.master
}

func (p *Pool) Slave() *sql.DB {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if len(p.slaves) == 0 {
		return p.master
	}
	return p.slaves[p.rnd.Intn(len(p.slaves))]
}

func (p *Pool) Close() error {
	var firstErr error
	if err := p.master.Close(); err != nil && firstErr == nil {
		firstErr = err
	}
	for _, s := range p.slaves {
		if err := s.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func openDB(dsn string, maxOpen, maxIdle, maxLife int) (*sql.DB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	db.SetConnMaxLifetime(time.Duration(maxLife) * time.Second)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}
