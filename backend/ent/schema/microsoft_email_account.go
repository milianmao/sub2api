// Package schema 定义 Ent ORM 的数据库 schema。
package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// MicrosoftEmailAccount 定义 Microsoft 邮箱账号实体的 schema。
type MicrosoftEmailAccount struct {
	ent.Schema
}

// Annotations 返回 schema 的注解配置。
func (MicrosoftEmailAccount) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "microsoft_email_accounts"},
	}
}

// Mixin 返回该 schema 使用的混入组件。
func (MicrosoftEmailAccount) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

// Fields 定义 Microsoft 邮箱账号实体的所有字段。
func (MicrosoftEmailAccount) Fields() []ent.Field {
	return []ent.Field{
		field.String("email").
			MaxLen(255).
			NotEmpty(),
		field.String("password").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			NotEmpty(),
		field.String("client_id").
			MaxLen(255).
			NotEmpty(),
		field.String("refresh_token").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			NotEmpty(),
		field.String("status").
			MaxLen(20).
			Default("active"),
		field.Time("last_check_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("last_fetch_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.String("last_error").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.String("notes").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
	}
}

// Indexes 定义 Microsoft 邮箱账号实体的索引。
func (MicrosoftEmailAccount) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("email").Unique(),
		index.Fields("status"),
		index.Fields("last_check_at"),
		index.Fields("last_fetch_at"),
	}
}
