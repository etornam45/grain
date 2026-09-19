package schema

import "github.com/etornam45/grain/schema"

var UserStatus = schema.Enum("user_status", "active", "suspended", "banned")

type usersColumns struct {
	ID, Name, Email, Age, Status *schema.ColumnDef
}

var Users = schema.Table("users",
	schema.Column("id", schema.UUID()).PrimaryKey().Default("gen_random_uuid()"),
	schema.Column("name", schema.Varchar(255)).NotNull(),
	schema.Column("email", schema.Varchar(255)).NotNull().Unique(),
	schema.Column("age", schema.Int()),
	schema.Column("status", UserStatus).NotNull().Default("active"),
)

type ordersColumns struct {
	ID, UserID, Total *schema.ColumnDef
}

var Orders = schema.Table("orders",
	schema.Column("id", schema.Serial()).PrimaryKey(),
	schema.Column("user_id", schema.UUID()).
		NotNull().
		References(Users.Col("id")).
		OnDelete(schema.Cascade),
	schema.Column("total", schema.Numeric(10, 2)).NotNull(),
)
