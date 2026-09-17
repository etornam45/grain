package migrate

type ChangeKind string

const (
	CreateTable     ChangeKind = "create_table"
	DropTable       ChangeKind = "drop_table"
	RenameTable     ChangeKind = "rename_table"
	AddColumn       ChangeKind = "add_column"
	DropColumn      ChangeKind = "drop_column"
	RenameColumn    ChangeKind = "rename_column"
	AlterColumnType ChangeKind = "alter_column_type"
	CreateEnum      ChangeKind = "create_enum"
	AddEnumValue    ChangeKind = "add_enum_value"
)

type Change struct {
	Kind    ChangeKind
	UpSQL   string
	DownSQL string

	// Destructive means this change can lose data in a way no amount of user
	// confirmation resolves — e.g. a column type or a NOT NULL column with no default.
	Destructive bool

	// RequiresNoTx means this statement cannot run inside the transaction
	// Apply() wraps each migration file in — e.g. ALTER TYPE ... ADD VALUE
	// FIXME: Run in different transaction (create both up and down section for -- +RequiresNoTx up and -- + RequiresNoTx down )
	RequiresNoTx bool
	Note         string
}
