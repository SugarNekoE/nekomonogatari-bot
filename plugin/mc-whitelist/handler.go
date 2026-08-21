package mcwhitelist

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"

	framework "forge.asnk.io/sugar/nekomonogatari-bot/plugin"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type commandKind uint8

const (
	commandHelp commandKind = iota
	commandAdd
	commandList
	commandDelete
)

type parsedCommand struct {
	Kind       commandKind
	PlayerName string
	UsageKey   messageKey
}

func parseMCWLCommand(arguments []string) parsedCommand {
	if len(arguments) == 0 {
		return parsedCommand{Kind: commandHelp}
	}
	switch strings.ToLower(arguments[0]) {
	case "add":
		if len(arguments) != 2 {
			return parsedCommand{Kind: commandHelp, UsageKey: messageUsageAdd}
		}
		return parsedCommand{Kind: commandAdd, PlayerName: arguments[1]}
	case "list":
		if len(arguments) != 1 {
			return parsedCommand{Kind: commandHelp}
		}
		return parsedCommand{Kind: commandList}
	case "del":
		if len(arguments) != 2 {
			return parsedCommand{Kind: commandHelp, UsageKey: messageUsageDelete}
		}
		return parsedCommand{Kind: commandDelete, PlayerName: arguments[1]}
	case "help":
		return parsedCommand{Kind: commandHelp}
	default:
		return parsedCommand{Kind: commandHelp}
	}
}

func (p *plugin) handleCommand(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update == nil || update.Message == nil || update.Message.From == nil || update.Message.From.IsBot {
		return
	}
	_, arguments := framework.ParseCommand(update.Message.Text)
	command := parseMCWLCommand(arguments)
	userID := update.Message.From.ID
	switch command.Kind {
	case commandAdd:
		p.handleAdd(ctx, b, update.Message, userID, command.PlayerName)
	case commandList:
		p.handleList(ctx, b, update.Message, userID)
	case commandDelete:
		p.handleDelete(ctx, b, update.Message, userID, command.PlayerName)
	default:
		key := messageHelp
		if command.UsageKey != "" {
			key = command.UsageKey
		}
		p.sendReply(ctx, b, update.Message, p.text(key), p.menuKeyboard(userID))
	}
}

func (p *plugin) handleAdd(ctx context.Context, b *bot.Bot, message *models.Message, userID int64, playerName string) {
	item, count, err := p.service.add(ctx, userID, playerName)
	text := p.operationError(err, true, playerName)
	if err == nil {
		text = p.text(messageAddSuccess, item.PlayerName, count)
	}
	p.sendReply(ctx, b, message, text, nil)
}

func (p *plugin) handleDelete(ctx context.Context, b *bot.Bot, message *models.Message, userID int64, playerName string) {
	item, count, err := p.service.removeByName(ctx, userID, playerName)
	text := p.operationError(err, false, playerName)
	if err == nil {
		text = p.text(messageDeleteSuccess, item.PlayerName, count)
	}
	p.sendReply(ctx, b, message, text, nil)
}

func (p *plugin) handleList(ctx context.Context, b *bot.Bot, message *models.Message, userID int64) {
	items, err := p.service.list(ctx, userID)
	if err != nil {
		log.Printf("[%s] list user %d: %v", pluginName, userID, err)
		p.sendReply(ctx, b, message, p.text(messageServiceError), nil)
		return
	}
	p.sendReply(ctx, b, message, p.formatList(items), nil)
}

func (p *plugin) sendReply(ctx context.Context, b *bot.Bot, message *models.Message, text string, markup models.ReplyMarkup) {
	parameters := &bot.SendMessageParams{
		ChatID:          message.Chat.ID,
		MessageThreadID: message.MessageThreadID,
		Text:            text,
		ReplyParameters: &models.ReplyParameters{
			MessageID:                message.ID,
			AllowSendingWithoutReply: true,
		},
		ReplyMarkup: markup,
	}
	if _, err := b.SendMessage(ctx, parameters); err != nil {
		log.Printf("[%s] send Telegram message: %v", pluginName, err)
	}
}

