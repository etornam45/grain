package query

import (
	"fmt"
	"regexp"
)


type Subquery struct {
	sql  string
	args []any
}

// Sub captures a SelectBuilder as a reusable Subquery. The inner generic row
// type is never scanned into — only the SQL/args are used.
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

var placeholderRe = regexp.MustCompile(`\$\d+`)

// renumberPlaceholders rewrites sequential $N placeholders starting at start.
// All Grain-generated SQL numbers placeholders left-to-right from $1, so this
// is safe for anything produced by this package.
func renumberPlaceholders(sql string, start int) string {
	idx := start
	return placeholderRe.ReplaceAllStringFunc(sql, func(string) string {
		p := fmt.Sprintf("$%d", idx)
		idx++
		return p
	})
}