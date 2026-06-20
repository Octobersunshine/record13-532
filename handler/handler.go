package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"sqlrouter/router"
	"sqlrouter/sqltype"
	"sqlrouter/stats"
)

type QueryRequest struct {
	SQL    string `json:"sql"`
	Params []any  `json:"params,omitempty"`
}

type ColumnInfo struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type QueryResponse struct {
	OK           bool         `json:"ok"`
	Type         string       `json:"type,omitempty"`
	Target       string       `json:"target,omitempty"`
	Columns      []ColumnInfo `json:"columns,omitempty"`
	Rows         []map[string]any `json:"rows,omitempty"`
	RowsAffected int64        `json:"rows_affected,omitempty"`
	LastInsertID int64        `json:"last_insert_id,omitempty"`
	Error        string       `json:"error,omitempty"`
}

type TxStatement struct {
	SQL    string `json:"sql"`
	Params []any  `json:"params,omitempty"`
}

type TxRequest struct {
	Statements []TxStatement `json:"statements"`
}

type TxResult struct {
	Type         string            `json:"type"`
	Columns      []ColumnInfo      `json:"columns,omitempty"`
	Rows         []map[string]any  `json:"rows,omitempty"`
	RowsAffected int64             `json:"rows_affected,omitempty"`
	LastInsertID int64             `json:"last_insert_id,omitempty"`
}

type TxResponse struct {
	OK       bool       `json:"ok"`
	Target   string     `json:"target,omitempty"`
	Results  []TxResult `json:"results,omitempty"`
	Error    string     `json:"error,omitempty"`
}

func Query(rtr *router.Router) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, QueryResponse{
				OK:    false,
				Error: "method not allowed, use POST",
			})
			return
		}

		var req QueryRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, QueryResponse{
				OK:    false,
				Error: "invalid request body: " + err.Error(),
			})
			return
		}

		if req.SQL == "" {
			writeJSON(w, http.StatusBadRequest, QueryResponse{
				OK:    false,
				Error: "sql is required",
			})
			return
		}

		sqlType := sqltype.Classify(req.SQL)
		typeStr := typeString(sqlType)
		target := targetString(sqlType)

		switch sqlType {
		case sqltype.Read:
			rows, err := rtr.Query(r.Context(), req.SQL, req.Params...)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, QueryResponse{
					OK:    false,
					Type:  typeStr,
					Error: err.Error(),
				})
				return
			}
			defer rows.Close()

			colInfos, result, err := scanRows(rows)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, QueryResponse{
					OK:    false,
					Type:  typeStr,
					Error: err.Error(),
				})
				return
			}

			writeJSON(w, http.StatusOK, QueryResponse{
				OK:      true,
				Type:    typeStr,
				Target:  target,
				Columns: colInfos,
				Rows:    result,
			})

		case sqltype.Write:
			result, err := rtr.Exec(r.Context(), req.SQL, req.Params...)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, QueryResponse{
					OK:    false,
					Type:  typeStr,
					Error: err.Error(),
				})
				return
			}
			affected, _ := result.RowsAffected()
			lastID, _ := result.LastInsertId()
			writeJSON(w, http.StatusOK, QueryResponse{
				OK:           true,
				Type:         typeStr,
				Target:       target,
				RowsAffected: affected,
				LastInsertID: lastID,
			})

		default:
			writeJSON(w, http.StatusBadRequest, QueryResponse{
				OK:    false,
				Type:  typeStr,
				Error: "cannot determine SQL type (read/write)",
			})
		}
	}
}

