package query

import (
	"fmt"
	"strings"
)


type Subquery struct {
	sql  string
	args []any
}

// Sub captures a SelectBuilder as a reusable Subquery. The inner generic row
// type is never scanned into
func Sub[T any](b *SelectBuilder[T]) Subquery {
	s, args := b.SQL()
	return Subquery{sql: s, args: args}
}

// As renders the subquery as a column reference with the given alias, e.g.
// `(SELECT ...) AS total`. Only usable for subqueries without bound arguments:
// placeholder values can't be threaded through the outer select list yet.
func (s Subquery) As(alias string) ColRef {
	if len(s.args) > 0 {
		panic("grain: Subquery.As: subselect with bound arguments cannot be used as a column; " +
			"wrap it in query.Raw instead")
	}
	return ColRef{name: fmt.Sprintf("(%s) AS %s", s.sql, alias)}
}

// sqlAt renumbers the subquery's placeholders to start at argOffset.
func (s Subquery) sqlAt(argOffset int) string {
	return renumberPlaceholders(s.sql, argOffset)
}

// renumberPlaceholders rewrites every placeholder marker in sql to a
// sequential index starting at `start`. Both `$N` and `?` are treated as bind
// markers and consume one argument in order of appearance. Markers inside
// single-quoted string literals, double-quoted identifiers and dollar-quoted
// strings are left untouched, so a literal `'$5'` (e.g. a price) is not
// mistaken for a placeholder.
func renumberPlaceholders(sql string, start int) string {
	var b strings.Builder
	idx := start
	var inSingle, inDouble bool
	var inDollar string
	for i := 0; i < len(sql); i++ {
		ch := sql[i]
		switch {
		case inSingle, inDouble:
			b.WriteByte(ch)
			if ch == '"' && inDouble || ch == '\'' && inSingle {
				if i+1 < len(sql) && sql[i+1] == ch {
					b.WriteByte(sql[i+1])
					i++
				} else if inSingle {
					inSingle = false
				} else {
					inDouble = false
				}
			}
		case inDollar != "":
			if strings.HasPrefix(sql[i:], inDollar) {
				b.WriteString(inDollar)
				i += len(inDollar) - 1
				inDollar = ""
			} else {
				b.WriteByte(ch)
			}
		case ch == '\'':
			inSingle = true
			b.WriteByte(ch)
		case ch == '"':
			inDouble = true
			b.WriteByte(ch)
		case ch == '$':
			if tag, ok := dollarQuoteTag(sql, i); ok {
				b.WriteString(tag)
				i += len(tag) - 1
				inDollar = tag
			} else if j := i + 1; j < len(sql) && isDigit(sql[j]) {
				for j < len(sql) && isDigit(sql[j]) {
					j++
				}
				fmt.Fprintf(&b, "$%d", idx)
				idx++
				i = j - 1
			} else {
				b.WriteByte(ch)
			}
		case ch == '?':
			fmt.Fprintf(&b, "$%d", idx)
			idx++
		default:
			b.WriteByte(ch)
		}
	}
	return b.String()
}

// dollarQuoteTag reports whether sql[i] starts a dollar-quoted string and
// returns its delimiter (e.g. "$$" or "$tag$").
func dollarQuoteTag(sql string, i int) (string, bool) {
	if i >= len(sql) || sql[i] != '$' {
		return "", false
	}
	j := i + 1
	for j < len(sql) && (isIdentByte(sql[j])) {
		j++
	}
	if j < len(sql) && sql[j] == '$' {
		return sql[i : j+1], true
	}
	return "", false
}

func isDigit(ch byte) bool  { return ch >= '0' && ch <= '9' }
func isIdentByte(ch byte) bool {
	return ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || isDigit(ch) || ch == '_'
}