package stats

import (
	"sort"
	"strings"
	"sync"
	"time"

	"sqlrouter/sqltype"
)

const defaultSlowThreshold = 100 * time.Millisecond

type Target int

const (
	Master Target = iota
	Slave
)

func (t Target) String() string {
	switch t {
	case Master:
		return "master"
	case Slave:
		return "slave"
	default:
		return "unknown"
	}
}

type QueryRecord struct {
	SQL        string
	Type       sqltype.SQLType
	Target     Target
	Durations  []time.Duration
	TotalTime  time.Duration
	Count      int64
	MaxTime    time.Duration
	MinTime    time.Duration
}

type QueryStat struct {
	SQL       string `json:"sql"`
	Type      string `json:"type"`
	Count     int64  `json:"count"`
	TotalTime string `json:"total_time"`
	AvgTime   string `json:"avg_time"`
	MaxTime   string `json:"max_time"`
	MinTime   string `json:"min_time"`
}

type TargetStats struct {
	Target        string       `json:"target"`
	TotalQueries  int64        `json:"total_queries"`
	TotalTime     string       `json:"total_time"`
	SlowCount     int64        `json:"slow_count"`
	TopByAvgTime  []QueryStat  `json:"top_by_avg_time"`
	TopByMaxTime  []QueryStat  `json:"top_by_max_time"`
	TopByCount    []QueryStat  `json:"top_by_count"`
	SlowQueries   []QueryStat  `json:"slow_queries"`
}

type StatsSummary struct {
	SlowThreshold  string        `json:"slow_threshold"`
	MasterStats    TargetStats   `json:"master"`
	SlaveStats     TargetStats   `json:"slave"`
	AllStats       TargetStats   `json:"all"`
}

type Collector struct {
	mu            sync.RWMutex
	master        map[string]*QueryRecord
	slave         map[string]*QueryRecord
	slowThreshold time.Duration
}

func NewCollector() *Collector {
	return &Collector{
		master:        make(map[string]*QueryRecord),
		slave:         make(map[string]*QueryRecord),
		slowThreshold: defaultSlowThreshold,
	}
}

func (c *Collector) SetSlowThreshold(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.slowThreshold = d
}

func (c *Collector) Record(target Target, sql string, duration time.Duration) {
	sqlType := sqltype.Classify(sql)

	c.mu.Lock()
	defer c.mu.Unlock()

	var store map[string]*QueryRecord
	switch target {
	case Master:
		store = c.master
	case Slave:
		store = c.slave
	default:
		return
	}

	key := normalizeSQL(sql)
	rec, ok := store[key]
	if !ok {
		rec = &QueryRecord{
			SQL:     key,
			Type:    sqlType,
			Target:  target,
			MinTime: duration,
		}
		store[key] = rec
	}
	rec.Durations = append(rec.Durations, duration)
	rec.TotalTime += duration
	rec.Count++
	if duration > rec.MaxTime {
		rec.MaxTime = duration
	}
	if duration < rec.MinTime {
		rec.MinTime = duration
	}
}

func (c *Collector) Summary(topN int) StatsSummary {
	c.mu.RLock()
	defer c.mu.RUnlock()

	masterList := c.toList(c.master)
	slaveList := c.toList(c.slave)
	allList := append(masterList, slaveList...)

	return StatsSummary{
		SlowThreshold: c.slowThreshold.String(),
		MasterStats:   c.buildTargetStats("master", masterList, topN),
		SlaveStats:    c.buildTargetStats("slave", slaveList, topN),
		AllStats:      c.buildTargetStats("all", allList, topN),
	}
}

func (c *Collector) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.master = make(map[string]*QueryRecord)
	c.slave = make(map[string]*QueryRecord)
}

func (c *Collector) toList(store map[string]*QueryRecord) []*QueryRecord {
	list := make([]*QueryRecord, 0, len(store))
	for _, r := range store {
		list = append(list, r)
	}
	return list
}

func (c *Collector) buildTargetStats(name string, list []*QueryRecord, topN int) TargetStats {
	ts := TargetStats{Target: name}
	var totalDur time.Duration

	for _, r := range list {
		ts.TotalQueries += r.Count
		totalDur += r.TotalTime
		for _, d := range r.Durations {
			if d >= c.slowThreshold {
				ts.SlowCount++
			}
		}
	}
	ts.TotalTime = totalDur.String()

	byAvg := make([]*QueryRecord, len(list))
	copy(byAvg, list)
	sort.Slice(byAvg, func(i, j int) bool {
		ai := byAvg[i].TotalTime / time.Duration(byAvg[i].Count)
		aj := byAvg[j].TotalTime / time.Duration(byAvg[j].Count)
		return ai > aj
	})
	ts.TopByAvgTime = c.toStat(byAvg, topN)

	byMax := make([]*QueryRecord, len(list))
	copy(byMax, list)
	sort.Slice(byMax, func(i, j int) bool {
		return byMax[i].MaxTime > byMax[j].MaxTime
	})
	ts.TopByMaxTime = c.toStat(byMax, topN)

	byCount := make([]*QueryRecord, len(list))
	copy(byCount, list)
	sort.Slice(byCount, func(i, j int) bool {
		return byCount[i].Count > byCount[j].Count
	})
	ts.TopByCount = c.toStat(byCount, topN)

	var slowList []*QueryRecord
	for _, r := range list {
		for _, d := range r.Durations {
			if d >= c.slowThreshold {
				slowList = append(slowList, r)
				break
			}
		}
	}
	sort.Slice(slowList, func(i, j int) bool {
		return slowList[i].MaxTime > slowList[j].MaxTime
	})
	ts.SlowQueries = c.toStat(slowList, topN)

	return ts
}

func (c *Collector) toStat(list []*QueryRecord, n int) []QueryStat {
	if n <= 0 || n > len(list) {
		n = len(list)
	}
	result := make([]QueryStat, 0, n)
	for i := 0; i < n; i++ {
		r := list[i]
		avg := r.TotalTime / time.Duration(r.Count)
		result = append(result, QueryStat{
			SQL:       r.SQL,
			Type:      sqltype.String(r.Type),
			Count:     r.Count,
			TotalTime: r.TotalTime.String(),
			AvgTime:   avg.String(),
			MaxTime:   r.MaxTime.String(),
			MinTime:   r.MinTime.String(),
		})
	}
	return result
}

func normalizeSQL(sql string) string {
	s := strings.TrimSpace(sql)
	s = strings.TrimSuffix(s, ";")
	return s
}
