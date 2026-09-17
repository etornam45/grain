package scan

import (
	"database/sql"
	"fmt"
	"reflect"
	"strings"
)

// **All** scans every row into a []T, matching each result column to a struct
// field via its `db:"table.column"` tag (or `db:"column"` for non-joined
// queries).
func All[T any](rows *sql.Rows) ([]T, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var out []T
	for rows.Next() {
		var t T
		ptrs, err := destinations(&t, cols)
		if err != nil {
			return nil, err
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// One scans a single row into a T. Returns sql.ErrNoRows if there were none —
// callers (e.g. SelectBuilder.First) typically translate that into (nil, nil).
func One[T any](row *sql.Row, cols []string) (T, error) {
	var t T
	ptrs, err := destinations(&t, cols)
	if err != nil {
		return t, err
	}
	return t, row.Scan(ptrs...)
}

func destinations(t any, cols []string) ([]any, error) {
	v := reflect.ValueOf(t).Elem()
	typ := v.Type()

	fieldByTag := map[string]reflect.Value{}
	for i := 0; i < typ.NumField(); i++ {
		tag := typ.Field(i).Tag.Get("db")
		if tag == "" {
			continue
		}
		fieldByTag[tag] = v.Field(i)
	}

	ptrs := make([]any, len(cols))
	for i, col := range cols {
		field, ok := fieldByTag[col]
		if !ok {
			// Fall back to matching by bare column name, so a query result
			// column "name" still matches a `db:"users.name"` tag.
			for tag, f := range fieldByTag {
				if tag == col || strings.HasSuffix(tag, "."+col) {
					field, ok = f, true
					break
				}
			}
		}
		if !ok {
			return nil, fmt.Errorf("grain: scan: no destination field for column %q on %s", col, typ.Name())
		}
		ptrs[i] = field.Addr().Interface()
	}
	return ptrs, nil
}
