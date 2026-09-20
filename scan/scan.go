package scan

import (
	"database/sql"
	"fmt"
	"reflect"
	"strings"
)

// **All** scans every row into a []T, matching each result column to a struct
// field via its `db:"table.column"` tag (or `db:"column"` for non-joined
// queries). Embedded structs are flattened; a `db:"-"` tag skips the field;
// pointer fields are allocated and set when their column is non-null.
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

// collectFields walks a struct (and embedded structs) collecting every
// db-tagged field. Embedded fields flatten into the parent; field paths with
// duplicate tags across the hierarchy are reported as errors.
func collectFields(typ reflect.Type, v reflect.Value, out map[string]reflect.Value) error {
	for i := 0; i < typ.NumField(); i++ {
		sf := typ.Field(i)
		field := v.Field(i)

		embedded := sf.Anonymous && (sf.Type.Kind() == reflect.Struct ||
			(sf.Type.Kind() == reflect.Pointer && sf.Type.Elem().Kind() == reflect.Struct))
		if embedded {
			ef := field
			var etype reflect.Type
			if sf.Type.Kind() == reflect.Pointer {
				if ef.IsNil() {
					// Unexported embedded types aren't settable via reflection;
					// skip those (their fields can't be written either).
					if !ef.CanSet() {
						continue
					}
					ef.Set(reflect.New(sf.Type.Elem()))
				}
				ef = ef.Elem()
				etype = sf.Type.Elem()
			} else {
				etype = sf.Type
			}
			if err := collectFields(etype, ef, out); err != nil {
				return err
			}
			continue
		}

		tag := sf.Tag.Get("db")
		if tag == "" || tag == "-" {
			continue
		}
		if idx := strings.IndexByte(tag, ','); idx != -1 {
			tag = tag[:idx]
		}
		if _, dup := out[tag]; dup {
			return fmt.Errorf("grain: scan: duplicate db tag %q on %s", tag, typ.Name())
		}
		out[tag] = field
	}
	return nil
}

func destinations(t any, cols []string) ([]any, error) {
	v := reflect.ValueOf(t).Elem()
	typ := v.Type()

	// T may itself be a pointer (e.g. All[*User]); allocate and deref so field
	// collection can walk the underlying struct.
	for typ.Kind() == reflect.Pointer {
		if v.IsNil() {
			v.Set(reflect.New(typ.Elem()))
		}
		v = v.Elem()
		typ = v.Type()
	}

	fieldByTag := map[string]reflect.Value{}
	if err := collectFields(typ, v, fieldByTag); err != nil {
		return nil, err
	}

	ptrs := make([]any, len(cols))
	for i, col := range cols {
		field, ok := fieldByTag[col]
		if !ok {
			// Fall back to matching by bare column name, so a query result
			// column "name" still matches a `db:"users.name"` tag. Ambiguous
			// matches (multiple tags ending in .name) are an error.
			var matches []reflect.Value
			for tag, f := range fieldByTag {
				if tag == col || strings.HasSuffix(tag, "."+col) {
					matches = append(matches, f)
				}
			}
			if len(matches) != 1 {
				if len(matches) > 1 {
					return nil, fmt.Errorf("grain: scan: column %q is ambiguous on %s", col, typ.Name())
				}
				return nil, fmt.Errorf("grain: scan: no destination field for column %q on %s", col, typ.Name())
			}
			field = matches[0]
		}
		ptrs[i] = field.Addr().Interface()
	}
	return ptrs, nil
}