package schema

import "fmt"

type ColumnType struct {
	SQLType    string
	IsEnum     bool
	EnumName   string
	EnumValues []string
}

func Serial() *ColumnType      { return &ColumnType{SQLType: "SERIAL"} }
func BigSerial() *ColumnType   { return &ColumnType{SQLType: "BIGSERIAL"} }
func Int() *ColumnType         { return &ColumnType{SQLType: "INTEGER"} }
func BigInt() *ColumnType      { return &ColumnType{SQLType: "BIGINT"} }
func Bool() *ColumnType        { return &ColumnType{SQLType: "BOOLEAN"} }
func Text() *ColumnType        { return &ColumnType{SQLType: "TEXT"} }
func UUID() *ColumnType        { return &ColumnType{SQLType: "UUID"} }
func Timestamp() *ColumnType   { return &ColumnType{SQLType: "TIMESTAMP"} }
func TimestampTZ() *ColumnType { return &ColumnType{SQLType: "TIMESTAMPTZ"} }
func JSONB() *ColumnType       { return &ColumnType{SQLType: "JSONB"} }

func Varchar(length int) *ColumnType {
	return &ColumnType{SQLType: fmt.Sprintf("VARCHAR(%d)", length)}
}

func Numeric(precision, scale int) *ColumnType {
	return &ColumnType{SQLType: fmt.Sprintf("NUMERIC(%d,%d)", precision, scale)}
}

func Enum(name string, values ...string) *ColumnType {
	return &ColumnType{SQLType: name, IsEnum: true, EnumName: name, EnumValues: values}
}

type ReferentialAction string

const (
	Cascade  ReferentialAction = "CASCADE"
	SetNull  ReferentialAction = "SET NULL"
	Restrict ReferentialAction = "RESTRICT"
	NoAction ReferentialAction = "NO ACTION"
)
