package handler

import (
	"encoding/json"
	"net/http"

	"sqlrouter/router"
	"sqlrouter/sqltype"
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
	OK      bool         `json:"ok"`
	Type    string       `json:"type,omitempty"`
	Target  string       `json:"target,omitempty"`
	Columns []ColumnInfo `json:"columns,omitempty"`
	Rows    []map[any]any `json:"rows,omitempty"`
	RowsAffected int64   `json:"rows_affected,omitempty"`
	LastInsertID int64   `json:"last_insert_id,omitempty"`
	Error   string       `json:"error,omitempty"`
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

			cols, err := rows.ColumnTypes()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, QueryResponse{
					OK:    false,
					Type:  typeStr,
					Error: err.Error(),
				})
				return
			}

			colInfos := make([]ColumnInfo, len(cols))
			for i, c := range cols {
				colInfos[i] = ColumnInfo{Name: c.Name(), Type: c.DatabaseTypeName()}
			}

			var result []map[any]any
			for rows.Next() {
				values := make([]any, len(cols))
				ptrs := make([]any, len(cols))
				for i := range values {
					ptrs[i] = &values[i]
				}
				if err := rows.Scan(ptrs...); err != nil {
					writeJSON(w, http.StatusInternalServerError, QueryResponse{
						OK:    false,
						Type:  typeStr,
						Error: err.Error(),
					})
					return
				}
				row := make(map[any]any)
				for i, c := range colInfos {
					row[c.Name] = values[i]
				}
				result = append(result, row)
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
