package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

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

	// Ensure .env has restricted permissions if it exists
	if _, err := os.Stat(".env"); err == nil {
		_ = os.Chmod(".env", 0600)
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

func NewUserStore(dbPath string) (store *UserStore, err error) {
	// Use DSN with WAL mode and busy timeout for better SQLite performance and reliability
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Defer database closure on error
	defer func() {
		if err != nil {
			_ = db.Close()
		}
	}()

	// Verify connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	// Ensure the database file has restricted permissions
	_ = os.Chmod(dbPath, 0600)

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY
	)`)
	if err != nil {
		return nil, fmt.Errorf("create users table: %w", err)
	}

	return &UserStore{db: db}, nil
}

func (s *UserStore) AddUser(id int64) error {
	_, err := s.db.Exec("INSERT OR REPLACE INTO users (id) VALUES (?)", id)
	return err
}

func (s *UserStore) DeleteUser(id int64) error {
	_, err := s.db.Exec("DELETE FROM users WHERE id = ?", id)
	return err
}

func (s *UserStore) UserExists(id int64) (bool, error) {
	var exists bool
	err := s.db.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE id = ?)", id).Scan(&exists)
	return exists, err
}

func (s *UserStore) ListUsers(ownerID int64) ([]string, error) {
	rows, err := s.db.Query("SELECT id FROM users")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []string
	result = append(result, fmt.Sprintf("User: %d (Owner, cannot be deleted)", ownerID))

	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		result = append(result, fmt.Sprintf("User: %d", id))
	}
	return result, nil
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

	limiter := NewRateLimiter(5, time.Minute) // 5 commands per minute per user

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

		userID := update.Message.From.ID

		// Rate limiting
		if !limiter.Allow(userID) {
			logger.Printf("Rate limit exceeded for User ID: %d", userID)
			// Silent ignore or send message? Let's send a quiet one.
			// msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Too many requests. Please wait.")
			// bot.Send(msg)
			continue
		}

		// Check authorization: Owner or exists in userStore
		if userID != cfg.OwnerID {
			authorized, err := userStore.UserExists(userID)
			if err != nil {
				logger.Printf("Error checking authorization for %d: %v", userID, err)
			}
			if !authorized {
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Access Denied.")
				bot.Send(msg)
				logger.Printf("Unauthorized access attempt from User ID: %d", userID)
				continue
			}
		}

		handleCommand(bot, update.Message, logger, cfg, userStore)
	}
}

type RateLimiter struct {
	mu       sync.Mutex
	counts   map[int64][]time.Time
	limit    int
	interval time.Duration
}

func NewRateLimiter(limit int, interval time.Duration) *RateLimiter {
	return &RateLimiter{
		counts:   make(map[int64][]time.Time),
		limit:    limit,
		interval: interval,
	}
}

func (rl *RateLimiter) Allow(userID int64) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.interval)

	// Filter out old timestamps
	var current []time.Time
	for _, t := range rl.counts[userID] {
		if t.After(cutoff) {
			current = append(current, t)
		}
	}

	if len(current) >= rl.limit {
		rl.counts[userID] = current
		return false
	}

	rl.counts[userID] = append(current, now)
	return true
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

var serviceNameRegex = regexp.MustCompile(`^[a-zA-Z0-9\-_.]+$`)

func handleCommand(bot *tgbotapi.BotAPI, msg *tgbotapi.Message, logger *TelegramLogger, cfg *Config, userStore *UserStore) {
	chatID := msg.Chat.ID
	userID := msg.From.ID
	command := msg.Command()
	args := msg.CommandArguments()

	isOwner := userID == cfg.OwnerID
	isAuthorized := isOwner
	if !isAuthorized {
		authorized, err := userStore.UserExists(userID)
		if err != nil {
			logger.Printf("Error checking authorization for %d: %v", userID, err)
		}
		isAuthorized = authorized
	}

	logger.Printf("Command received: /%s from chat %d (User %d)", command, chatID, userID)

	helpText := "Available commands:\n" +
		"/status - Check server status, RAM usage, and disk space\n" +
		"/get_ram_usage - Show current RAM usage\n" +
		"/get_disk_usage - Show free disk space on all drives\n" +
		"/list_services - List available services\n" +
		"/restart_service <name> - Restart a service\n" +
		"/restart_server - Reboot the server"

	if isOwner {
		helpText += "\n\nOwner commands:\n" +
			"/add_user <id> - Add an authorized user\n" +
			"/delete_user <id> - Remove a user\n" +
			"/list_users - List all authorized users"
	}

	switch command {
	case "start", "help":
		reply := tgbotapi.NewMessage(chatID, "Welcome! "+helpText)
		bot.Send(reply)

	case "restart_server":
		if !isAuthorized {
			bot.Send(tgbotapi.NewMessage(chatID, "Permission denied."))
			return
		}
		logger.Printf("Restarting server requested by chat %d", chatID)
		bot.Send(tgbotapi.NewMessage(chatID, "Restarting server..."))
		cmd := exec.Command("sudo", "reboot")
		if err := cmd.Run(); err != nil {
			logger.Printf("Failed to restart server: %v", err)
			bot.Send(tgbotapi.NewMessage(chatID, "Failed to restart server. See logs for details."))
		}

	case "restart_service":
		if !isAuthorized {
			bot.Send(tgbotapi.NewMessage(chatID, "Permission denied."))
			return
		}
		serviceName := strings.TrimSpace(args)
		if serviceName == "" {
			bot.Send(tgbotapi.NewMessage(chatID, "Please provide a service name. Usage: /restart_service <name>"))
			return
		}

		if !serviceNameRegex.MatchString(serviceName) {
			bot.Send(tgbotapi.NewMessage(chatID, "Invalid service name format."))
			logger.Printf("Invalid service name attempt: %s", serviceName)
			return
		}

		logger.Printf("Restarting service %s requested by chat %d", serviceName, chatID)

		if len(cfg.ControlledServices) > 0 && !slices.Contains(cfg.ControlledServices, serviceName) {
			bot.Send(tgbotapi.NewMessage(chatID, "Service is not in the controlled list."))
			logger.Printf("Unauthorized attempt to restart service: %s", serviceName)
			return
		}

		bot.Send(tgbotapi.NewMessage(chatID, fmt.Sprintf("Restarting service: %s...", serviceName)))
		cmd := exec.Command("sudo", "systemctl", "restart", serviceName)
		if err := cmd.Run(); err != nil {
			logger.Printf("Failed to restart service %s: %v", serviceName, err)
			bot.Send(tgbotapi.NewMessage(chatID, "Failed to restart service. See logs for details."))
		} else {
			successStr := fmt.Sprintf("Service %s restarted successfully.", serviceName)
			bot.Send(tgbotapi.NewMessage(chatID, successStr))
			logger.Printf(successStr)
		}

	case "list_services":
		if !isAuthorized {
			bot.Send(tgbotapi.NewMessage(chatID, "Permission denied."))
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
		if !isAuthorized {
			bot.Send(tgbotapi.NewMessage(chatID, "Permission denied."))
			return
		}
		uptime, err := exec.Command("uptime", "-p").Output()
		if err != nil {
			uptime, _ = exec.Command("uptime").Output()
		}
		ramUsage, _ := getRAMUsageInfo()
		diskInfo, _ := getDiskSpaceInfo()
		bot.Send(tgbotapi.NewMessage(chatID, fmt.Sprintf("Server Status:\nUptime: %s\n\nRAM Usage:\n%s\n\nDisk Space:\n%s", strings.TrimSpace(string(uptime)), ramUsage, diskInfo)))

	case "get_ram_usage":
		if !isAuthorized {
			bot.Send(tgbotapi.NewMessage(chatID, "Permission denied."))
			return
		}
		ramUsage, err := getRAMUsageInfo()
		if err != nil {
			bot.Send(tgbotapi.NewMessage(chatID, "Failed to get RAM usage: "+err.Error()))
			return
		}
		bot.Send(tgbotapi.NewMessage(chatID, "RAM Usage:\n"+ramUsage))

	case "get_disk_usage":
		if !isAuthorized {
			bot.Send(tgbotapi.NewMessage(chatID, "Permission denied."))
			return
		}
		diskInfo, err := getDiskSpaceInfo()
		if err != nil {
			bot.Send(tgbotapi.NewMessage(chatID, "Failed to get disk space: "+err.Error()))
			return
		}
		bot.Send(tgbotapi.NewMessage(chatID, "Free Disk Space:\n"+diskInfo))

	case "add_user":
		if !isOwner {
			bot.Send(tgbotapi.NewMessage(chatID, "Only the owner can add users."))
			return
		}
		parts := strings.Fields(args)
		if len(parts) < 1 {
			bot.Send(tgbotapi.NewMessage(chatID, "Usage: /add_user <id>"))
			return
		}
		id, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			bot.Send(tgbotapi.NewMessage(chatID, "Invalid user ID."))
			return
		}
		if err := userStore.AddUser(id); err != nil {
			bot.Send(tgbotapi.NewMessage(chatID, "Failed to add user: "+err.Error()))
		} else {
			bot.Send(tgbotapi.NewMessage(chatID, fmt.Sprintf("User %d added with full permissions.", id)))
			logger.Printf("User %d added with full permissions by owner", id)
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

	case "list_users":
		if !isOwner {
			bot.Send(tgbotapi.NewMessage(chatID, "Only the owner can list users."))
			return
		}
		users, err := userStore.ListUsers(cfg.OwnerID)
		if err != nil {
			bot.Send(tgbotapi.NewMessage(chatID, "Failed to list users: "+err.Error()))
			return
		}
		if len(users) == 0 {
			bot.Send(tgbotapi.NewMessage(chatID, "No authorized users found."))
		} else {
			bot.Send(tgbotapi.NewMessage(chatID, "Authorized Users:\n"+strings.Join(users, "\n")))
		}

	default:
		bot.Send(tgbotapi.NewMessage(chatID, "I don't know that command"))
	}
}

func getRAMUsageInfo() (string, error) {
	// Using free -h to get human-readable memory usage
	out, err := exec.Command("free", "-h").Output()
	if err != nil {
		// Fallback for macOS if free -h is not available
		out, err = exec.Command("top", "-l", "1", "-s", "0", "-n", "0").Output()
		if err != nil {
			return "", err
		}
		// Basic extraction for macOS top output
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "PhysMem:") {
				return line, nil
			}
		}
		return string(out), nil
	}
	return strings.TrimSpace(string(out)), nil
}

func getDiskSpaceInfo() (string, error) {
	// Using df -h to get human-readable disk space info on all mounted filesystems
	out, err := exec.Command("df", "-h").Output()
	if err != nil {
		return "", err
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return "No disk space information available.", nil
	}

	// Filter and format the output to be cleaner
	var result []string
	result = append(result, lines[0]) // Header

	for i := 1; i < len(lines); i++ {
		line := lines[i]
		// Skip temporary or virtual filesystems if they are too many
		if strings.HasPrefix(line, "tmpfs") || strings.HasPrefix(line, "devtmpfs") || strings.HasPrefix(line, "udev") {
			continue
		}
		result = append(result, line)
	}

	return strings.Join(result, "\n"), nil
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
