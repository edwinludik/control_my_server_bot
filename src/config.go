package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Token              string
	OwnerID            int64
	LogChannelID       int64
	ControlledServices []string
}

func loadConfig() (*Config, error) {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN environment variable not set")
	}

	ownerIDStr := os.Getenv("TELEGRAM_OWNER_ID")
	if ownerIDStr == "" {
		return nil, fmt.Errorf("TELEGRAM_OWNER_ID environment variable not set")
	}

	ownerID, err := strconv.ParseInt(ownerIDStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid TELEGRAM_OWNER_ID: %v", err)
	}

	logChannelIDStr := os.Getenv("TELEGRAM_LOG_CHANNEL_ID")
	if logChannelIDStr == "" {
		return nil, fmt.Errorf("TELEGRAM_LOG_CHANNEL_ID environment variable not set")
	}

	logChannelID, err := strconv.ParseInt(logChannelIDStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid TELEGRAM_LOG_CHANNEL_ID: %v", err)
	}

	controlledServicesStr := os.Getenv("CONTROLLED_SERVICES")
	var controlledServices []string
	if controlledServicesStr != "" {
		services := strings.Split(controlledServicesStr, ",")
		for _, s := range services {
			trimmed := strings.TrimSpace(s)
			if trimmed != "" {
				controlledServices = append(controlledServices, trimmed)
			}
		}
	}

	// Ensure .env has restricted permissions if it exists
	if _, err := os.Stat(".env"); err == nil {
		_ = os.Chmod(".env", 0600)
	}

	return &Config{
		Token:              token,
		OwnerID:            ownerID,
		LogChannelID:       logChannelID,
		ControlledServices: controlledServices,
	}, nil
}
