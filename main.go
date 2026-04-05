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
	//goland:noinspection GoUnhandledErrorResult
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

	if err := rows.Err(); err != nil {
		return nil, err
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
	//goland:noinspection GoUnhandledErrorResult
	defer userStore.Close()

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
	} else {
		if len(services) > 0 {
			logger.Printf("📋 Available Services on startup:\n• %s", strings.Join(services, "\n• "))
		} else {
			logger.Printf("ℹ️ No available services found on startup.")
		}
	}

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("Recovered from panic in update loop: %v", r)
				}
			}()

			if update.Message == nil { // ignore any non-Message updates
				return
			}

			if !update.Message.IsCommand() { // ignore any non-command Messages
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
		}()
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

func (rl *RateLimiter) Allow(userID int64) (bool, time.Duration) {
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
		// The cooldown ends when the oldest timestamp in 'current' falls out of the window
		cooldown := time.Until(current[0].Add(rl.interval))
		if cooldown < 0 {
			cooldown = 0
		}
		return false, cooldown
	}

	rl.counts[userID] = append(current, now)
	return true, 0
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
	if _, err := l.bot.Send(tgMsg); err != nil {
		log.Printf("Failed to send log message to Telegram: %v (original message: %s)", err, msg)
	}
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

	helpText := "🤖 *Available Commands:*\n" +
		"• /ping — Return \"Pong!\"\n" +
		"• /status — Check server status, RAM, CPU, and disk space\n" +
		"• /get\\_cpu\\_usage — Show current CPU usage\n" +
		"• /get\\_ram\\_usage — Show current RAM usage\n" +
		"• /get\\_disk\\_usage — Show free disk space on all drives\n" +
		"• /get\\_services — List available services\n" +
		"• /restart\\_service <name> — Restart a service\n" +
		"• /restart\\_server — Reboot the server"

	if isOwner {
		helpText += "\n\n🔑 *Owner Commands:*\n" +
			"• /add\\_user <id> — Add an authorized user\n" +
			"• /delete\\_user <id> — Remove a user\n" +
			"• /get\\_users — List all authorized users"
	}

	switch command {
	case "ping":
		if _, err := bot.Send(tgbotapi.NewMessage(chatID, "🏓 Pong!")); err != nil {
			log.Printf("Failed to send ping response: %v", err)
		}

	case "start", "help":
		msg := tgbotapi.NewMessage(chatID, helpText)
		msg.ParseMode = tgbotapi.ModeMarkdown
		if _, err := bot.Send(msg); err != nil {
			log.Printf("Failed to send help message: %v", err)
		}

	case "restart_server":
		if !isAuthorized {
			if _, err := bot.Send(tgbotapi.NewMessage(chatID, "🚫 Permission denied.")); err != nil {
				log.Printf("Failed to send permission denied message: %v", err)
			}
			return
		}
		logger.Printf("🔄 Restarting server requested by chat %d (User %d)", chatID, userID)
		if _, err := bot.Send(tgbotapi.NewMessage(chatID, "🔄 Restarting server...")); err != nil {
			log.Printf("Failed to send restarting server message: %v", err)
		}
		cmd := exec.Command("sudo", "reboot")
		if err := cmd.Run(); err != nil {
			logger.Printf("❌ Failed to restart server: %v", err)
			if _, err := bot.Send(tgbotapi.NewMessage(chatID, "❌ Failed to restart server. See logs for details.")); err != nil {
				log.Printf("Failed to send restart failure message: %v", err)
			}
		}

	case "restart_service":
		if !isAuthorized {
			if _, err := bot.Send(tgbotapi.NewMessage(chatID, "🚫 Permission denied.")); err != nil {
				log.Printf("Failed to send permission denied message: %v", err)
			}
			return
		}
		serviceName := strings.TrimSpace(args)
		if serviceName == "" {
			if _, err := bot.Send(tgbotapi.NewMessage(chatID, "ℹ️ Please provide a service name.\nUsage: `/restart_service <name>`")); err != nil {
				log.Printf("Failed to send usage message: %v", err)
			}
			return
		}

		if !serviceNameRegex.MatchString(serviceName) {
			if _, err := bot.Send(tgbotapi.NewMessage(chatID, "❌ Invalid service name format.")); err != nil {
				log.Printf("Failed to send invalid service name message: %v", err)
			}
			logger.Printf("⚠️ Invalid service name attempt: %s", serviceName)
			return
		}

		logger.Printf("🔄 Restarting service %s requested by chat %d (User %d)", serviceName, chatID, userID)

		if len(cfg.ControlledServices) > 0 && !slices.Contains(cfg.ControlledServices, serviceName) {
			if _, err := bot.Send(tgbotapi.NewMessage(chatID, "🚫 Service is not in the controlled list.")); err != nil {
				log.Printf("Failed to send service not controlled message: %v", err)
			}
			logger.Printf("⚠️ Unauthorized attempt to restart service: %s", serviceName)
			return
		}

		if _, err := bot.Send(tgbotapi.NewMessage(chatID, fmt.Sprintf("🔄 Restarting service: %s...", serviceName))); err != nil {
			log.Printf("Failed to send restarting service message: %v", err)
		}
		cmd := exec.Command("sudo", "systemctl", "restart", serviceName)
		if err := cmd.Run(); err != nil {
			logger.Printf("❌ Failed to restart service %s: %v", serviceName, err)
			if _, err := bot.Send(tgbotapi.NewMessage(chatID, "❌ Failed to restart service. See logs for details.")); err != nil {
				log.Printf("Failed to send restart failure message: %v", err)
			}
		} else {
			successStr := fmt.Sprintf("✅ Service %s restarted successfully.", serviceName)
			if _, err := bot.Send(tgbotapi.NewMessage(chatID, successStr)); err != nil {
				log.Printf("Failed to send success message: %v", err)
			}
			logger.Printf(successStr)
		}

	case "get_services":
		if !isAuthorized {
			if _, err := bot.Send(tgbotapi.NewMessage(chatID, "🚫 Permission denied.")); err != nil {
				log.Printf("Failed to send permission denied message: %v", err)
			}
			return
		}
		services, err := getAvailableServices(cfg)
		if err != nil {
			if _, err := bot.Send(tgbotapi.NewMessage(chatID, "❌ Failed to list services: "+err.Error())); err != nil {
				log.Printf("Failed to send list services failure message: %v", err)
			}
			return
		}

		if len(services) == 0 {
			if _, err := bot.Send(tgbotapi.NewMessage(chatID, "ℹ️ No services found.")); err != nil {
				log.Printf("Failed to send no services found message: %v", err)
			}
		} else {
			msg := tgbotapi.NewMessage(chatID, "📋 *Available Services:*\n• "+strings.Join(services, "\n• "))
			msg.ParseMode = tgbotapi.ModeMarkdown
			if _, err := bot.Send(msg); err != nil {
				log.Printf("Failed to send services list message: %v", err)
			}
		}

	case "status":
		if !isAuthorized {
			if _, err := bot.Send(tgbotapi.NewMessage(chatID, "🚫 Permission denied.")); err != nil {
				log.Printf("Failed to send permission denied message: %v", err)
			}
			return
		}
		uptime, err := exec.Command("uptime", "-p").Output()
		if err != nil {
			uptime, _ = exec.Command("uptime").Output()
		}
		cpuUsage, _ := getCPUUsageInfo()
		ramUsage, _ := getRAMUsageInfo()
		diskInfo, _ := getDiskSpaceInfo()
		statusMsg := fmt.Sprintf("🖥 *Server Status*\n\n*Uptime:* %s\n\n*CPU Usage:*\n%s\n\n*RAM Usage:*\n%s\n\n*Disk Space:*\n%s",
			strings.TrimSpace(string(uptime)), cpuUsage, ramUsage, diskInfo)
		msg := tgbotapi.NewMessage(chatID, statusMsg)
		msg.ParseMode = tgbotapi.ModeMarkdown
		if _, err := bot.Send(msg); err != nil {
			log.Printf("Failed to send status message: %v", err)
		}

	case "get_cpu_usage":
		if !isAuthorized {
			if _, err := bot.Send(tgbotapi.NewMessage(chatID, "🚫 Permission denied.")); err != nil {
				log.Printf("Failed to send permission denied message: %v", err)
			}
			return
		}
		cpuUsage, err := getCPUUsageInfo()
		if err != nil {
			if _, err := bot.Send(tgbotapi.NewMessage(chatID, "❌ Failed to get CPU usage: "+err.Error())); err != nil {
				log.Printf("Failed to send get CPU usage failure message: %v", err)
			}
			return
		}
		msg := tgbotapi.NewMessage(chatID, "📊 *CPU Usage:*\n"+cpuUsage)
		msg.ParseMode = tgbotapi.ModeMarkdown
		if _, err := bot.Send(msg); err != nil {
			log.Printf("Failed to send CPU usage message: %v", err)
		}

	case "get_ram_usage":
		if !isAuthorized {
			if _, err := bot.Send(tgbotapi.NewMessage(chatID, "🚫 Permission denied.")); err != nil {
				log.Printf("Failed to send permission denied message: %v", err)
			}
			return
		}
		ramUsage, err := getRAMUsageInfo()
		if err != nil {
			if _, err := bot.Send(tgbotapi.NewMessage(chatID, "❌ Failed to get RAM usage: "+err.Error())); err != nil {
				log.Printf("Failed to send get RAM usage failure message: %v", err)
			}
			return
		}
		msg := tgbotapi.NewMessage(chatID, "💾 *RAM Usage:*\n"+ramUsage)
		msg.ParseMode = tgbotapi.ModeMarkdown
		if _, err := bot.Send(msg); err != nil {
			log.Printf("Failed to send RAM usage message: %v", err)
		}

	case "get_disk_usage":
		if !isAuthorized {
			if _, err := bot.Send(tgbotapi.NewMessage(chatID, "🚫 Permission denied.")); err != nil {
				log.Printf("Failed to send permission denied message: %v", err)
			}
			return
		}
		diskInfo, err := getDiskSpaceInfo()
		if err != nil {
			if _, err := bot.Send(tgbotapi.NewMessage(chatID, "❌ Failed to get disk space: "+err.Error())); err != nil {
				log.Printf("Failed to send get disk space failure message: %v", err)
			}
			return
		}
		msg := tgbotapi.NewMessage(chatID, "💽 *Free Disk Space:*\n"+diskInfo)
		msg.ParseMode = tgbotapi.ModeMarkdown
		if _, err := bot.Send(msg); err != nil {
			log.Printf("Failed to send disk space message: %v", err)
		}

	case "add_user":
		if !isOwner {
			if _, err := bot.Send(tgbotapi.NewMessage(chatID, "🚫 Only the owner can add users.")); err != nil {
				log.Printf("Failed to send permission denied message: %v", err)
			}
			return
		}
		parts := strings.Fields(args)
		if len(parts) < 1 {
			if _, err := bot.Send(tgbotapi.NewMessage(chatID, "ℹ️ Usage: `/add_user <id>`")); err != nil {
				log.Printf("Failed to send usage message: %v", err)
			}
			return
		}
		id, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			if _, err := bot.Send(tgbotapi.NewMessage(chatID, "❌ Invalid user ID.")); err != nil {
				log.Printf("Failed to send invalid user ID message: %v", err)
			}
			return
		}
		if err := userStore.AddUser(id); err != nil {
			if _, err := bot.Send(tgbotapi.NewMessage(chatID, "❌ Failed to add user: "+err.Error())); err != nil {
				log.Printf("Failed to send add user failure message: %v", err)
			}
		} else {
			successStr := fmt.Sprintf("✅ User %d added with full permissions.", id)
			if _, err := bot.Send(tgbotapi.NewMessage(chatID, successStr)); err != nil {
				log.Printf("Failed to send user added message: %v", err)
			}
			logger.Printf("👤 User %d added with full permissions by owner", id)
		}

	case "delete_user":
		if !isOwner {
			if _, err := bot.Send(tgbotapi.NewMessage(chatID, "🚫 Only the owner can delete users.")); err != nil {
				log.Printf("Failed to send permission denied message: %v", err)
			}
			return
		}
		id, err := strconv.ParseInt(strings.TrimSpace(args), 10, 64)
		if err != nil {
			if _, err := bot.Send(tgbotapi.NewMessage(chatID, "ℹ️ Usage: `/delete_user <id>`")); err != nil {
				log.Printf("Failed to send usage message: %v", err)
			}
			return
		}
		if err := userStore.DeleteUser(id); err != nil {
			if _, err := bot.Send(tgbotapi.NewMessage(chatID, "❌ Failed to delete user: "+err.Error())); err != nil {
				log.Printf("Failed to send delete user failure message: %v", err)
			}
		} else {
			successStr := fmt.Sprintf("✅ User %d deleted.", id)
			if _, err := bot.Send(tgbotapi.NewMessage(chatID, successStr)); err != nil {
				log.Printf("Failed to send user deleted message: %v", err)
			}
			logger.Printf("👤 User %d deleted by owner", id)
		}

	case "get_users":
		if !isOwner {
			if _, err := bot.Send(tgbotapi.NewMessage(chatID, "🚫 Only the owner can list users.")); err != nil {
				log.Printf("Failed to send permission denied message: %v", err)
			}
			return
		}
		users, err := userStore.ListUsers(cfg.OwnerID)
		if err != nil {
			if _, err := bot.Send(tgbotapi.NewMessage(chatID, "❌ Failed to list users: "+err.Error())); err != nil {
				log.Printf("Failed to send list users failure message: %v", err)
			}
			return
		}
		if len(users) == 0 {
			if _, err := bot.Send(tgbotapi.NewMessage(chatID, "ℹ️ No authorized users found.")); err != nil {
				log.Printf("Failed to send no users found message: %v", err)
			}
		} else {
			msg := tgbotapi.NewMessage(chatID, "👥 *Authorized Users:*\n• "+strings.Join(users, "\n• "))
			msg.ParseMode = tgbotapi.ModeMarkdown
			if _, err := bot.Send(msg); err != nil {
				log.Printf("Failed to send users list message: %v", err)
			}
		}

	default:
		if _, err := bot.Send(tgbotapi.NewMessage(chatID, "❓ I don't know that command. Type /help for a list of available commands.")); err != nil {
			log.Printf("Failed to send unknown command message: %v", err)
		}
	}
}

func getCPUUsageInfo() (string, error) {
	// Using top -bn1 | grep "Cpu(s)" for Linux
	out, err := exec.Command("sh", "-c", "top -bn1 | grep \"Cpu(s)\"").Output()
	if err != nil {
		// Fallback for macOS: top -l 1 -n 0 | grep "CPU usage"
		out, err = exec.Command("sh", "-c", "top -l 1 -n 0 | grep \"CPU usage\"").Output()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(out)), nil
	}
	return strings.TrimSpace(string(out)), nil
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
