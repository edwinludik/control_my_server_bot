package main

import (
	"fmt"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var serviceNameRegex = regexp.MustCompile(`^[a-zA-Z0-9\-_.]+$`)

func isServiceAllowed(serviceName string, cfg *Config) bool {
	// 1. Basic regex check to prevent command injection
	if !serviceNameRegex.MatchString(serviceName) {
		return false
	}
	// 2. Check if it's in the whitelist (if any)
	if len(cfg.ControlledServices) > 0 {
		return slices.Contains(cfg.ControlledServices, serviceName)
	}
	// 3. Final check: Does it exist in the system?
	available, err := getAvailableServices(cfg)
	if err != nil {
		return false
	}
	return slices.Contains(available, serviceName)
}

func handleCommand(msg *tgbotapi.Message, logger *TelegramLogger, cfg *Config, userStore *UserStore) {
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

	logger.Printf("Command received: /%s from %s (Chat %d)", command, formatUser(msg.From), chatID)

	helpText := "Available Commands:\n" +
		"• /ping — Return \"Pong!\"\n" +
		"• /status — Check server status, RAM, CPU, and disk space\n" +
		"• /get\\_cpu\\_usage — Show current CPU usage\n" +
		"• /get\\_ram\\_usage — Show current RAM usage\n" +
		"• /get\\_disk\\_usage — Show free disk space on all drives\n" +
		"• /get\\_services — List available services\n" +
		"• /get\\_update — Check for bot updates\n" +
		"• /restart\\_server — Reboot the server"

	if isOwner {
		helpText += "\n\n *Owner Commands:*\n" +
			"• /add\\_user <id> — Add an authorized user\n" +
			"• /delete\\_user <id> — Remove a user\n" +
			"• /get\\_users — List all authorized users"
	}

	switch command {
	case "ping":
		logger.SendMessage(chatID, "Pong!")

	case "start", "help":
		logger.SendMarkdown(chatID, helpText)

	case "restart_server":
		if !isAuthorized {
			logger.SendMessage(chatID, "🚫 Permission denied.")
			return
		}
		logger.Printf("Server restart requested by %s (Chat %d)", formatUser(msg.From), chatID)

		msg := tgbotapi.NewMessage(chatID, "⚠️ *Are you sure you want to restart the server?*")
		msg.ParseMode = tgbotapi.ModeMarkdown
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Yes, restart", "confirm_restart_server"),
				tgbotapi.NewInlineKeyboardButtonData("No, cancel", "close_message"),
			),
		)
		logger.Send(msg)

	case "get_update":
		if !isAuthorized {
			logger.SendMessage(chatID, "🚫 Permission denied.")
			return
		}
		handleUpdateCommand(chatID, logger, cfg)

	case "get_services":
		if !isAuthorized {
			logger.SendMessage(chatID, "🚫 Permission denied.")
			return
		}
		services, err := getAvailableServices(cfg)
		if err != nil {
			logger.SendMessage(chatID, "❌ Failed to list services: "+err.Error())
			return
		}

		if len(services) == 0 {
			logger.SendMessage(chatID, "No services found.")
		} else {
			msgText := "Available Services:"
			msg := tgbotapi.NewMessage(chatID, msgText)
			msg.ParseMode = tgbotapi.ModeMarkdown

			var keyboard [][]tgbotapi.InlineKeyboardButton
			for _, service := range services {
				if !isServiceAllowed(service, cfg) {
					continue
				}
				status := getServiceStatus(service)
				statusEmoji := "❓"
				if strings.Contains(status, "active (running)") {
					statusEmoji = "🟢"
				} else if strings.Contains(status, "inactive") {
					statusEmoji = "🔴"
				} else if strings.Contains(status, "failed") {
					statusEmoji = "❌"
				}
				row := []tgbotapi.InlineKeyboardButton{
					tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("%s %s", statusEmoji, service), fmt.Sprintf("service_view:%s", service)),
				}
				keyboard = append(keyboard, row)
			}

			keyboard = append(keyboard, []tgbotapi.InlineKeyboardButton{
				tgbotapi.NewInlineKeyboardButtonData("❌ Close", "close_message"),
			})
			msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(keyboard...)
			logger.Send(msg)
		}

	case "status":
		if !isAuthorized {
			logger.SendMessage(chatID, "🚫 Permission denied.")
			return
		}
		uptime, err := exec.Command("uptime", "-p").Output()
		if err != nil {
			uptime, _ = exec.Command("uptime").Output()
		}
		cpuUsage, _ := getCPUUsageInfo()
		ramUsage, _ := getRAMUsageInfo()
		diskInfo, _ := getDiskSpaceInfo()

		statusMsg := fmt.Sprintf("Server Status\n\nUptime: %s\n\nCPU Usage:\n%s\n\nRAM Usage:\n%s\n\nDisk Space:\n%s",
			strings.TrimSpace(string(uptime)), cpuUsage, ramUsage, diskInfo)

		// Get services status
		services, err := getAvailableServices(cfg)
		if err == nil && len(services) > 0 {
			var servicesStatus []string
			for _, service := range services {
				if !isServiceAllowed(service, cfg) {
					continue
				}
				status := getServiceStatus(service)
				statusEmoji := "❓"
				if strings.Contains(status, "active (running)") {
					statusEmoji = "🟢"
				} else if strings.Contains(status, "inactive") {
					statusEmoji = "🔴"
				} else if strings.Contains(status, "failed") {
					statusEmoji = "❌"
				}
				servicesStatus = append(servicesStatus, fmt.Sprintf("%s %s", statusEmoji, service))
			}
			if len(servicesStatus) > 0 {
				statusMsg += "\n\nServices Status:\n" + strings.Join(servicesStatus, "\n")
			}
		}

		logger.SendMarkdown(chatID, statusMsg)

	case "get_cpu_usage":
		if !isAuthorized {
			logger.SendMessage(chatID, "🚫 Permission denied.")
			return
		}
		cpuUsage, err := getCPUUsageInfo()
		if err != nil {
			logger.SendMessage(chatID, "❌ Failed to get CPU usage: "+err.Error())
			return
		}
		logger.SendMarkdown(chatID, "CPU Usage:\n"+cpuUsage)

	case "get_ram_usage":
		if !isAuthorized {
			logger.SendMessage(chatID, "🚫 Permission denied.")
			return
		}
		ramUsage, err := getRAMUsageInfo()
		if err != nil {
			logger.SendMessage(chatID, "❌ Failed to get RAM usage: "+err.Error())
			return
		}
		logger.SendMarkdown(chatID, "RAM Usage:\n"+ramUsage)

	case "get_disk_usage":
		if !isAuthorized {
			logger.SendMessage(chatID, "🚫 Permission denied.")
			return
		}
		diskInfo, err := getDiskSpaceInfo()
		if err != nil {
			logger.SendMessage(chatID, "❌ Failed to get disk space: "+err.Error())
			return
		}
		logger.SendMarkdown(chatID, "Free Disk Space:\n"+diskInfo)

	case "add_user":
		if !isOwner {
			logger.SendMessage(chatID, "🚫 Only the owner can add users.")
			return
		}
		parts := strings.Fields(args)
		if len(parts) < 1 {
			logger.SendHTML(chatID, "Usage: <code>/add_user <id></code>")
			return
		}
		id, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			logger.SendMessage(chatID, "❌ Invalid user ID.")
			return
		}
		if err := userStore.AddUser(id); err != nil {
			logger.SendMessage(chatID, "❌ Failed to add user: "+err.Error())
		} else {
			successStr := fmt.Sprintf("User %d added with full permissions.", id)
			logger.SendMessage(chatID, successStr)
			logger.Printf("User %d added with full permissions by owner (%s)", id, formatUser(msg.From))
		}

	case "delete_user":
		if !isOwner {
			logger.SendMessage(chatID, "🚫 Only the owner can delete users.")
			return
		}
		id, err := strconv.ParseInt(strings.TrimSpace(args), 10, 64)
		if err != nil {
			logger.SendHTML(chatID, "Usage: <code>/delete_user <id></code>")
			return
		}
		if err := userStore.DeleteUser(id); err != nil {
			logger.SendMessage(chatID, "❌ Failed to delete user: "+err.Error())
		} else {
			successStr := fmt.Sprintf("User %d deleted.", id)
			logger.SendMessage(chatID, successStr)
			logger.Printf("User %d deleted by owner (%s)", id, formatUser(msg.From))
		}

	case "get_users":
		if !isOwner {
			logger.SendMessage(chatID, "🚫 Only the owner can list users.")
			return
		}
		users, err := userStore.ListUsers(cfg.OwnerID)
		if err != nil {
			logger.SendMessage(chatID, "❌ Failed to list users: "+err.Error())
			return
		}
		if len(users) == 0 {
			logger.SendMessage(chatID, "No authorized users found.")
		} else {
			logger.SendMarkdown(chatID, "Authorized Users:\n• "+strings.Join(users, "\n• "))
		}

	default:
		logger.SendMessage(chatID, "❓ I don't know that command. Type /help for a list of available commands.")
	}
}

func getAvailableServices(cfg *Config) ([]string, error) {
	if len(cfg.ControlledServices) > 0 {
		return cfg.ControlledServices, nil
	}

	// List all services if no specific list is provided
	out, err := exec.Command("systemctl", "list-units", "--type=service", "--state=running", "--no-legend", "--no-pager").Output()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var services []string
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) > 0 {
			// systemctl list-units output: UNIT LOAD ACTIVE SUB DESCRIPTION
			// Fields[0] is the service name like 'nginx.service'
			services = append(services, strings.TrimSuffix(fields[0], ".service"))
		}
	}
	return services, nil
}

func getServiceStatus(serviceName string) string {
	// #nosec G204
	out, err := exec.Command("systemctl", "status", serviceName).Output()
	if err != nil {
		// systemctl status returns non-zero if service is not running or not found
		if len(out) > 0 {
			return parseStatus(string(out))
		}
		return "unknown"
	}
	return parseStatus(string(out))
}

func parseStatus(output string) string {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "Active:") {
			parts := strings.SplitN(line, "Active:", 2)
			if len(parts) > 1 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return "unknown"
}
