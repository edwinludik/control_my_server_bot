package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
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

	return &Config{
		Token:              token,
		OwnerID:            ownerID,
		LogChannelID:       logChannelID,
		ControlledServices: controlledServices,
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

	logger := NewTelegramLogger(bot, cfg.LogChannelID)

	logger.Printf("Authorized on account %s", bot.Self.UserName)

	// Log available services on start
	services, err := getAvailableServices(cfg)
	if err != nil {
		logger.Printf("Failed to get available services on start: %v", err)
	} else {
		if len(services) > 0 {
			logger.Printf("Available Services on startup:\n%s", strings.Join(services, "\n"))
		} else {
			logger.Printf("No available services found on startup.")
		}
	}

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
			logger.Printf("Unauthorized access attempt from User ID: %d", update.Message.From.ID)
			continue
		}

		handleCommand(bot, update.Message, logger, cfg)
	}
}

type TelegramLogger struct {
	bot       *tgbotapi.BotAPI
	channelID int64
}

func NewTelegramLogger(bot *tgbotapi.BotAPI, channelID int64) *TelegramLogger {
	return &TelegramLogger{
		bot:       bot,
		channelID: channelID,
	}
}

func (l *TelegramLogger) Printf(format string, v ...any) {
	msg := fmt.Sprintf(format, v...)
	log.Print(msg)
	tgMsg := tgbotapi.NewMessage(l.channelID, msg)
	_, _ = l.bot.Send(tgMsg)
}

func handleCommand(bot *tgbotapi.BotAPI, msg *tgbotapi.Message, logger *TelegramLogger, cfg *Config) {
	chatID := msg.Chat.ID
	command := msg.Command()
	args := msg.CommandArguments()

	logger.Printf("Command received: /%s from chat %d", command, chatID)

	switch command {
	case "start":
		reply := tgbotapi.NewMessage(chatID, "Welcome! Use /restart_server, /restart_service <name>, /list_services, or /status to control your server.")
		bot.Send(reply)

	case "restart_server":
		logger.Printf("Restarting server requested by chat %d", chatID)
		bot.Send(tgbotapi.NewMessage(chatID, "Restarting server..."))
		cmd := exec.Command("sudo", "reboot")
		if err := cmd.Run(); err != nil {
			errStr := fmt.Sprintf("Failed to restart server: %v", err)
			bot.Send(tgbotapi.NewMessage(chatID, errStr))
			logger.Printf(errStr)
		}

	case "restart_service":
		serviceName := strings.TrimSpace(args)
		if serviceName == "" {
			bot.Send(tgbotapi.NewMessage(chatID, "Please provide a service name. Usage: /restart_service <name>"))
			return
		}

		logger.Printf("Restarting service %s requested by chat %d", serviceName, chatID)

		if len(cfg.ControlledServices) > 0 && !slices.Contains(cfg.ControlledServices, serviceName) {
			bot.Send(tgbotapi.NewMessage(chatID, fmt.Sprintf("Service %s is not in the controlled list.", serviceName)))
			logger.Printf("Unauthorized attempt to restart service: %s", serviceName)
			return
		}

		bot.Send(tgbotapi.NewMessage(chatID, fmt.Sprintf("Restarting service: %s...", serviceName)))
		cmd := exec.Command("sudo", "systemctl", "restart", serviceName)
		if err := cmd.Run(); err != nil {
			errStr := fmt.Sprintf("Failed to restart service %s: %v", serviceName, err)
			bot.Send(tgbotapi.NewMessage(chatID, errStr))
			logger.Printf(errStr)
		} else {
			successStr := fmt.Sprintf("Service %s restarted successfully.", serviceName)
			bot.Send(tgbotapi.NewMessage(chatID, successStr))
			logger.Printf(successStr)
		}

	case "list_services":
		services, err := getAvailableServices(cfg)
		if err != nil {
			bot.Send(tgbotapi.NewMessage(chatID, "Failed to list services: "+err.Error()))
			return
		}

		if len(services) == 0 {
			bot.Send(tgbotapi.NewMessage(chatID, "No services found."))
		} else {
			bot.Send(tgbotapi.NewMessage(chatID, "Available Services:\n"+strings.Join(services, "\n")))
		}

	case "status":
		uptime, err := exec.Command("uptime", "-p").Output()
		if err != nil {
			uptime, _ = exec.Command("uptime").Output()
		}
		bot.Send(tgbotapi.NewMessage(chatID, fmt.Sprintf("Server Status:\nUptime: %s", strings.TrimSpace(string(uptime)))))

	default:
		bot.Send(tgbotapi.NewMessage(chatID, "I don't know that command"))
	}
}

func getAvailableServices(cfg *Config) ([]string, error) {
	if len(cfg.ControlledServices) > 0 {
		return cfg.ControlledServices, nil
	}

	// List all services if no specific list is provided
	out, err := exec.Command("systemctl", "list-units", "--type=service", "--state=running", "--no-legend").Output()
	if err != nil {
		return nil, err
	}

	var services []string
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) > 0 {
			// systemctl list-units output: UNIT LOAD ACTIVE SUB DESCRIPTION
			// Fields[0] is the service name
			services = append(services, fields[0])
		}
	}
	return services, nil
}
