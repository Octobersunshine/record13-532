package stats

import (
	"testing"
	"time"
)

func TestCollectorRecord(t *testing.T) {
	c := NewCollector()
	c.SetSlowThreshold(50 * time.Millisecond)

	c.Record(Slave, "SELECT * FROM users", 10*time.Millisecond)
	c.Record(Slave, "SELECT * FROM users", 20*time.Millisecond)
	c.Record(Master, "INSERT INTO users (name) VALUES (?)", 100*time.Millisecond)
	c.Record(Master, "UPDATE users SET name=? WHERE id=?", 80*time.Millisecond)

	s := c.Summary(10)

	if s.MasterStats.TotalQueries != 2 {
		t.Errorf("master total queries = %d, want 2", s.MasterStats.TotalQueries)
	}
	if s.SlaveStats.TotalQueries != 2 {
		t.Errorf("slave total queries = %d, want 2", s.SlaveStats.TotalQueries)
	}
	if s.AllStats.TotalQueries != 4 {
		t.Errorf("all total queries = %d, want 4", s.AllStats.TotalQueries)
	}
	if s.AllStats.SlowCount != 2 {
		t.Errorf("slow count = %d, want 2", s.AllStats.SlowCount)
	}

	found := false
	for _, q := range s.MasterStats.TopByCount {
		if q.SQL == "INSERT INTO users (name) VALUES (?)" {
			if q.Count != 1 {
				t.Errorf("insert count = %d, want 1", q.Count)
			}
			found = true
		}
	}
	if !found {
		t.Error("insert statement not found in master stats")
	}

	for _, q := range s.SlaveStats.TopByAvgTime {
		if q.SQL == "SELECT * FROM users" {
			if q.Count != 2 {
				t.Errorf("select count = %d, want 2", q.Count)
			}
		}
	}
}

func TestCollectorReset(t *testing.T) {
	c := NewCollector()
	c.Record(Master, "SELECT 1", 10*time.Millisecond)
	s := c.Summary(10)
	if s.AllStats.TotalQueries != 1 {
		t.Fatalf("before reset, total = %d, want 1", s.AllStats.TotalQueries)
	}

	c.Reset()
	s = c.Summary(10)
	if s.AllStats.TotalQueries != 0 {
		t.Errorf("after reset, total = %d, want 0", s.AllStats.TotalQueries)
	}
}

func TestTopNLimit(t *testing.T) {
	c := NewCollector()
	sqls := []string{
		"SELECT * FROM t1",
		"SELECT * FROM t2",
		"SELECT * FROM t3",
		"SELECT * FROM t4",
		"SELECT * FROM t5",
		"SELECT * FROM t6",
		"SELECT * FROM t7",
	}
	for i, sql := range sqls {
		c.Record(Slave, sql, time.Duration(i+1)*time.Millisecond)
	}
	s := c.Summary(5)
	if len(s.SlaveStats.TopByAvgTime) != 5 {
		t.Errorf("topN = %d, want 5", len(s.SlaveStats.TopByAvgTime))
	}
	s = c.Summary(0)
	if len(s.SlaveStats.TopByAvgTime) != len(sqls) {
		t.Errorf("topN(0) = %d, want %d", len(s.SlaveStats.TopByAvgTime), len(sqls))
	}
}

func TestSlowQueries(t *testing.T) {
	c := NewCollector()
	c.SetSlowThreshold(50 * time.Millisecond)

	c.Record(Slave, "SELECT SLEEP(1)", 200*time.Millisecond)
	c.Record(Slave, "SELECT 1", 10*time.Millisecond)

	s := c.Summary(10)
	if len(s.AllStats.SlowQueries) != 1 {
		t.Errorf("slow queries = %d, want 1", len(s.AllStats.SlowQueries))
	}
	if s.AllStats.SlowQueries[0].SQL != "SELECT SLEEP(1)" {
		t.Errorf("slow sql = %q, want SELECT SLEEP(1)", s.AllStats.SlowQueries[0].SQL)
	}
}
