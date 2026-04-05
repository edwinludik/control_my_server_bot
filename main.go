package main

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
	_ "modernc.org/sqlite"
)

type Config struct {
	Token              string
	OwnerID            int64
	LogChannelID       int64
	ControlledServices []string
	DBPath             string
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

	dbPath := os.Getenv("DATABASE_PATH")
	if dbPath == "" {
		dbPath = ".user_ids"
	}

	return &Config{
		Token:              token,
		OwnerID:            ownerID,
		LogChannelID:       logChannelID,
		ControlledServices: controlledServices,
		DBPath:             dbPath,
	}, nil
}

type UserStore struct {
	db *sql.DB
}

func NewUserStore(dbPath string) (*UserStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY,
		permissions TEXT
	)`)
	if err != nil {
		return nil, err
	}

	return &UserStore{db: db}, nil
}

func (s *UserStore) AddUser(id int64, permissions string) error {
	_, err := s.db.Exec("INSERT OR REPLACE INTO users (id, permissions) VALUES (?, ?)", id, permissions)
	return err
}

func (s *UserStore) DeleteUser(id int64) error {
	_, err := s.db.Exec("DELETE FROM users WHERE id = ?", id)
	return err
}

func (s *UserStore) GetPermissions(id int64) (string, error) {
	var perms string
	err := s.db.QueryRow("SELECT permissions FROM users WHERE id = ?", id).Scan(&perms)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return perms, nil
}

func (s *UserStore) HasPermission(id int64, perm string) bool {
	perms, err := s.GetPermissions(id)
	if err != nil {
		return false
	}
	if perms == "*" {
		return true
	}
	return slices.Contains(strings.Split(perms, ","), perm)
}

func (s *UserStore) Close() error {
	return s.db.Close()
}

func main() {
	// Load .env file from the current directory if it exists
	if err := godotenv.Load(".env"); err != nil {
		log.Println("No .env file found in current directory, relying on environment variables")
	}

	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	userStore, err := NewUserStore(cfg.DBPath)
	if err != nil {
		log.Fatalf("failed to initialize user store: %v", err)
	}
	defer userStore.Close()

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

		// Check authorization: Owner or exists in userStore
		if update.Message.From.ID != cfg.OwnerID {
			perms, err := userStore.GetPermissions(update.Message.From.ID)
			if err != nil {
				logger.Printf("Error checking permissions for %d: %v", update.Message.From.ID, err)
			}
			if perms == "" {
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Access Denied.")
				bot.Send(msg)
				logger.Printf("Unauthorized access attempt from User ID: %d", update.Message.From.ID)
				continue
			}
		}

		handleCommand(bot, update.Message, logger, cfg, userStore)
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

func handleCommand(bot *tgbotapi.BotAPI, msg *tgbotapi.Message, logger *TelegramLogger, cfg *Config, userStore *UserStore) {
	chatID := msg.Chat.ID
	userID := msg.From.ID
	command := msg.Command()
	args := msg.CommandArguments()

	isOwner := userID == cfg.OwnerID
	hasPerm := func(p string) bool {
		return isOwner || userStore.HasPermission(userID, p)
	}

	logger.Printf("Command received: /%s from chat %d (User %d)", command, chatID, userID)

	switch command {
	case "start":
		help := "Welcome! Use /restart_server, /restart_service <name>, /list_services, or /status to control your server."
		if isOwner {
			help += "\n\nOwner commands:\n/add_user <id> <perms>\n/delete_user <id>\nPermissions can be '*' or comma-separated list of commands."
		}
		reply := tgbotapi.NewMessage(chatID, help)
		bot.Send(reply)

	case "restart_server":
		if !hasPerm("restart_server") {
			bot.Send(tgbotapi.NewMessage(chatID, "Permission denied: restart_server"))
			return
		}
		logger.Printf("Restarting server requested by chat %d", chatID)
		bot.Send(tgbotapi.NewMessage(chatID, "Restarting server..."))
		cmd := exec.Command("sudo", "reboot")
		if err := cmd.Run(); err != nil {
			errStr := fmt.Sprintf("Failed to restart server: %v", err)
			bot.Send(tgbotapi.NewMessage(chatID, errStr))
			logger.Printf(errStr)
		}

	case "restart_service":
		if !hasPerm("restart_service") {
			bot.Send(tgbotapi.NewMessage(chatID, "Permission denied: restart_service"))
			return
		}
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
		if !hasPerm("list_services") {
			bot.Send(tgbotapi.NewMessage(chatID, "Permission denied: list_services"))
			return
		}
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
		if !hasPerm("status") {
			bot.Send(tgbotapi.NewMessage(chatID, "Permission denied: status"))
			return
		}
		uptime, err := exec.Command("uptime", "-p").Output()
		if err != nil {
			uptime, _ = exec.Command("uptime").Output()
		}
		bot.Send(tgbotapi.NewMessage(chatID, fmt.Sprintf("Server Status:\nUptime: %s", strings.TrimSpace(string(uptime)))))

	case "add_user":
		if !isOwner {
			bot.Send(tgbotapi.NewMessage(chatID, "Only the owner can add users."))
			return
		}
		parts := strings.Fields(args)
		if len(parts) < 2 {
			bot.Send(tgbotapi.NewMessage(chatID, "Usage: /add_user <id> <permissions>"))
			return
		}
		id, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			bot.Send(tgbotapi.NewMessage(chatID, "Invalid user ID."))
			return
		}
		perms := parts[1]
		if err := userStore.AddUser(id, perms); err != nil {
			bot.Send(tgbotapi.NewMessage(chatID, "Failed to add user: "+err.Error()))
		} else {
			bot.Send(tgbotapi.NewMessage(chatID, fmt.Sprintf("User %d added with permissions: %s", id, perms)))
			logger.Printf("User %d added with permissions: %s by owner", id, perms)
		}

	case "delete_user":
		if !isOwner {
			bot.Send(tgbotapi.NewMessage(chatID, "Only the owner can delete users."))
			return
		}
		id, err := strconv.ParseInt(strings.TrimSpace(args), 10, 64)
		if err != nil {
			bot.Send(tgbotapi.NewMessage(chatID, "Usage: /delete_user <id>"))
			return
		}
		if err := userStore.DeleteUser(id); err != nil {
			bot.Send(tgbotapi.NewMessage(chatID, "Failed to delete user: "+err.Error()))
		} else {
			bot.Send(tgbotapi.NewMessage(chatID, fmt.Sprintf("User %d deleted.", id)))
			logger.Printf("User %d deleted by owner", id)
		}

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
