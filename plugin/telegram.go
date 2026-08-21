package plugin

import (
	"context"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func AllowGroups(groupIDs []int64) bot.Middleware {
	allowed := make(map[int64]struct{}, len(groupIDs))
	for _, id := range groupIDs {
		allowed[id] = struct{}{}
	}
	return func(next bot.HandlerFunc) bot.HandlerFunc {
		return func(ctx context.Context, b *bot.Bot, update *models.Update) {
			chatID, chatType, ok := Chat(update)
			if !ok || (chatType != models.ChatTypeGroup && chatType != models.ChatTypeSupergroup) {
				if update != nil && update.CallbackQuery != nil {
					_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: update.CallbackQuery.ID})
				}
				return
			}
			if _, ok := allowed[chatID]; !ok {
				if update.CallbackQuery != nil {
					_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
						CallbackQueryID: update.CallbackQuery.ID,
					})
				}
				return
			}
			next(ctx, b, update)
		}
	}
}

func ChatID(update *models.Update) (int64, bool) {
	chatID, _, ok := Chat(update)
	return chatID, ok
}

func Chat(update *models.Update) (int64, models.ChatType, bool) {
	if update == nil {
		return 0, "", false
	}
	if update.Message != nil {
		return update.Message.Chat.ID, update.Message.Chat.Type, true
	}
	if update.CallbackQuery == nil {
		return 0, "", false
	}
	switch update.CallbackQuery.Message.Type {
	case models.MaybeInaccessibleMessageTypeMessage:
		if update.CallbackQuery.Message.Message != nil {
			chat := update.CallbackQuery.Message.Message.Chat
			return chat.ID, chat.Type, true
		}
	case models.MaybeInaccessibleMessageTypeInaccessibleMessage:
		if update.CallbackQuery.Message.InaccessibleMessage != nil {
			chat := update.CallbackQuery.Message.InaccessibleMessage.Chat
			return chat.ID, chat.Type, true
		}
	}
	return 0, "", false
}

func Command(name string, botUsername ...string) bot.MatchFunc {
	name = strings.ToLower(strings.TrimPrefix(name, "/"))
	expectedUsername := ""
	if len(botUsername) != 0 {
		expectedUsername = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(botUsername[0]), "@"))
	}
	return func(update *models.Update) bool {
		if update == nil || update.Message == nil || update.Message.Text == "" {
			return false
		}
		command, suffix, _ := parseCommand(update.Message.Text)
		return command == name && (suffix == "" || (expectedUsername != "" && suffix == expectedUsername))
	}
}

func ParseCommand(text string) (string, []string) {
	command, _, arguments := parseCommand(text)
	return command, arguments
}

func parseCommand(text string) (string, string, []string) {
	fields := strings.Fields(text)
	if len(fields) == 0 || !strings.HasPrefix(fields[0], "/") {
		return "", "", nil
	}
	command := strings.TrimPrefix(fields[0], "/")
	suffix := ""
	if at := strings.IndexByte(command, '@'); at >= 0 {
		suffix = strings.ToLower(command[at+1:])
		command = command[:at]
	}
	return strings.ToLower(command), suffix, fields[1:]
}

func DisplayName(user *models.User) string {
	if user == nil {
		return ""
	}
	if user.Username != "" {
		return "@" + user.Username
	}
	return strings.TrimSpace(user.FirstName + " " + user.LastName)
}
