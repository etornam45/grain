package main

import (
	"context"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib" // -> db.Open("pgx", dsn)

	//   _ "github.com/lib/pq"                -> db.Open("postgres", dsn)

	"github.com/etornam45/grain/db"
	"github.com/etornam45/grain/examples/basic/schema"
	"github.com/etornam45/grain/query"
)

type User struct {
	ID     string `db:"users.id"`
	Name   string `db:"users.name"`
	Email  string `db:"users.email"`
	Status string `db:"users.status"`
}

type UserOrderRow struct {
	Name  string `db:"users.name"`
	Total string `db:"orders.total"`
}

func main() {
	conn, err := db.Open("pgx", "postgres://user:pass@localhost:5432/app?sslmode=disable")
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	ctx := context.Background()
	var newID string
	err = query.Insert(schema.Users).
		Values(map[string]any{
			"name":   "Ama",
			"email":  "ama@example.com",
			"status": "active",
		}).
		Returning(schema.Users.Cols.ID.String()).
		Scan(ctx, conn, &newID)
	if err != nil {
		panic(err)
	}
	fmt.Println("inserted user:", newID)

	users, err := query.Select[User](schema.Users.Cols.ID, schema.Users.Cols.Name, schema.Users.Cols.Email, schema.Users.Cols.Status).
		From(schema.Users).
		Where(query.Eq(schema.Users.Cols.Status, "active")).
		OrderBy([]string {schema.Users.Cols.Name.Str()}, query.Asc).
		Limit(10).
		All(ctx, conn)
	if err != nil {
		panic(err)
	}
	fmt.Println("active users:", users)

	rows, err := query.Select[UserOrderRow](schema.Users.Cols.Name, schema.Orders.Cols.Total).
		From(schema.Users).
		InnerJoin(schema.Orders, query.Eq(schema.Users.Cols.ID, schema.Orders.Cols.UserID)).
		Where(query.Gt(schema.Orders.Cols.Total, 100)).
		All(ctx, conn)
	if err != nil {
		panic(err)
	}
	fmt.Println("big orders:", rows)

	err = conn.Transaction(ctx, func(tx *db.Tx) error {
		_, err := query.Delete(schema.Orders).Where(query.Eq(schema.Orders.Cols.UserID, newID)).Run(ctx, tx)
		if err != nil {
			return err
		}
		_, err = query.Delete(schema.Users).Where(query.Eq(schema.Users.Cols.ID, newID)).Run(ctx, tx)
		return err
	})
	if err != nil {
		panic(err)
	}
}
