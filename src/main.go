package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
)

func main() {
	var shutdownReason string
	var shutdownRequester string
	var shutdownTime time.Time

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

	// Send start message to all admins
	admins := []int64{cfg.OwnerID}
	if additionalAdmins, err := userStore.ListUserIDs(); err == nil {
		admins = append(admins, additionalAdmins...)
	}
	for _, adminID := range admins {
		logger.SendMessage(adminID, "Server started")
	}

	logger.Printf("Bot started and authorized as @%s", bot.Self.UserName)

	// Log available services on start
	services, err := getAvailableServices(cfg)
	if err != nil {
		logger.Printf("⚠️ Failed to get available services on start: %v", err)
	} else if len(services) > 0 {
		logger.Printf("Available Services on startup:\n• %s", strings.Join(services, "\n• "))
	} else {
		logger.Printf("⚠️ No available services found on startup.")
	}

	u := tgbotapi.NewUpdate(loadOffset())
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case sig := <-sigChan:
			log.Printf("Received signal: %v. Shutting down...", sig)
			reason := "Process received signal " + sig.String()
			if shutdownReason != "" {
				reason = shutdownReason
				if shutdownRequester != "" {
					reason += fmt.Sprintf(" (Requested by %s at %s)", shutdownRequester, shutdownTime.Format("15:04:05"))
				}
			} else if sig == syscall.SIGTERM {
				if systemRequester := getSystemShutdownRequester(); systemRequester != "" {
					reason = "System shutdown initiated: " + systemRequester
				} else {
					reason = "System shutdown or service stop (SIGTERM)"
				}
			} else if sig == syscall.SIGINT {
				reason = "Manual interruption (SIGINT)"
			}

			for _, adminID := range admins {
				logger.SendMessage(adminID, "Server shutting down: "+reason)
			}
			return
		case update, ok := <-updates:
			if !ok {
				return
			}
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

					// Check authorization: Owner or exists in userStore
					if userID != cfg.OwnerID {
						authorized, err := userStore.UserExists(userID)
						if err != nil {
							logger.Printf("Error checking authorization for %d: %v", userID, err)
						}
						if !authorized {
							logger.SendMessage(update.Message.Chat.ID, "Access Denied.")
							logger.Printf("Unauthorized access attempt from %s (Chat %d)", formatUser(update.Message.From), update.Message.Chat.ID)
							return
						}
					}

					handleCommand(update.Message, logger, cfg, userStore)
				} else if update.CallbackQuery != nil {
					if update.CallbackQuery.From == nil {
						return
					}
					userID := update.CallbackQuery.From.ID

					// Check authorization for callbacks
					if userID != cfg.OwnerID {
						authorized, err := userStore.UserExists(userID)
						if err != nil {
							logger.Printf("Error checking authorization for %d: %v", userID, err)
						}
						if !authorized {
							logger.Request(tgbotapi.NewCallback(update.CallbackQuery.ID, "Access Denied."))
							logger.Printf("Unauthorized callback attempt from %s (Chat %d)", formatUser(update.CallbackQuery.From), update.CallbackQuery.Message.Chat.ID)
							return
						}
					}

					newReason, newRequester, newTime := handleCallback(bot, update.CallbackQuery, logger, cfg)
					if newReason != "" {
						shutdownReason = newReason
						shutdownRequester = newRequester
						shutdownTime = newTime
					}
				}
			}()
		}
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