type callbackAction struct {
	OwnerID   int64
	Action    string
	BindingID int64
}

func parseCallbackData(value string) (callbackAction, error) {
	parts := strings.Split(value, "|")
	if len(parts) < 3 || len(parts) > 4 || parts[0] != "mcwl" {
		return callbackAction{}, errors.New("invalid callback structure")
	}
	ownerID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || ownerID <= 0 {
		return callbackAction{}, errors.New("invalid callback owner")
	}
	action := callbackAction{OwnerID: ownerID, Action: parts[2]}
	switch action.Action {
	case "add", "list", "delete", "back", "close":
		if len(parts) != 3 {
			return callbackAction{}, errors.New("unexpected callback argument")
		}
	case "rm":
		if len(parts) != 4 {
			return callbackAction{}, errors.New("missing binding id")
		}
		action.BindingID, err = strconv.ParseInt(parts[3], 10, 64)
		if err != nil || action.BindingID <= 0 {
			return callbackAction{}, errors.New("invalid binding id")
		}
	default:
		return callbackAction{}, errors.New("unknown callback action")
	}
	return action, nil
}

func (p *plugin) handleCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update == nil || update.CallbackQuery == nil {
		return
	}
	query := update.CallbackQuery
	action, err := parseCallbackData(query.Data)
	if err != nil {
		p.answerCallback(ctx, b, query.ID, p.text(messageMenuInvalid), true)
		return
	}
	if query.From.IsBot || query.From.ID != action.OwnerID {
		p.answerCallback(ctx, b, query.ID, p.text(messageMenuOwnerOnly), true)
		return
	}
	message := query.Message.Message
	if query.Message.Type != models.MaybeInaccessibleMessageTypeMessage || message == nil {
		p.answerCallback(ctx, b, query.ID, p.text(messageMenuInvalid), true)
		return
	}
	p.answerCallback(ctx, b, query.ID, "", false)

	switch action.Action {
	case "add":
		p.editMenu(ctx, b, message, p.text(messageMenuAdd), p.backKeyboard(action.OwnerID))
	case "list":
		items, listErr := p.service.list(ctx, action.OwnerID)
		if listErr != nil {
			log.Printf("[%s] callback list user %d: %v", pluginName, action.OwnerID, listErr)
			p.editMenu(ctx, b, message, p.text(messageServiceError), p.backKeyboard(action.OwnerID))
			return
		}
		p.editMenu(ctx, b, message, p.formatList(items), p.backKeyboard(action.OwnerID))
	case "delete":
		p.renderDeleteMenu(ctx, b, message, action.OwnerID, "")
	case "rm":
		item, count, removeErr := p.service.removeByID(ctx, action.OwnerID, action.BindingID)
		result := p.operationError(removeErr, false, item.PlayerName)
		if removeErr == nil {
			result = p.text(messageDeleteSuccess, item.PlayerName, count)
		}
		p.renderDeleteMenu(ctx, b, message, action.OwnerID, result)
	case "back":
		p.editMenu(ctx, b, message, p.text(messageHelp), p.menuKeyboard(action.OwnerID))
	case "close":
		p.editMenu(ctx, b, message, p.text(messageMenuClosed), &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{}})
	}
}

func (p *plugin) renderDeleteMenu(ctx context.Context, b *bot.Bot, message *models.Message, ownerID int64, result string) {
	items, err := p.service.list(ctx, ownerID)
	if err != nil {
		log.Printf("[%s] build delete menu for user %d: %v", pluginName, ownerID, err)
		p.editMenu(ctx, b, message, p.text(messageServiceError), p.backKeyboard(ownerID))
		return
	}
	text := p.text(messageMenuDelete)
	if len(items) == 0 {
		text = p.text(messageMenuDeleteEmpty)
	}
	if result != "" {
		text = result + "\n\n" + text
	}
	p.editMenu(ctx, b, message, text, p.deleteKeyboard(ownerID, items))
}

