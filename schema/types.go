package schema

import "fmt"

type ColumnType struct {
	SQLType    string
	IsEnum     bool
	EnumName   string
	EnumValues []string
}

func Serial() *ColumnType       { return &ColumnType{SQLType: "SERIAL"} }
func SmallSerial() *ColumnType  { return &ColumnType{SQLType: "SMALLSERIAL"} }
func BigSerial() *ColumnType    { return &ColumnType{SQLType: "BIGSERIAL"} }
func SmallInt() *ColumnType     { return &ColumnType{SQLType: "SMALLINT"} }
func Int() *ColumnType          { return &ColumnType{SQLType: "INTEGER"} }
func BigInt() *ColumnType       { return &ColumnType{SQLType: "BIGINT"} }
func Real() *ColumnType         { return &ColumnType{SQLType: "REAL"} }
func Double() *ColumnType       { return &ColumnType{SQLType: "DOUBLE PRECISION"} }
func Bool() *ColumnType         { return &ColumnType{SQLType: "BOOLEAN"} }
func Text() *ColumnType         { return &ColumnType{SQLType: "TEXT"} }
func Bytea() *ColumnType        { return &ColumnType{SQLType: "BYTEA"} }
func UUID() *ColumnType         { return &ColumnType{SQLType: "UUID"} }
func Date() *ColumnType         { return &ColumnType{SQLType: "DATE"} }
func Time() *ColumnType         { return &ColumnType{SQLType: "TIME"} }
func Timetz() *ColumnType       { return &ColumnType{SQLType: "TIMETZ"} }
func Interval() *ColumnType     { return &ColumnType{SQLType: "INTERVAL"} }
func Timestamp() *ColumnType    { return &ColumnType{SQLType: "TIMESTAMP"} }
func TimestampTZ() *ColumnType  { return &ColumnType{SQLType: "TIMESTAMPTZ"} }
func JSONB() *ColumnType        { return &ColumnType{SQLType: "JSONB"} }
func Money() *ColumnType        { return &ColumnType{SQLType: "MONEY"} }

func Varchar(length int) *ColumnType {
	return &ColumnType{SQLType: fmt.Sprintf("VARCHAR(%d)", length)}
}

func Char(length int) *ColumnType {
	return &ColumnType{SQLType: fmt.Sprintf("CHAR(%d)", length)}
}

func Numeric(precision, scale int) *ColumnType {
	return &ColumnType{SQLType: fmt.Sprintf("NUMERIC(%d,%d)", precision, scale)}
}

// Array wraps the inner type as a PostgreSQL one-dimensional array,
// e.g. Array(Int()) renders as INTEGER[].
func Array(inner *ColumnType) *ColumnType {
	return &ColumnType{SQLType: inner.SQLType + "[]"}
}

var EnumRegistry = map[string]*ColumnType{}

func Enum(name string, values ...string) *ColumnType {
	ct := &ColumnType{SQLType: name, IsEnum: true, EnumName: name, EnumValues: values}
	EnumRegistry[name] = ct
	return ct
}

type ReferentialAction string

const (
	Cascade  ReferentialAction = "CASCADE"
	SetNull  ReferentialAction = "SET NULL"
	Restrict ReferentialAction = "RESTRICT"
	NoAction ReferentialAction = "NO ACTION"
)
