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
	l.SendMessage(l.channelID, msg)
}

func (l *TelegramLogger) SendMessage(chatID int64, text string) {
	l.Send(tgbotapi.NewMessage(chatID, text))
}

func (l *TelegramLogger) SendMarkdown(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeMarkdown
	l.Send(msg)
}

func (l *TelegramLogger) SendHTML(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeHTML
	l.Send(msg)
}

func (l *TelegramLogger) Request(c tgbotapi.Chattable) {
	if _, err := l.bot.Request(c); err != nil {
		log.Printf("Failed to send request to Telegram: %v", err)
	}
}

func (l *TelegramLogger) Send(c tgbotapi.Chattable) {
	if _, err := l.bot.Send(c); err != nil {
		log.Printf("Failed to send message to Telegram: %v", err)
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
