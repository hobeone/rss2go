package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoad(t *testing.T) {
	t.Run("DefaultNoFileSucceeds", func(t *testing.T) {
		cfg, err := Load("")
		assert.NoError(t, err)
		assert.NotNil(t, cfg)
		assert.Equal(t, "rss2go.db", cfg.DBPath)
	})

	t.Run("ExplicitMissingFileFails", func(t *testing.T) {
		_, err := Load("nonexistent_config.yaml")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "error reading config file nonexistent_config.yaml")
	})

	t.Run("ExistingFileSucceeds", func(t *testing.T) {
		content := `
db_path: "overridden.db"
log_level: "debug"
`
		err := os.WriteFile("test_config.yaml", []byte(content), 0644)
		assert.NoError(t, err)
		defer os.Remove("test_config.yaml")

		cfg, err := Load("test_config.yaml")
		assert.NoError(t, err)
		assert.Equal(t, "overridden.db", cfg.DBPath)
		assert.Equal(t, "debug", cfg.LogLevel)
	})
}
