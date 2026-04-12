package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
)

func main() {
	// Load .env file from the current directory if it exists
	if err := godotenv.Load(".env"); err != nil {
		if !os.IsNotExist(err) {
			log.Printf("Failed to load .env file: %v", err)
		} else {
			log.Println("No .env file found in current directory, relying on environment variables")
		}
	}

	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	userStore, err := NewUserStore()
	if err != nil {
		log.Fatalf("failed to initialize user store: %v", err)
	}
	defer func() {
		_ = userStore.Close()
	}()

	bot, err := tgbotapi.NewBotAPI(cfg.Token)
	if err != nil {
		log.Fatalf("failed to create bot: %v", err)
	}

	logger := NewTelegramLogger(bot, cfg.LogChannelID)

	logger.Printf("🚀 Bot started and authorized as @%s", bot.Self.UserName)

	limiter := NewRateLimiter(5, time.Minute) // 5 commands per minute per user

	// Log available services on start
	services, err := getAvailableServices(cfg)
	if err != nil {
		logger.Printf("⚠️ Failed to get available services on start: %v", err)
	} else if len(services) > 0 {
		logger.Printf("📋 Available Services on startup:\n• %s", strings.Join(services, "\n• "))
	} else {
		logger.Printf("ℹ️ No available services found on startup.")
	}

	u := tgbotapi.NewUpdate(loadOffset())
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		saveOffset(update.UpdateID + 1)
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("Recovered from panic in update loop: %v", r)
				}
			}()

			if update.Message != nil {
				if !update.Message.IsCommand() { // ignore any non-command Messages
					return
				}

				if update.Message.From == nil {
					return
				}
				userID := update.Message.From.ID

				// Rate limiting
				if allowed, cooldown := limiter.Allow(userID); !allowed {
					logger.Printf("Rate limit exceeded for User ID: %d (Cooldown: %v)", userID, cooldown.Round(time.Second))
					// Send message with cooldown time
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Too many requests. Please wait %v.", cooldown.Round(time.Second)))
					if _, err := bot.Send(msg); err != nil {
						log.Printf("Failed to send rate limit message to User %d: %v", userID, err)
					}
					return
				}

				// Check authorization: Owner or exists in userStore
				if userID != cfg.OwnerID {
					authorized, err := userStore.UserExists(userID)
					if err != nil {
						logger.Printf("Error checking authorization for %d: %v", userID, err)
					}
					if !authorized {
						msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Access Denied.")
						if _, err := bot.Send(msg); err != nil {
							log.Printf("Failed to send access denied message to User %d: %v", userID, err)
						}
						logger.Printf("Unauthorized access attempt from User ID: %d", userID)
						return
					}
				}

				handleCommand(bot, update.Message, logger, cfg, userStore)
			} else if update.CallbackQuery != nil {
				if update.CallbackQuery.From == nil {
					return
				}
				userID := update.CallbackQuery.From.ID

				// Rate limiting for callbacks too
				if allowed, cooldown := limiter.Allow(userID); !allowed {
					logger.Printf("Rate limit exceeded for Callback from User ID: %d (Cooldown: %v)", userID, cooldown.Round(time.Second))
					callback := tgbotapi.NewCallback(update.CallbackQuery.ID, fmt.Sprintf("Too many requests. Please wait %v.", cooldown.Round(time.Second)))
					if _, err := bot.Request(callback); err != nil {
						log.Printf("Failed to send rate limit callback answer to User %d: %v", userID, err)
					}
					return
				}

				// Check authorization for callbacks
				if userID != cfg.OwnerID {
					authorized, err := userStore.UserExists(userID)
					if err != nil {
						logger.Printf("Error checking authorization for %d: %v", userID, err)
					}
					if !authorized {
						callback := tgbotapi.NewCallback(update.CallbackQuery.ID, "Access Denied.")
						if _, err := bot.Request(callback); err != nil {
							log.Printf("Failed to send access denied callback answer to User %d: %v", userID, err)
						}
						logger.Printf("Unauthorized callback attempt from User ID: %d", userID)
						return
					}
				}

				handleCallback(bot, update.CallbackQuery, logger, cfg)
			}
		}()
	}
}

const offsetFile = ".session_offset"

func loadOffset() int {
	data, err := os.ReadFile(offsetFile)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("Failed to read offset file: %v", err)
		}
		return 0
	}
	offset, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		log.Printf("Failed to parse offset from file: %v", err)
		return 0
	}
	return offset
}

func saveOffset(offset int) {
	err := os.WriteFile(offsetFile, []byte(strconv.Itoa(offset)), 0600)
	if err != nil {
		log.Printf("Failed to save offset to file: %v", err)
	}
}
