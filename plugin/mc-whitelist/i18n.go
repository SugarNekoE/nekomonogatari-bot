package mcwhitelist

import (
	"fmt"

	"forge.asnk.io/sugar/nekomonogatari-bot/config"
)

type messageKey string

const (
	messageHelp            messageKey = "help"
	messageUsageAdd        messageKey = "usage_add"
	messageUsageDelete     messageKey = "usage_delete"
	messageInvalidName     messageKey = "invalid_name"
	messageLimit           messageKey = "limit"
	messageAlreadyBound    messageKey = "already_bound"
	messageNotBound        messageKey = "not_bound"
	messageAddSuccess      messageKey = "add_success"
	messageDeleteSuccess   messageKey = "delete_success"
	messageListEmpty       messageKey = "list_empty"
	messageListHeader      messageKey = "list_header"
	messagePendingAdd      messageKey = "pending_add"
	messagePendingDelete   messageKey = "pending_delete"
	messageServiceError    messageKey = "service_error"
	messageMenuAdd         messageKey = "menu_add"
	messageMenuDelete      messageKey = "menu_delete"
	messageMenuDeleteEmpty messageKey = "menu_delete_empty"
	messageMenuOwnerOnly   messageKey = "menu_owner_only"
	messageMenuInvalid     messageKey = "menu_invalid"
	messageMenuClosed      messageKey = "menu_closed"
	messageStatusAdding    messageKey = "status_adding"
	messageStatusDeleting  messageKey = "status_deleting"
	messageButtonAdd       messageKey = "button_add"
	messageButtonList      messageKey = "button_list"
	messageButtonDelete    messageKey = "button_delete"
	messageButtonBack      messageKey = "button_back"
	messageButtonClose     messageKey = "button_close"
)

