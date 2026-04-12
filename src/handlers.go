package main

import (
	"fmt"
	"log"
	"os/exec"
	"regexp"
	"slices"
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
		cmd := exec.Command("reboot")
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
		// #nosec G204
		cmd := exec.Command("systemctl", "restart", serviceName)
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
			logger.Printf("%s", successStr)
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
				row := []tgbotapi.InlineKeyboardButton{
					tgbotapi.NewInlineKeyboardButtonData("📦 "+service, fmt.Sprintf("service_view:%s", service)),
				}
				keyboard = append(keyboard, row)
			}

			msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(keyboard...)
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

func handleCallback(bot *tgbotapi.BotAPI, query *tgbotapi.CallbackQuery, logger *TelegramLogger, cfg *Config) {
	data := query.Data
	if data == "services_list" {
		services, err := getAvailableServices(cfg)
		if err != nil {
			callback := tgbotapi.NewCallback(query.ID, "❌ Failed to list services: "+err.Error())
			if _, err := bot.Request(callback); err != nil {
				log.Printf("Failed to send callback answer: %v", err)
			}
			return
		}

		msgText := "📋 *Available Services:*"
		editMsg := tgbotapi.NewEditMessageText(query.Message.Chat.ID, query.Message.MessageID, msgText)
		editMsg.ParseMode = tgbotapi.ModeMarkdown

		var keyboard [][]tgbotapi.InlineKeyboardButton
		for _, service := range services {
			row := []tgbotapi.InlineKeyboardButton{
				tgbotapi.NewInlineKeyboardButtonData("📦 "+service, fmt.Sprintf("service_view:%s", service)),
			}
			keyboard = append(keyboard, row)
		}
		editMsg.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{InlineKeyboard: keyboard}

		if _, err := bot.Send(editMsg); err != nil {
			log.Printf("Failed to edit message to show services list: %v", err)
		}
		return
	}

	parts := strings.SplitN(data, ":", 2)
	if len(parts) != 2 {
		return
	}

	action := parts[0]
	serviceName := parts[1]

	if !serviceNameRegex.MatchString(serviceName) {
		callback := tgbotapi.NewCallback(query.ID, "❌ Invalid service name.")
		if _, err := bot.Request(callback); err != nil {
			log.Printf("Failed to send callback answer: %v", err)
		}
		return
	}

	if len(cfg.ControlledServices) > 0 && !slices.Contains(cfg.ControlledServices, serviceName) {
		callback := tgbotapi.NewCallback(query.ID, "🚫 Service is not controlled.")
		if _, err := bot.Request(callback); err != nil {
			log.Printf("Failed to send callback answer: %v", err)
		}
		return
	}

	if action == "service_view" {
		status := getServiceStatus(serviceName)
		statusEmoji := "❓"
		if strings.Contains(status, "active (running)") {
			statusEmoji = "🟢"
		} else if strings.Contains(status, "inactive") {
			statusEmoji = "🔴"
		} else if strings.Contains(status, "failed") {
			statusEmoji = "❌"
		}

		msgText := fmt.Sprintf("📦 *Service:* <code>%s</code>\n*Status:* %s %s", serviceName, statusEmoji, status)
		editMsg := tgbotapi.NewEditMessageText(query.Message.Chat.ID, query.Message.MessageID, msgText)
		editMsg.ParseMode = tgbotapi.ModeHTML

		var keyboard [][]tgbotapi.InlineKeyboardButton
		var row []tgbotapi.InlineKeyboardButton

		if strings.Contains(status, "active (running)") {
			row = append(row, tgbotapi.NewInlineKeyboardButtonData("🛑 Stop", fmt.Sprintf("service_stop:%s", serviceName)))
			row = append(row, tgbotapi.NewInlineKeyboardButtonData("🔄 Restart", fmt.Sprintf("service_restart:%s", serviceName)))
		} else {
			row = append(row, tgbotapi.NewInlineKeyboardButtonData("▶️ Start", fmt.Sprintf("service_start:%s", serviceName)))
		}
		keyboard = append(keyboard, row)

		backRow := []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData("⬅️ Back to list", "services_list"),
		}
		keyboard = append(keyboard, backRow)

		editMsg.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{InlineKeyboard: keyboard}

		if _, err := bot.Send(editMsg); err != nil {
			log.Printf("Failed to edit message for service view %s: %v", serviceName, err)
		}
		return
	}

	var cmd *exec.Cmd
	var actionVerb, actionPast string
	switch action {
	case "service_start":
		actionVerb = "starting"
		actionPast = "started"
		// #nosec G204
		cmd = exec.Command("systemctl", "start", serviceName)
	case "service_stop":
		actionVerb = "stopping"
		actionPast = "stopped"
		// #nosec G204
		cmd = exec.Command("systemctl", "stop", serviceName)
	case "service_restart":
		actionVerb = "restarting"
		actionPast = "restarted"
		// #nosec G204
		cmd = exec.Command("systemctl", "restart", serviceName)
	default:
		return
	}

	// Answer callback to remove loading state
	callback := tgbotapi.NewCallback(query.ID, fmt.Sprintf("🔄 %s %s...", strings.ToUpper(actionVerb[:1])+actionVerb[1:], serviceName))
	if _, err := bot.Request(callback); err != nil {
		log.Printf("Failed to send callback answer: %v", err)
	}

	err := cmd.Run()
	if err != nil {
		logger.Printf("❌ Failed to %s service %s: %v", actionVerb, serviceName, err)
		msg := tgbotapi.NewMessage(query.Message.Chat.ID, fmt.Sprintf("❌ Failed to %s service %s.", actionVerb, serviceName))
		if _, err := bot.Send(msg); err != nil {
			log.Printf("Failed to send error message: %v", err)
		}
	} else {
		successMsg := fmt.Sprintf("✅ Service %s %s successfully.", serviceName, actionPast)
		logger.Printf("%s", successMsg)

		// Update the original message with new status
		status := getServiceStatus(serviceName)
		statusEmoji := "❓"
		if strings.Contains(status, "active (running)") {
			statusEmoji = "🟢"
		} else if strings.Contains(status, "inactive") {
			statusEmoji = "🔴"
		} else if strings.Contains(status, "failed") {
			statusEmoji = "❌"
		}

		newText := fmt.Sprintf("📦 *Service:* <code>%s</code>\n*Status:* %s %s\n\n%s", serviceName, statusEmoji, status, successMsg)
		editMsg := tgbotapi.NewEditMessageText(query.Message.Chat.ID, query.Message.MessageID, newText)
		editMsg.ParseMode = tgbotapi.ModeHTML

		var keyboard [][]tgbotapi.InlineKeyboardButton
		var row []tgbotapi.InlineKeyboardButton

		if strings.Contains(status, "active (running)") {
			row = append(row, tgbotapi.NewInlineKeyboardButtonData("🛑 Stop", fmt.Sprintf("service_stop:%s", serviceName)))
			row = append(row, tgbotapi.NewInlineKeyboardButtonData("🔄 Restart", fmt.Sprintf("service_restart:%s", serviceName)))
		} else {
			row = append(row, tgbotapi.NewInlineKeyboardButtonData("▶️ Start", fmt.Sprintf("service_start:%s", serviceName)))
		}

		keyboard = append(keyboard, row)

		backRow := []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData("⬅️ Back to list", "services_list"),
		}
		keyboard = append(keyboard, backRow)

		editMsg.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{InlineKeyboard: keyboard}

		if _, err := bot.Send(editMsg); err != nil {
			log.Printf("Failed to edit message: %v", err)
			// If edit fails (e.g. text is the same), at least send a new message
			msg := tgbotapi.NewMessage(query.Message.Chat.ID, successMsg)
			if _, err := bot.Send(msg); err != nil {
				log.Printf("Failed to send success message: %v", err)
			}
		}
	}
}
