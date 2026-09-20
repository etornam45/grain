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

	AlterColumnIdentity    ChangeKind = "alter_column_identity"
	AlterColumnDefault     ChangeKind = "alter_column_default"
	AlterColumnNullability ChangeKind = "alter_column_nullability"
	AlterColumnGenerated   ChangeKind = "alter_column_generated"
	AlterColumnCheck       ChangeKind = "alter_column_check"
	AddColumnUnique        ChangeKind = "add_column_unique"
	DropColumnUnique       ChangeKind = "drop_column_unique"
	AddPrimaryKey          ChangeKind = "add_primary_key"
	DropPrimaryKey         ChangeKind = "drop_primary_key"
	AddForeignKey          ChangeKind = "add_foreign_key"
	DropForeignKey         ChangeKind = "drop_foreign_key"
	AddUniqueConstraint    ChangeKind = "add_unique_constraint"
	DropUniqueConstraint   ChangeKind = "drop_unique_constraint"
	AddConstraint          ChangeKind = "add_constraint"
	DropConstraint         ChangeKind = "drop_constraint"

	CreateEnum   ChangeKind = "create_enum"
	DropEnum     ChangeKind = "drop_enum"
	RenameEnum   ChangeKind = "rename_enum"
	AddEnumValue ChangeKind = "add_enum_value"
	AddIndex     ChangeKind = "add_index"
	DropIndex    ChangeKind = "drop_index"
)

type Change struct {
	Kind    ChangeKind
	UpSQL   string
	DownSQL string

	// Destructive means this change can lose data in a way no amount of user
	// confirmation resolves — e.g. a column type or a NOT NULL column with no default.
	Destructive bool

	// RequiresNoTx means this statement cannot run inside the transaction
	// Apply() wraps each migration file in — e.g. ALTER TYPE ... ADD VALUE.
	// The generate step emits these into dedicated `-- +RequiresNoTx Up|Down`
	// sections that the apply step runs outside the migration transaction.
	RequiresNoTx bool
	Note         string
}
