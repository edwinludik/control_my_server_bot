package main

import (
	"fmt"
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

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

func formatUser(user *tgbotapi.User) string {
	if user == nil {
		return "unknown user"
	}
	name := user.FirstName
	if user.LastName != "" {
		name += " " + user.LastName
	}
	if user.UserName != "" {
		return fmt.Sprintf("%s (@%s, ID: %d)", name, user.UserName, user.ID)
	}
	return fmt.Sprintf("%s (ID: %d)", name, user.ID)
}
