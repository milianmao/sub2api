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

// CardMailboxCredential 定义卡密邮箱凭据实体的 schema。
type CardMailboxCredential struct {
	ent.Schema
}

// Annotations 返回 schema 的注解配置。
func (CardMailboxCredential) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "card_mailbox_credentials"},
	}
}

// Mixin 返回该 schema 使用的混入组件。
func (CardMailboxCredential) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

// Fields 定义卡密邮箱凭据实体的所有字段。
func (CardMailboxCredential) Fields() []ent.Field {
	return []ent.Field{
		field.String("email").
			MaxLen(255).
			NotEmpty(),
		field.String("mailbox_url").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			NotEmpty(),
		field.String("raw_json").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			NotEmpty(),
		field.String("last_code").
			Optional().
			Nillable().
			MaxLen(64),
		field.String("last_status").
			Optional().
			Nillable().
			MaxLen(20),
		field.String("last_error").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.Time("last_fetched_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

// Indexes 定义卡密邮箱凭据实体的索引。
func (CardMailboxCredential) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("email").Unique(),
		index.Fields("last_status"),
		index.Fields("last_fetched_at"),
	}
}
