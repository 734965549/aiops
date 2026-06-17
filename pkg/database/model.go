package database

import (
	"time"

	"gorm.io/gorm"
)

// BaseModel 是平台所有持久化模型必须嵌入的基类，体现统一的建表规范：
//
//   - ID：自增主键（PostgreSQL BIGSERIAL），与业务标识严格分离；
//     业务上对外暴露的「用户 ID / 资源 ID」等应作为单独的列（带唯一索引）。
//   - CreatedAt / UpdatedAt：在程序内通过 GORM 钩子设置，
//     不使用数据库 DEFAULT、不依赖触发器，保证多副本与跨数据库迁移时行为一致。
//
// 任何新建模型只需 `embedding` BaseModel 即可获得统一的主键与时间戳行为：
//
//	type AlertModel struct {
//	    database.BaseModel
//	    AlertID string `gorm:"column:alert_id;type:varchar(36);uniqueIndex"`
//	    ...
//	}
//
// 注意：BeforeCreate/BeforeUpdate 钩子只在 Create / Save / Updates / Update 等
// 走 GORM Statement 的写入路径触发；如确需使用 db.Exec 直写 SQL，请在
// 业务代码中显式维护 created_at / updated_at。
type BaseModel struct {
	ID        int64     `gorm:"column:id;primaryKey;autoIncrement" json:"-"`
	CreatedAt time.Time `gorm:"column:created_at;not null" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null" json:"updated_at"`
}

// BeforeCreate 在 INSERT 前由 GORM 调用，确保创建/更新时间在程序内被赋值。
//
// 如果上层显式传入了 CreatedAt（例如数据迁移脚本），则保留原值，
// 仅补齐 UpdatedAt，便于补数据时不破坏历史时间线。
func (m *BaseModel) BeforeCreate(_ *gorm.DB) error {
	now := time.Now()
	if m.CreatedAt.IsZero() {
		m.CreatedAt = now
	}
	if m.UpdatedAt.IsZero() {
		m.UpdatedAt = now
	}
	return nil
}

// BeforeUpdate 在 UPDATE 前由 GORM 调用，永远刷新 UpdatedAt。
//
// 注意：仅当通过 Save / Update / Updates 等走 GORM Hook 的方法更新时生效；
// 调用 UpdateColumn / UpdateColumns 会跳过 Hook，业务侧应避免在主流程中使用。
func (m *BaseModel) BeforeUpdate(_ *gorm.DB) error {
	m.UpdatedAt = time.Now()
	return nil
}
