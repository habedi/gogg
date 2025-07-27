package operations_test

import (
	"testing"

	"github.com/habedi/gogg/db"
	"github.com/habedi/gogg/pkg/operations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupTestDB(t *testing.T) {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		// Disable logger for cleaner test output
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	db.Db = gormDB
	require.NoError(t, db.Db.AutoMigrate(&db.Game{}))
}

func TestEstimateGameSize(t *testing.T) {
	setupTestDB(t)

	// This JSON structure mimics the GOG API format that your custom unmarshaller expects.
	rawJSONData := `
	{
		"title": "Test Sizer Game",
		"downloads": [
			["English", { "windows": [{"name": "setup.exe", "size": "1.5 GB"}], "linux": [{"name": "game.sh", "size": "1.4 GB"}] }],
			["German", { "windows": [{"name": "setup_de.exe", "size": "1.5 GB"}] }]
		],
		"dlcs": [
			{
				"title": "Test DLC",
				"downloads": [
					["English", { "windows": [{"name": "dlc.exe", "size": "512 MB"}] }]
				]
			}
		]
	}`

	require.NoError(t, db.PutInGame(123, "Test Sizer Game", rawJSONData))

	t.Run("Windows with DLC", func(t *testing.T) {
		params := operations.EstimationParams{
			LanguageCode:  "en",
			PlatformName:  "windows",
			IncludeExtras: false,
			IncludeDLCs:   true,
		}
		size, _, err := operations.EstimateGameSize(123, params)
		require.NoError(t, err)
		// 1.5GB (1.5 * 1024^3) + 512MB (512 * 1024^2) = 2147483648 bytes
		assert.Equal(t, int64(2147483648), size)
	})

	t.Run("Windows without DLC", func(t *testing.T) {
		params := operations.EstimationParams{
			LanguageCode:  "en",
			PlatformName:  "windows",
			IncludeExtras: false,
			IncludeDLCs:   false,
		}
		size, _, err := operations.EstimateGameSize(123, params)
		require.NoError(t, err)
		// 1.5GB (1.5 * 1024^3) = 1610612736 bytes
		assert.Equal(t, int64(1610612736), size)
	})

	t.Run("Game not found", func(t *testing.T) {
		_, _, err := operations.EstimateGameSize(999, operations.EstimationParams{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "game with ID 999 not found")
	})
}
