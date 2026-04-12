package main

import (
	"fmt"
	"log"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

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
		logger.Printf("🔄 Server restart requested by chat %d (User %d)", chatID, userID)

		msg := tgbotapi.NewMessage(chatID, "⚠️ *Are you sure you want to restart the server?*")
		msg.ParseMode = tgbotapi.ModeMarkdown
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("✅ Yes, restart", "confirm_restart_server"),
				tgbotapi.NewInlineKeyboardButtonData("❌ No, cancel", "close_message"),
			),
		)
		if _, err := bot.Send(msg); err != nil {
			log.Printf("Failed to send restart confirmation message: %v", err)
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
			msgText := "📋 *Available Services:*"
			msg := tgbotapi.NewMessage(chatID, msgText)
			msg.ParseMode = tgbotapi.ModeMarkdown

			var keyboard [][]tgbotapi.InlineKeyboardButton
			for _, service := range services {
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

			msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(append(keyboard, []tgbotapi.InlineKeyboardButton{
				tgbotapi.NewInlineKeyboardButtonData("Close", "close_message"),
			})...)
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

		// Get services status
		services, err := getAvailableServices(cfg)
		if err == nil && len(services) > 0 {
			var servicesStatus []string
			for _, service := range services {
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
			statusMsg += "\n\n📋 *Services Status:*\n" + strings.Join(servicesStatus, "\n")
		}

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
			msg := tgbotapi.NewMessage(chatID, "ℹ️ Usage: <code>/add_user <id></code>")
			msg.ParseMode = tgbotapi.ModeHTML
			if _, err := bot.Send(msg); err != nil {
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
			msg := tgbotapi.NewMessage(chatID, "ℹ️ Usage: <code>/delete_user <id></code>")
			msg.ParseMode = tgbotapi.ModeHTML
			if _, err := bot.Send(msg); err != nil {
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
