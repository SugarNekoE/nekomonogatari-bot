package mcstatus

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"forge.asnk.io/sugar/nekomonogatari-bot/config"
)

type labels struct {
	address, players, motd, unknown, empty, unavailable, usage string
}

var catalog = map[config.Language]labels{
	config.LanguageEnglish: {
		address: "Server address", players: "Players", motd: "MOTD", unknown: "Unknown", empty: "(empty)",
		unavailable: "Server status unavailable. The server may be offline or unreachable.",
		usage:       "Usage: /mc or /mc <server_address>\nUse a hostname or IP address, optionally with a port (host:25565 or [IPv6]:25565).",
	},
	config.LanguageChinese: {
		address: "服务器地址", players: "在线人数", motd: "MOTD", unknown: "未知", empty: "（空）",
		unavailable: "无法获取服务器状态，服务器可能已离线或无法连接。",
		usage:       "用法：/mc 或 /mc <服务器地址>\n支持域名或 IP 地址，可指定端口（host:25565 或 [IPv6]:25565）。",
	},
}

func formatStatus(language config.Language, name, address string, result status) string {
	l := catalog[language]
	players := l.unknown
	if result.Players != nil && result.Players.Online != nil && result.Players.Max != nil {
		players = fmt.Sprintf("%d/%d", *result.Players.Online, *result.Players.Max)
	}
	var component any
	_ = json.Unmarshal(result.Description, &component)
	motd := plainText(componentText(component, 0), 1400)
	if strings.TrimSpace(motd) == "" {
		motd = l.empty
	}
	return fmt.Sprintf("%s\n%s: %s\n%s: %s\n%s:\n%s", plainText(name, 200), l.address, address, l.players, players, l.motd, motd)
}

// A server description can be a string, an array, or a nested chat component.
func componentText(component any, depth int) string {
	if depth > 32 {
		return ""
	}
	var text strings.Builder
	switch value := component.(type) {
	case string:
		return value
	case []any:
		for _, child := range value {
			text.WriteString(componentText(child, depth+1))
		}
	case map[string]any:
		if valueText, ok := value["text"].(string); ok {
			text.WriteString(valueText)
		} else if fallback, ok := value["fallback"].(string); ok {
			text.WriteString(fallback)
		} else if translation, ok := value["translate"].(string); ok {
			text.WriteString(translation)
		}
		text.WriteString(componentText(value["extra"], depth+1))
	}
	return text.String()
}

// Strip Minecraft legacy color/style codes and controls. Rune limits leave
// room below Telegram's UTF-16 message limit even for supplementary characters.
func plainText(text string, limit int) string {
	runes := []rune(text)
	var result []rune
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == '§' && i+1 < len(runes) && strings.ContainsRune("0123456789abcdefklmnorx", unicode.ToLower(runes[i+1])) {
			i++
			continue
		}
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			continue
		}
		if len(result) == limit {
			return string(result) + "…"
		}
		result = append(result, r)
	}
	return string(result)
}
