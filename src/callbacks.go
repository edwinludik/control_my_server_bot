package main

import (
	"fmt"
	"log"
	"os/exec"
	"slices"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func handleCallback(bot *tgbotapi.BotAPI, query *tgbotapi.CallbackQuery, logger *TelegramLogger, cfg *Config) {
	data := query.Data
	if data == "close_message" {
		deleteMsg := tgbotapi.NewDeleteMessage(query.Message.Chat.ID, query.Message.MessageID)
		if _, err := bot.Send(deleteMsg); err != nil {
			log.Printf("Failed to delete message: %v", err)
		}
		return
	}
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

		if _, err := bot.Send(editMsg); err != nil {
			log.Printf("Failed to edit message to show services list: %v", err)
		}
		return
	}
	if data == "confirm_restart_server" {
		logger.Printf("Restarting server confirmed by %s", formatUser(query.From))
		if _, err := bot.Send(tgbotapi.NewMessage(query.Message.Chat.ID, "🔄 Restarting server...")); err != nil {
			log.Printf("Failed to send restarting server message: %v", err)
		}

		// Answer callback to remove loading state
		callback := tgbotapi.NewCallback(query.ID, "🔄 Restarting server...")
		if _, err := bot.Request(callback); err != nil {
			log.Printf("Failed to send callback answer: %v", err)
		}

		cmd := exec.Command("reboot")
		if err := cmd.Run(); err != nil {
			logger.Printf("❌ Failed to restart server: %v", err)
			if _, err := bot.Send(tgbotapi.NewMessage(query.Message.Chat.ID, "❌ Failed to restart server. See logs for details.")); err != nil {
				log.Printf("Failed to send restart failure message: %v", err)
			}
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
	callback := tgbotapi.NewCallback(query.ID, fmt.Sprintf("%s %s...", strings.ToUpper(actionVerb[:1])+actionVerb[1:], serviceName))
	if _, err := bot.Request(callback); err != nil {
		log.Printf("Failed to send callback answer: %v", err)
	}

	err := cmd.Run()
	if err != nil {
		logger.Printf("❌ Failed to %s service %s by %s: %v", actionVerb, serviceName, formatUser(query.From), err)
		msg := tgbotapi.NewMessage(query.Message.Chat.ID, fmt.Sprintf("❌ Failed to %s service %s.", actionVerb, serviceName))
		if _, err := bot.Send(msg); err != nil {
			log.Printf("Failed to send error message: %v", err)
		}
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

		newText := fmt.Sprintf("*Service:* <code>%s</code>\n*Status:* %s %s\n\n%s", serviceName, statusEmoji, status, successMsg)
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
