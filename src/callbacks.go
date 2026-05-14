package main

import (
	"fmt"
	"html"
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
	if data == "docker_list" {
		containers, err := getDockerContainers()
		if err != nil {
			logger.Request(tgbotapi.NewCallback(query.ID, "❌ Failed to list Docker containers: "+err.Error()))
			return
		}

		msgText := "Docker Containers:"
		editMsg := tgbotapi.NewEditMessageText(query.Message.Chat.ID, query.Message.MessageID, msgText)

		var keyboard [][]tgbotapi.InlineKeyboardButton
		for _, container := range containers {
			statusEmoji := "❓"
			if container.State == "running" {
				statusEmoji = "🟢"
			} else if container.State == "exited" {
				statusEmoji = "🔴"
			} else if container.State == "paused" {
				statusEmoji = "🟡"
			}

			row := []tgbotapi.InlineKeyboardButton{
				tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("%s %s (%s)", statusEmoji, container.Names, container.Image), fmt.Sprintf("docker_view:%s", container.ID)),
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
	target := parts[1]

	if strings.HasPrefix(action, "docker_") {
		handleDockerCallback(bot, query, logger, action, target)
		return
	}

	if !isServiceAllowed(target, cfg) {
		logger.Request(tgbotapi.NewCallback(query.ID, "❌ Unauthorized or invalid service."))
		return
	}

	if action == "service_view" {
		status := getServiceStatus(target)
		statusEmoji := "❓"
		if strings.Contains(status, "active (running)") {
			statusEmoji = "🟢"
		} else if strings.Contains(status, "inactive") {
			statusEmoji = "🔴"
		} else if strings.Contains(status, "failed") {
			statusEmoji = "❌"
		}

		msgText := fmt.Sprintf("Service: <code>%s</code>\nStatus: %s %s", target, statusEmoji, status)
		editMsg := tgbotapi.NewEditMessageText(query.Message.Chat.ID, query.Message.MessageID, msgText)
		editMsg.ParseMode = tgbotapi.ModeHTML

		var keyboard [][]tgbotapi.InlineKeyboardButton
		var row []tgbotapi.InlineKeyboardButton

		if strings.Contains(status, "active (running)") {
			row = append(row, tgbotapi.NewInlineKeyboardButtonData("Stop", fmt.Sprintf("service_stop:%s", target)))
			row = append(row, tgbotapi.NewInlineKeyboardButtonData("Restart", fmt.Sprintf("service_restart:%s", target)))
		} else {
			row = append(row, tgbotapi.NewInlineKeyboardButtonData("Start", fmt.Sprintf("service_start:%s", target)))
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
		cmd = exec.Command("systemctl", "start", target)
	case "service_stop":
		actionVerb = "stopping"
		actionPast = "stopped"
		// #nosec G204
		cmd = exec.Command("systemctl", "stop", target)
	case "service_restart":
		actionVerb = "restarting"
		actionPast = "restarted"
		// #nosec G204
		cmd = exec.Command("systemctl", "restart", target)
	default:
		return
	}

	// Answer callback to remove loading state
	logger.Request(tgbotapi.NewCallback(query.ID, fmt.Sprintf("%s %s...", strings.ToUpper(actionVerb[:1])+actionVerb[1:], target)))

	err := cmd.Run()
	if err != nil {
		logger.Printf("❌ Failed to %s service %s by %s: %v", actionVerb, target, formatUser(query.From), err)
		logger.SendMessage(query.Message.Chat.ID, fmt.Sprintf("❌ Failed to %s service %s.", actionVerb, target))
	} else {
		successMsg := fmt.Sprintf("Service %s %s successfully.", target, actionPast)
		logger.Printf("%s (Actioned by %s)", successMsg, formatUser(query.From))

		// Update the original message with new status
		status := getServiceStatus(target)
		statusEmoji := "❓"
		if strings.Contains(status, "active (running)") {
			statusEmoji = "🟢"
		} else if strings.Contains(status, "inactive") {
			statusEmoji = "🔴"
		} else if strings.Contains(status, "failed") {
			statusEmoji = "❌"
		}

		newText := fmt.Sprintf("Service: <code>%s</code>\nStatus: %s %s\n\n%s", target, statusEmoji, status, successMsg)
		editMsg := tgbotapi.NewEditMessageText(query.Message.Chat.ID, query.Message.MessageID, newText)
		editMsg.ParseMode = tgbotapi.ModeHTML

		var keyboard [][]tgbotapi.InlineKeyboardButton
		var row []tgbotapi.InlineKeyboardButton

		if strings.Contains(status, "active (running)") {
			row = append(row, tgbotapi.NewInlineKeyboardButtonData("Stop", fmt.Sprintf("service_stop:%s", target)))
			row = append(row, tgbotapi.NewInlineKeyboardButtonData("Restart", fmt.Sprintf("service_restart:%s", target)))
		} else {
			row = append(row, tgbotapi.NewInlineKeyboardButtonData("Start", fmt.Sprintf("service_start:%s", target)))
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

func handleDockerCallback(bot *tgbotapi.BotAPI, query *tgbotapi.CallbackQuery, logger *TelegramLogger, action, containerID string) {
	if action == "docker_view" {
		container, err := getDockerContainer(containerID)
		if err != nil {
			logger.Request(tgbotapi.NewCallback(query.ID, "❌ Failed to get container info: "+err.Error()))
			return
		}

		statusEmoji := "❓"
		if container.State == "running" {
			statusEmoji = "🟢"
		} else if container.State == "exited" {
			statusEmoji = "🔴"
		} else if container.State == "paused" {
			statusEmoji = "🟡"
		}

		msgText := fmt.Sprintf("Container: <code>%s</code>\nImage: <code>%s</code>\nStatus: %s %s",
			container.Names, container.Image, statusEmoji, container.Status)
		editMsg := tgbotapi.NewEditMessageText(query.Message.Chat.ID, query.Message.MessageID, msgText)
		editMsg.ParseMode = tgbotapi.ModeHTML

		var keyboard [][]tgbotapi.InlineKeyboardButton
		var row1 []tgbotapi.InlineKeyboardButton
		var row2 []tgbotapi.InlineKeyboardButton

		if container.State == "running" {
			row1 = append(row1, tgbotapi.NewInlineKeyboardButtonData("Stop", fmt.Sprintf("docker_stop:%s", containerID)))
			row1 = append(row1, tgbotapi.NewInlineKeyboardButtonData("Restart", fmt.Sprintf("docker_restart:%s", containerID)))
		} else {
			row1 = append(row1, tgbotapi.NewInlineKeyboardButtonData("Start", fmt.Sprintf("docker_start:%s", containerID)))
		}
		keyboard = append(keyboard, row1)

		row2 = append(row2, tgbotapi.NewInlineKeyboardButtonData("Logs", fmt.Sprintf("docker_logs:%s", containerID)))
		row2 = append(row2, tgbotapi.NewInlineKeyboardButtonData("Repull", fmt.Sprintf("docker_repull:%s", containerID)))
		keyboard = append(keyboard, row2)

		keyboard = append(keyboard, []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData("Back to list", "docker_list"),
		})

		editMsg.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{InlineKeyboard: keyboard}
		logger.Send(editMsg)
		return
	}

	if action == "docker_logs" {
		logs, err := getDockerLogs(containerID, 100)
		if err != nil {
			logger.Request(tgbotapi.NewCallback(query.ID, "❌ Failed to get logs: "+err.Error()))
			return
		}
		if logs == "" {
			logs = "(Empty logs)"
		}

		// Telegram has a limit on message length (4096 characters)
		if len(logs) > 4000 {
			logs = logs[len(logs)-4000:]
		}

		msgText := fmt.Sprintf("Last 100 lines of logs for <code>%s</code>:\n<pre>%s</pre>", containerID, html.EscapeString(logs))
		msg := tgbotapi.NewMessage(query.Message.Chat.ID, msgText)
		msg.ParseMode = tgbotapi.ModeHTML
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Close", "close_message"),
			),
		)
		logger.Send(msg)
		logger.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	}

	var cmd *exec.Cmd
	var actionVerb, actionPast string
	var imageToPull string

	switch action {
	case "docker_start":
		actionVerb = "starting"
		actionPast = "started"
		cmd = exec.Command("docker", "start", containerID)
	case "docker_stop":
		actionVerb = "stopping"
		actionPast = "stopped"
		cmd = exec.Command("docker", "stop", containerID)
	case "docker_restart":
		actionVerb = "restarting"
		actionPast = "restarted"
		cmd = exec.Command("docker", "restart", containerID)
	case "docker_repull":
		container, err := getDockerContainer(containerID)
		if err != nil {
			logger.Request(tgbotapi.NewCallback(query.ID, "❌ Failed to get container info for repull: "+err.Error()))
			return
		}
		imageToPull = container.Image
		actionVerb = "pulling"
		actionPast = "pulled"
	default:
		return
	}

	logger.Request(tgbotapi.NewCallback(query.ID, fmt.Sprintf("%s %s...", strings.ToUpper(actionVerb[:1])+actionVerb[1:], containerID)))

	var err error
	var output string
	if action == "docker_repull" {
		output, err = dockerPull(imageToPull)
	} else if cmd != nil {
		err = cmd.Run()
	} else {
		return
	}

	if err != nil {
		logger.Printf("❌ Failed to %s container %s by %s: %v", actionVerb, containerID, formatUser(query.From), err)
		logger.SendMessage(query.Message.Chat.ID, fmt.Sprintf("❌ Failed to %s container %s.", actionVerb, containerID))
	} else {
		successMsg := fmt.Sprintf("Container %s %s successfully.", containerID, actionPast)
		if action == "docker_repull" {
			successMsg = fmt.Sprintf("Image <code>%s</code> pulled successfully.", imageToPull)
			// Show first few lines of output
			if len(output) > 200 {
				output = output[:200] + "..."
			}
			successMsg += fmt.Sprintf("\n<pre>%s</pre>", html.EscapeString(output))
		}
		logger.Printf("%s (Actioned by %s)", successMsg, formatUser(query.From))

		// For repull, we don't necessarily update the view as it might be a lot of text,
		// but for start/stop/restart we definitely want to update the view.
		if action != "docker_repull" {
			// Reuse docker_view to update the message
			handleDockerCallback(bot, query, logger, "docker_view", containerID)
		} else {
			msg := tgbotapi.NewMessage(query.Message.Chat.ID, successMsg)
			msg.ParseMode = tgbotapi.ModeHTML
			logger.Send(msg)
		}
	}
}