var messages = map[config.Language]map[messageKey]string{
	config.LanguageEnglish: {
		messageHelp: "Minecraft whitelist\n" +
			"/mcwl — open this help and navigation menu\n" +
			"/mcwl add <player_name> — bind and whitelist a player\n" +
			"/mcwl list — show your bound players\n" +
			"/mcwl del <player_name> — unbind and remove a player\n\n" +
			"Each Telegram account may bind up to 2 Java Edition names (3–16 letters, numbers, or underscores).",
		messageUsageAdd:        "Usage: /mcwl add <player_name>\nExample: /mcwl add Steve",
		messageUsageDelete:     "Usage: /mcwl del <player_name>\nUse /mcwl list to see your bound players.",
		messageInvalidName:     "Invalid player name. Use 3–16 letters, numbers, or underscores.",
		messageLimit:           "You already have 2 bound players. Remove one with /mcwl del <player_name> before adding another.",
		messageAlreadyBound:    "That Minecraft player name is already bound.",
		messageNotBound:        "That player is not bound to your Telegram account. Use /mcwl list to check your bindings.",
		messageAddSuccess:      "Added %s to the Minecraft whitelist. Bound players: %d/2. Use /mcwl list to review them.",
		messageDeleteSuccess:   "Removed %s from the Minecraft whitelist. Bound players: %d/2. Use /mcwl add <player_name> to add another.",
		messageListEmpty:       "You have no bound Minecraft players. Use /mcwl add <player_name> or /mcwl to get started.",
		messageListHeader:      "Your Minecraft players (%d/2):",
		messagePendingAdd:      "The server result for adding %s is uncertain. The request was saved and will be retried automatically.",
		messagePendingDelete:   "The server result for removing %s is uncertain. The request was saved and will be retried automatically.",
		messageServiceError:    "The Minecraft server is unavailable or rejected the command. Your binding was not changed; please try again later.",
		messageMenuAdd:         "Add a Minecraft player\nSend /mcwl add <player_name> in this group. Names must contain 3–16 letters, numbers, or underscores. You may bind up to 2.",
		messageMenuDelete:      "Choose one of your players to remove from both your bindings and the Minecraft whitelist:",
		messageMenuDeleteEmpty: "You have no players to remove. Use /mcwl add <player_name> to add one.",
		messageMenuOwnerOnly:   "This menu belongs to another user. Run /mcwl to open your own.",
		messageMenuInvalid:     "This menu is invalid or expired. Run /mcwl to open a new one.",
		messageMenuClosed:      "Minecraft whitelist menu closed. Run /mcwl to open it again.",
		messageStatusAdding:    "adding; retry pending",
		messageStatusDeleting:  "removing; retry pending",
		messageButtonAdd:       "➕ Add",
		messageButtonList:      "📋 List",
		messageButtonDelete:    "➖ Delete",
		messageButtonBack:      "↩ Back",
		messageButtonClose:     "✖ Close",
	},
	config.LanguageChinese: {
		messageHelp: "Minecraft 白名单\n" +
			"/mcwl — 打开帮助与导航菜单\n" +
			"/mcwl add <玩家名> — 绑定玩家并加入白名单\n" +
			"/mcwl list — 查看已绑定玩家\n" +
			"/mcwl del <玩家名> — 解绑玩家并移出白名单\n\n" +
			"每个 Telegram 账号最多绑定 2 个 Java 版玩家名（3–16 位英文字母、数字或下划线）。",
		messageUsageAdd:        "用法：/mcwl add <玩家名>\n示例：/mcwl add Steve",
		messageUsageDelete:     "用法：/mcwl del <玩家名>\n可先用 /mcwl list 查看已绑定玩家。",
		messageInvalidName:     "玩家名格式无效：请使用 3–16 位英文字母、数字或下划线。",
		messageLimit:           "你已经绑定了 2 个玩家。请先用 /mcwl del <玩家名> 删除一个，再添加新玩家。",
		messageAlreadyBound:    "该 Minecraft 玩家名已经被绑定。",
		messageNotBound:        "该玩家未绑定到你的 Telegram 账号。请用 /mcwl list 查看绑定。",
		messageAddSuccess:      "已将 %s 加入 Minecraft 白名单。已绑定：%d/2。可用 /mcwl list 查看。",
		messageDeleteSuccess:   "已将 %s 从 Minecraft 白名单移除。已绑定：%d/2。可用 /mcwl add <玩家名> 添加其他玩家。",
		messageListEmpty:       "你还没有绑定 Minecraft 玩家。请用 /mcwl add <玩家名> 或 /mcwl 开始。",
		messageListHeader:      "你的 Minecraft 玩家（%d/2）：",
		messagePendingAdd:      "服务器对添加 %s 的执行结果不确定。请求已保存，机器人会自动重试。",
		messagePendingDelete:   "服务器对移除 %s 的执行结果不确定。请求已保存，机器人会自动重试。",
		messageServiceError:    "Minecraft 服务器当前不可用或拒绝了命令。绑定没有改变，请稍后重试。",
		messageMenuAdd:         "添加 Minecraft 玩家\n请在本群发送 /mcwl add <玩家名>。玩家名需为 3–16 位英文字母、数字或下划线，最多绑定 2 个。",
		messageMenuDelete:      "请选择要从绑定和 Minecraft 白名单中移除的玩家：",
		messageMenuDeleteEmpty: "你没有可删除的玩家。请用 /mcwl add <玩家名> 添加。",
		messageMenuOwnerOnly:   "此菜单属于其他用户。请运行 /mcwl 打开自己的菜单。",
		messageMenuInvalid:     "此菜单无效或已过期。请运行 /mcwl 打开新菜单。",
		messageMenuClosed:      "Minecraft 白名单菜单已关闭。可用 /mcwl 再次打开。",
		messageStatusAdding:    "正在添加，等待重试",
		messageStatusDeleting:  "正在移除，等待重试",
		messageButtonAdd:       "➕ 添加",
		messageButtonList:      "📋 查看",
		messageButtonDelete:    "➖ 删除",
		messageButtonBack:      "↩ 返回",
		messageButtonClose:     "✖ 关闭",
	},
}

func translate(language config.Language, key messageKey, arguments ...any) string {
	catalog, ok := messages[language]
	if !ok {
		catalog = messages[config.LanguageEnglish]
	}
	template, ok := catalog[key]
	if !ok {
		template = messages[config.LanguageEnglish][key]
	}
	if len(arguments) == 0 {
		return template
	}
	return fmt.Sprintf(template, arguments...)
}
