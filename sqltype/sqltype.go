package sqltype

import (
	"strings"
	"unicode"
)

type SQLType int

const (
	Read SQLType = iota
	Write
	Unknown
)

func Classify(sql string) SQLType {
	s := strings.TrimSpace(sql)
	if s == "" {
		return Unknown
	}

	firstWord := extractFirstWord(s)
	switch strings.ToUpper(firstWord) {
	case "SELECT", "SHOW", "DESCRIBE", "DESC", "EXPLAIN":
		return Read
	case "INSERT", "UPDATE", "DELETE", "REPLACE", "CREATE", "ALTER",
		"DROP", "TRUNCATE", "RENAME", "GRANT", "REVOKE", "LOCK",
		"UNLOCK", "SET", "CALL", "BEGIN", "START", "COMMIT",
		"ROLLBACK", "SAVEPOINT", "RELEASE":
		return Write
	default:
		return Unknown
	}
}

func IsRead(sql string) bool  { return Classify(sql) == Read }
func IsWrite(sql string) bool { return Classify(sql) == Write }

func extractFirstWord(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsSpace(r) {
			break
		}
		b.WriteRune(unicode.ToUpper(r))
	}
	return b.String()
}