func Tx(rtr *router.Router) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, TxResponse{
				OK:    false,
				Error: "method not allowed, use POST",
			})
			return
		}

		var req TxRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, TxResponse{
				OK:    false,
				Error: "invalid request body: " + err.Error(),
			})
			return
		}

		if len(req.Statements) == 0 {
			writeJSON(w, http.StatusBadRequest, TxResponse{
				OK:    false,
				Error: "at least one statement is required",
			})
			return
		}

		tx, err := rtr.BeginTx(r.Context(), nil)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, TxResponse{
				OK:    false,
				Error: "begin transaction failed: " + err.Error(),
			})
			return
		}

		var results []TxResult
		success := false
		defer func() {
			if !success {
				tx.Rollback()
			}
		}()

		for i, stmt := range req.Statements {
			if stmt.SQL == "" {
				writeJSON(w, http.StatusBadRequest, TxResponse{
					OK:    false,
					Error: fmt.Sprintf("statement[%d]: sql is empty", i),
				})
				return
			}

			sqlType := sqltype.Classify(stmt.SQL)
			typeStr := typeString(sqlType)

			switch sqlType {
			case sqltype.Read:
				rows, err := tx.Query(r.Context(), stmt.SQL, stmt.Params...)
				if err != nil {
					writeJSON(w, http.StatusInternalServerError, TxResponse{
						OK:    false,
						Error: fmt.Sprintf("statement[%d]: %s", i, err.Error()),
					})
					return
				}
				colInfos, result, err := scanRows(rows)
				rows.Close()
				if err != nil {
					writeJSON(w, http.StatusInternalServerError, TxResponse{
						OK:    false,
						Error: fmt.Sprintf("statement[%d]: %s", i, err.Error()),
					})
					return
				}
				results = append(results, TxResult{
					Type:    typeStr,
					Columns: colInfos,
					Rows:    result,
				})

			case sqltype.Write:
				res, err := tx.Exec(r.Context(), stmt.SQL, stmt.Params...)
				if err != nil {
					writeJSON(w, http.StatusInternalServerError, TxResponse{
						OK:    false,
						Error: fmt.Sprintf("statement[%d]: %s", i, err.Error()),
					})
					return
				}
				affected, _ := res.RowsAffected()
				lastID, _ := res.LastInsertId()
				results = append(results, TxResult{
					Type:         typeStr,
					RowsAffected: affected,
					LastInsertID: lastID,
				})

			default:
				writeJSON(w, http.StatusBadRequest, TxResponse{
					OK:    false,
					Error: fmt.Sprintf("statement[%d]: cannot determine SQL type", i),
				})
				return
			}
		}

		if err := tx.Commit(); err != nil {
			writeJSON(w, http.StatusInternalServerError, TxResponse{
				OK:    false,
				Error: "commit failed: " + err.Error(),
			})
			return
		}
		success = true

		writeJSON(w, http.StatusOK, TxResponse{
			OK:      true,
			Target:  "master",
			Results: results,
		})
	}
}

func scanRows(rows *sql.Rows) ([]ColumnInfo, []map[string]any, error) {
	cols, err := rows.ColumnTypes()
	if err != nil {
		return nil, nil, err
	}

	colInfos := make([]ColumnInfo, len(cols))
	for i, c := range cols {
		colInfos[i] = ColumnInfo{Name: c.Name(), Type: c.DatabaseTypeName()}
	}

	var result []map[string]any
	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, nil, err
		}
		row := make(map[string]any)
		for i, c := range colInfos {
			row[c.Name] = values[i]
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return colInfos, result, nil
}

func typeString(t sqltype.SQLType) string {
	switch t {
	case sqltype.Read:
		return "read"
	case sqltype.Write:
		return "write"
	default:
		return "unknown"
	}
}

func Stats(coll *stats.Collector) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		topN := 10
		if v := r.URL.Query().Get("top"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				topN = n
			}
		}

		if r.URL.Query().Get("reset") == "1" {
			coll.Reset()
			writeJSON(w, http.StatusOK, map[string]any{
				"ok":      true,
				"message": "stats reset",
			})
			return
		}

		if v := r.URL.Query().Get("slow_threshold"); v != "" {
			if d, err := strconv.Atoi(v); err == nil && d > 0 {
				coll.SetSlowThreshold(time.Duration(d) * time.Millisecond)
			}
		}

		summary := coll.Summary(topN)
		writeJSON(w, http.StatusOK, summary)
	}
}

func targetString(t sqltype.SQLType) string {
	switch t {
	case sqltype.Read:
		return "slave"
	case sqltype.Write:
		return "master"
	default:
		return "unknown"
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
