package sqltype

import (
	"testing"
)

func TestClassify(t *testing.T) {
	tests := []struct {
		sql    string
		want   SQLType
	}{
		{"SELECT * FROM users", Read},
		{"  select id from t", Read},
		{"SHOW TABLES", Read},
		{"DESCRIBE users", Read},
		{"DESC users", Read},
		{"EXPLAIN SELECT * FROM t", Read},

		{"INSERT INTO users (name) VALUES ('alice')", Write},
		{"UPDATE users SET name='bob'", Write},
		{"DELETE FROM users WHERE id=1", Write},
		{"CREATE TABLE t (id INT)", Write},
		{"ALTER TABLE t ADD col INT", Write},
		{"DROP TABLE t", Write},
		{"TRUNCATE TABLE t", Write},
		{"SET NAMES utf8mb4", Write},
		{"BEGIN", Write},
		{"START TRANSACTION", Write},

		{"", Unknown},
		{"   ", Unknown},
	}

	for _, tt := range tests {
		got := Classify(tt.sql)
		if got != tt.want {
			t.Errorf("Classify(%q) = %v, want %v", tt.sql, got, tt.want)
		}
	}
}

func TestIsRead(t *testing.T) {
	if !IsRead("SELECT 1") {
		t.Error("IsRead(SELECT 1) = false, want true")
	}
	if IsRead("INSERT INTO t VALUES(1)") {
		t.Error("IsRead(INSERT) = true, want false")
	}
}

func TestIsWrite(t *testing.T) {
	if !IsWrite("DELETE FROM t") {
		t.Error("IsWrite(DELETE) = false, want true")
	}
	if IsWrite("SELECT 1") {
		t.Error("IsWrite(SELECT) = true, want false")
	}
}
