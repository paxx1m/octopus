package migrate

import (
	"fmt"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/looplj/axonhub/llm"
	"gorm.io/gorm"
)

func init() {
	RegisterBeforeAutoMigration(Migration{
		Version: 3,
		Up:      migrateChannelTypeToAxonhub,
	})
	RegisterAfterAutoMigration(Migration{
		Version: 4,
		Up:      addChannelKeyModeAndWeight,
	})
}

// 003: 将旧 transformer 的数字渠道类型迁移为 axonhub/llm 使用的字符串类型。
func migrateChannelTypeToAxonhub(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}
	if !db.Migrator().HasTable("channels") || !db.Migrator().HasColumn("channels", "type") {
		return nil
	}

	switch db.Dialector.Name() {
	case "mysql":
		if err := db.Exec("ALTER TABLE `channels` MODIFY COLUMN `type` varchar(191)").Error; err != nil {
			return fmt.Errorf("failed to alter channels.type: %w", err)
		}
	case "postgres":
		if err := db.Exec(`ALTER TABLE "channels" ALTER COLUMN "type" TYPE text USING "type"::text`).Error; err != nil {
			return fmt.Errorf("failed to alter channels.type: %w", err)
		}
	}

	typeExpr := `CAST("type" AS TEXT)`
	typeColumn := `"type"`
	switch db.Dialector.Name() {
	case "mysql":
		typeExpr = "CAST(`type` AS CHAR)"
		typeColumn = "`type`"
	case "postgres":
		typeExpr = `"type"::text`
	}

	if err := db.Exec(fmt.Sprintf(`
UPDATE channels
SET %s = CASE %s
	WHEN '0' THEN ?
	WHEN '1' THEN ?
	WHEN '2' THEN ?
	WHEN '3' THEN ?
	WHEN '4' THEN ?
	WHEN '5' THEN ?
	ELSE %s
END
`, typeColumn, typeExpr, typeColumn), llm.APIFormatOpenAIChatCompletion.String(),
		llm.APIFormatOpenAIResponse.String(),
		llm.APIFormatAnthropicMessage.String(),
		llm.APIFormatGeminiContents.String(),
		model.ChannelTypeDoubao.String(),
		llm.APIFormatOpenAIEmbedding.String()).Error; err != nil {
		return fmt.Errorf("failed to migrate channels.type: %w", err)
	}
	return nil
}

// 004: add channels.key_mode and channel_keys.weight columns
func addChannelKeyModeAndWeight(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}

	dialect := db.Dialector.Name()

	hasColumn := func(table, column string) bool {
		switch dialect {
		case "sqlite":
			var name string
			db.Raw("SELECT name FROM pragma_table_info(?) WHERE name = ? LIMIT 1", table, column).Scan(&name)
			return name == column
		case "mysql":
			var count int64
			db.Raw("SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?", table, column).Scan(&count)
			return count > 0
		case "postgres":
			var count int64
			db.Raw("SELECT COUNT(*) FROM information_schema.columns WHERE table_name = ? AND column_name = ?", table, column).Scan(&count)
			return count > 0
		default:
			return db.Migrator().HasColumn(table, column)
		}
	}

	if !hasColumn("channels", "key_mode") {
		var sql string
		switch dialect {
		case "sqlite":
			sql = "ALTER TABLE channels ADD COLUMN key_mode INTEGER DEFAULT 0"
		case "mysql":
			sql = "ALTER TABLE channels ADD COLUMN key_mode INT DEFAULT 0"
		case "postgres":
			sql = "ALTER TABLE channels ADD COLUMN key_mode INTEGER DEFAULT 0"
		default:
			sql = "ALTER TABLE channels ADD COLUMN key_mode INTEGER DEFAULT 0"
		}
		if err := db.Exec(sql).Error; err != nil {
			return fmt.Errorf("failed to add channels.key_mode: %w", err)
		}
	}

	if !hasColumn("channel_keys", "weight") {
		var sql string
		switch dialect {
		case "sqlite":
			sql = "ALTER TABLE channel_keys ADD COLUMN weight INTEGER DEFAULT 1"
		case "mysql":
			sql = "ALTER TABLE channel_keys ADD COLUMN weight INT DEFAULT 1"
		case "postgres":
			sql = "ALTER TABLE channel_keys ADD COLUMN weight INTEGER DEFAULT 1"
		default:
			sql = "ALTER TABLE channel_keys ADD COLUMN weight INTEGER DEFAULT 1"
		}
		if err := db.Exec(sql).Error; err != nil {
			return fmt.Errorf("failed to add channel_keys.weight: %w", err)
		}
	}

	return nil
}
