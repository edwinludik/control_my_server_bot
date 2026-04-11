package main

import (
	"os"
	"reflect"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	// Backup original env vars
	vars := []string{"TELEGRAM_BOT_TOKEN", "TELEGRAM_OWNER_ID", "TELEGRAM_LOG_CHANNEL_ID", "CONTROLLED_SERVICES"}
	original := make(map[string]string)
	for _, v := range vars {
		original[v] = os.Getenv(v)
	}
	defer func() {
		for k, v := range original {
			if v == "" {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, v)
			}
		}
	}()

	t.Run("ValidConfig", func(t *testing.T) {
		os.Setenv("TELEGRAM_BOT_TOKEN", "test_token")
		os.Setenv("TELEGRAM_OWNER_ID", "123456")
		os.Setenv("TELEGRAM_LOG_CHANNEL_ID", "-100123")
		os.Setenv("CONTROLLED_SERVICES", "nginx, docker,sshd ")

		cfg, err := loadConfig()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expected := &Config{
			Token:              "test_token",
			OwnerID:            123456,
			LogChannelID:       -100123,
			ControlledServices: []string{"nginx", "docker", "sshd"},
		}

		if !reflect.DeepEqual(cfg, expected) {
			t.Errorf("expected %+v, got %+v", expected, cfg)
		}
	})

	t.Run("MissingToken", func(t *testing.T) {
		os.Unsetenv("TELEGRAM_BOT_TOKEN")
		_, err := loadConfig()
		if err == nil {
			t.Error("expected error for missing TELEGRAM_BOT_TOKEN")
		}
	})

	t.Run("InvalidOwnerID", func(t *testing.T) {
		os.Setenv("TELEGRAM_BOT_TOKEN", "token")
		os.Setenv("TELEGRAM_OWNER_ID", "not_a_number")
		_, err := loadConfig()
		if err == nil {
			t.Error("expected error for invalid TELEGRAM_OWNER_ID")
		}
	})
}
