package main

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func handleCallback(bot *tgbotapi.BotAPI, query *tgbotapi.CallbackQuery, logger *TelegramLogger, cfg *Config) (reason, requester string, t time.Time) {
	data := query.Data
	if data == "close_message" {
		logger.Request(tgbotapi.NewDeleteMessage(query.Message.Chat.ID, query.Message.MessageID))
		return
	}
	if data == "services_list" {
		services, err := getAvailableServices(cfg)
		if err != nil {
			logger.Request(tgbotapi.NewCallback(query.ID, "❌ Failed to list services: "+err.Error()))
			return
		}

		msgText := "Available Services:"
		editMsg := tgbotapi.NewEditMessageText(query.Message.Chat.ID, query.Message.MessageID, msgText)

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
		editMsg.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{InlineKeyboard: keyboard}

		logger.Send(editMsg)
		return
	}
	if data == "confirm_restart_server" {
		logger.Printf("Restarting server confirmed by %s", formatUser(query.From))

		reason = "Restart Server"
		requester = formatUser(query.From)
		t = time.Now()

		// Update message to remove buttons and show confirmation
		now := t.Format("15:04:05")
		msgText := fmt.Sprintf("⚠️ Are you sure you want to restart the server?\n\n✅ Action confirmed at %s", now)
		editMsg := tgbotapi.NewEditMessageText(query.Message.Chat.ID, query.Message.MessageID, msgText)
		editMsg.ReplyMarkup = nil
		logger.Send(editMsg)

		// Answer callback to remove loading state
		logger.Request(tgbotapi.NewCallback(query.ID, "Restarting server..."))

		cmd := exec.Command("reboot")
		if err := cmd.Run(); err != nil {
			logger.Printf("❌ Failed to restart server: %v", err)
			logger.SendMessage(query.Message.Chat.ID, "❌ Failed to restart server. See logs for details.")
			// Reset return values if reboot fails
			return "", "", time.Time{}
		}
		return
	}
	if data == "confirm_update" {
		logger.Printf("Bot update confirmed by %s", formatUser(query.From))

		reason = "Bot Update"
		requester = formatUser(query.From)
		t = time.Now()

		// Answer callback to remove loading state
		logger.Request(tgbotapi.NewCallback(query.ID, "Updating bot..."))

		performUpdate(query.Message.Chat.ID, logger, cfg)
		return
	}

	parts := strings.SplitN(data, ":", 2)
	if len(parts) != 2 {
		return
	}

	action := parts[0]
	serviceName := parts[1]

	if !isServiceAllowed(serviceName, cfg) {
		logger.Request(tgbotapi.NewCallback(query.ID, "❌ Unauthorized or invalid service."))
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

		msgText := fmt.Sprintf("Service: <code>%s</code>\nStatus: %s %s", serviceName, statusEmoji, status)
		editMsg := tgbotapi.NewEditMessageText(query.Message.Chat.ID, query.Message.MessageID, msgText)
		editMsg.ParseMode = tgbotapi.ModeHTML

		var keyboard [][]tgbotapi.InlineKeyboardButton
		var row []tgbotapi.InlineKeyboardButton

		if strings.Contains(status, "active (running)") {
			row = append(row, tgbotapi.NewInlineKeyboardButtonData("Stop", fmt.Sprintf("service_stop:%s", serviceName)))
			row = append(row, tgbotapi.NewInlineKeyboardButtonData("Restart", fmt.Sprintf("service_restart:%s", serviceName)))
		} else {
			row = append(row, tgbotapi.NewInlineKeyboardButtonData("Start", fmt.Sprintf("service_start:%s", serviceName)))
		}
		keyboard = append(keyboard, row)

		backRow := []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData("Back to list", "services_list"),
		}
		keyboard = append(keyboard, backRow)

		editMsg.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{InlineKeyboard: keyboard}

		logger.Send(editMsg)
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
	logger.Request(tgbotapi.NewCallback(query.ID, fmt.Sprintf("%s %s...", strings.ToUpper(actionVerb[:1])+actionVerb[1:], serviceName)))

	err := cmd.Run()
	if err != nil {
		logger.Printf("❌ Failed to %s service %s by %s: %v", actionVerb, serviceName, formatUser(query.From), err)
		logger.SendMessage(query.Message.Chat.ID, fmt.Sprintf("❌ Failed to %s service %s.", actionVerb, serviceName))
	} else {
		successMsg := fmt.Sprintf("Service %s %s successfully.", serviceName, actionPast)
		logger.Printf("%s (Actioned by %s)", successMsg, formatUser(query.From))

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

		newText := fmt.Sprintf("Service: <code>%s</code>\nStatus: %s %s\n\n%s", serviceName, statusEmoji, status, successMsg)
		editMsg := tgbotapi.NewEditMessageText(query.Message.Chat.ID, query.Message.MessageID, newText)
		editMsg.ParseMode = tgbotapi.ModeHTML

		var keyboard [][]tgbotapi.InlineKeyboardButton
		var row []tgbotapi.InlineKeyboardButton

		if strings.Contains(status, "active (running)") {
			row = append(row, tgbotapi.NewInlineKeyboardButtonData("Stop", fmt.Sprintf("service_stop:%s", serviceName)))
			row = append(row, tgbotapi.NewInlineKeyboardButtonData("Restart", fmt.Sprintf("service_restart:%s", serviceName)))
		} else {
			row = append(row, tgbotapi.NewInlineKeyboardButtonData("Start", fmt.Sprintf("service_start:%s", serviceName)))
		}

		keyboard = append(keyboard, row)

		backRow := []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData("Back to list", "services_list"),
		}
		keyboard = append(keyboard, backRow)

		editMsg.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{InlineKeyboard: keyboard}

		if _, err := bot.Send(editMsg); err != nil {
			log.Printf("Failed to edit message: %v", err)
			// If edit fails (e.g. text is the same), at least send a new message
			logger.SendMessage(query.Message.Chat.ID, successMsg)
		}
	}
	return
}
