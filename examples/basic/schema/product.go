package schema

import "github.com/etornam45/grain/schema"

type ProductColums struct {
	Id, Name, Price, Sku, Barcode *schema.ColumnDef
}

var Product = schema.Table[ProductColums]("product",
	schema.Column("id", schema.UUID()).PrimaryKey().NotNull(),
	schema.Column("name", schema.Varchar(225)).NotNull().Index(),
	schema.Column("price", schema.Numeric(10, 2)).NotNull(),
	schema.Column("sku", schema.Varchar(15)).NotNull().Unique(),
	schema.Column("barcode", schema.Int()).Index(),
).
Index("idx_product_name_sku", "name", "sku")