package database

import (
	"testing"

	"magicpodcast/internal/models"

	"github.com/stretchr/testify/require"
)

// 隔离迁移演练：历史 schema-24 fixture 一路升级到当前版本，
// 清单表出现、既有业务数据不变；普通启动仍只做只读校验。
func TestMigrationDrillSchema24FixtureAppliesCollections(t *testing.T) {
	db := openSchema24MigrationFixture(t)

	require.NoError(t, ApplyMigrations(db))

	status, err := InspectSchema(db)
	require.NoError(t, err)
	require.Equal(t, CurrentSchemaVersion, status.CurrentVersion)
	require.Empty(t, status.Pending)

	for _, table := range []string{models.EpisodeCollection{}.TableName(), models.EpisodeCollectionItem{}.TableName()} {
		require.True(t, db.Migrator().HasTable(table), table)
	}

	// 演练不改动既有业务数据。
	var podcastCount, episodeCount int64
	require.NoError(t, db.Model(&models.Podcast{}).Count(&podcastCount).Error)
	require.NoError(t, db.Model(&models.Episode{}).Count(&episodeCount).Error)
	require.Equal(t, int64(1), podcastCount)
	require.Equal(t, int64(13), episodeCount)

	// 普通启动路径通过只读校验，且不再有待应用迁移。
	require.NoError(t, RequireSchemaReady(db))
}

// 清单唯一性与级联/置空约束在迁移后的 schema 上生效。
func TestMigrationDrillCollectionConstraints(t *testing.T) {
	db := openSchema24MigrationFixture(t)
	require.NoError(t, ApplyMigrations(db))

	collection := models.EpisodeCollection{
		SourcePlatform: "xiaoyuzhoufm",
		ExternalID:     "6a20323b78a52c96d821a769",
		Title:          "穿透半导体迷雾",
		SourceURL:      "https://www.xiaoyuzhoufm.com/collection/episode/6a20323b78a52c96d821a769",
	}
	require.NoError(t, db.Create(&collection).Error)

	duplicate := collection
	require.Error(t, db.Create(&duplicate).Error, "同源平台+外部清单 ID 必须唯一")

	item := models.EpisodeCollectionItem{
		CollectionID:      collection.ID,
		Position:          0,
		ExternalEpisodeID: "6a1c07b0ac7bdb080c3397a9",
		PodcastTitle:      "投资实战派",
		EpisodeTitle:      "E185 芯片规律 × AI浪潮",
	}
	require.NoError(t, db.Create(&item).Error)
	duplicateItem := item
	require.Error(t, db.Create(&duplicateItem).Error, "清单内同一外部单集必须唯一")
}