func (p *plugin) answerCallback(ctx context.Context, b *bot.Bot, id, text string, alert bool) {
	if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: id,
		Text:            text,
		ShowAlert:       alert,
	}); err != nil {
		log.Printf("[%s] answer callback query: %v", pluginName, err)
	}
}

func (p *plugin) editMenu(ctx context.Context, b *bot.Bot, message *models.Message, text string, markup models.ReplyMarkup) {
	if _, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:      message.Chat.ID,
		MessageID:   message.ID,
		Text:        text,
		ReplyMarkup: markup,
	}); err != nil {
		log.Printf("[%s] edit menu: %v", pluginName, err)
	}
}

func (p *plugin) menuKeyboard(ownerID int64) *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{
			{Text: p.text(messageButtonAdd), CallbackData: callbackData(ownerID, "add", 0)},
			{Text: p.text(messageButtonList), CallbackData: callbackData(ownerID, "list", 0)},
		},
		{{Text: p.text(messageButtonDelete), CallbackData: callbackData(ownerID, "delete", 0)}},
		{{Text: p.text(messageButtonClose), CallbackData: callbackData(ownerID, "close", 0)}},
	}}
}

func (p *plugin) backKeyboard(ownerID int64) *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: p.text(messageButtonBack), CallbackData: callbackData(ownerID, "back", 0)}},
	}}
}

func (p *plugin) deleteKeyboard(ownerID int64, items []binding) *models.InlineKeyboardMarkup {
	rows := make([][]models.InlineKeyboardButton, 0, len(items)+1)
	for _, item := range items {
		rows = append(rows, []models.InlineKeyboardButton{{
			Text:         "➖ " + item.PlayerName,
			CallbackData: callbackData(ownerID, "rm", item.ID),
		}})
	}
	rows = append(rows, []models.InlineKeyboardButton{{
		Text:         p.text(messageButtonBack),
		CallbackData: callbackData(ownerID, "back", 0),
	}})
	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func callbackData(ownerID int64, action string, bindingID int64) string {
	if bindingID > 0 {
		return fmt.Sprintf("mcwl|%d|%s|%d", ownerID, action, bindingID)
	}
	return fmt.Sprintf("mcwl|%d|%s", ownerID, action)
}

func (p *plugin) operationError(err error, adding bool, playerName string) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrInvalidPlayerName):
		return p.text(messageInvalidName)
	case errors.Is(err, ErrAccountLimit):
		return p.text(messageLimit)
	case errors.Is(err, ErrPlayerBound):
		return p.text(messageAlreadyBound)
	case errors.Is(err, ErrNotBound):
		return p.text(messageNotBound)
	case errors.Is(err, ErrPending):
		if adding {
			return p.text(messagePendingAdd, playerName)
		}
		return p.text(messagePendingDelete, playerName)
	default:
		log.Printf("[%s] whitelist operation for %q: %v", pluginName, playerName, err)
		return p.text(messageServiceError)
	}
}

func (p *plugin) formatList(items []binding) string {
	if len(items) == 0 {
		return p.text(messageListEmpty)
	}
	lines := make([]string, 0, len(items)+1)
	lines = append(lines, p.text(messageListHeader, len(items)))
	for index, item := range items {
		line := fmt.Sprintf("%d. %s", index+1, item.PlayerName)
		switch item.State {
		case statePendingAdd:
			line += " (" + p.text(messageStatusAdding) + ")"
		case statePendingDelete:
			line += " (" + p.text(messageStatusDeleting) + ")"
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (p *plugin) text(key messageKey, arguments ...any) string {
	return translate(p.language, key, arguments...)
}
