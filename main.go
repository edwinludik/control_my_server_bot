package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
)

type Config struct {
	Token   string
	OwnerID int64
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

	return &Config{
		Token:   token,
		OwnerID: ownerID,
	}, nil
}

func main() {
	// Load .env file if it exists
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, relying on environment variables")
	}

	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	bot, err := tgbotapi.NewBotAPI(cfg.Token)
	if err != nil {
		log.Fatalf("failed to create bot: %v", err)
	}

	log.Printf("Authorized on account %s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil { // ignore any non-Message updates
			continue
		}

		if !update.Message.IsCommand() { // ignore any non-command Messages
			continue
		}

		if update.Message.From.ID != cfg.OwnerID {
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Access Denied.")
			bot.Send(msg)
			continue
		}

		handleCommand(bot, update.Message)
	}
}

func handleCommand(bot *tgbotapi.BotAPI, msg *tgbotapi.Message) {
	chatID := msg.Chat.ID
	command := msg.Command()
	args := msg.CommandArguments()

	switch command {
	case "start":
		reply := tgbotapi.NewMessage(chatID, "Welcome! Use /restart_server, /restart_service <name>, or /status to control your server.")
		bot.Send(reply)

	case "restart_server":
		bot.Send(tgbotapi.NewMessage(chatID, "Restarting server..."))
		cmd := exec.Command("sudo", "reboot")
		if err := cmd.Run(); err != nil {
			bot.Send(tgbotapi.NewMessage(chatID, fmt.Sprintf("Failed to restart server: %v", err)))
		}

	case "restart_service":
		serviceName := strings.TrimSpace(args)
		if serviceName == "" {
			bot.Send(tgbotapi.NewMessage(chatID, "Please provide a service name. Usage: /restart_service <name>"))
			return
		}

		bot.Send(tgbotapi.NewMessage(chatID, fmt.Sprintf("Restarting service: %s...", serviceName)))
		cmd := exec.Command("sudo", "systemctl", "restart", serviceName)
		if err := cmd.Run(); err != nil {
			bot.Send(tgbotapi.NewMessage(chatID, fmt.Sprintf("Failed to restart service %s: %v", serviceName, err)))
		} else {
			bot.Send(tgbotapi.NewMessage(chatID, fmt.Sprintf("Service %s restarted successfully.", serviceName)))
		}

	case "status":
		uptime, _ := exec.Command("uptime", "-p").Output()
		bot.Send(tgbotapi.NewMessage(chatID, fmt.Sprintf("Server Status:\nUptime: %s", string(uptime))))

	default:
		bot.Send(tgbotapi.NewMessage(chatID, "I don't know that command"))
	}
}
