package mcwhitelist

import (
	"math"
	"strings"
	"testing"

	"forge.asnk.io/sugar/nekomonogatari-bot/config"
)

func TestParseMCWLCommand(t *testing.T) {
	tests := []struct {
		name       string
		arguments  []string
		kind       commandKind
		playerName string
		usage      messageKey
	}{
		{name: "help", kind: commandHelp},
		{name: "add", arguments: []string{"ADD", "Steve"}, kind: commandAdd, playerName: "Steve"},
		{name: "add missing", arguments: []string{"add"}, kind: commandHelp, usage: messageUsageAdd},
		{name: "add extra", arguments: []string{"add", "Steve", "extra"}, kind: commandHelp, usage: messageUsageAdd},
		{name: "list", arguments: []string{"LiSt"}, kind: commandList},
		{name: "list extra", arguments: []string{"list", "extra"}, kind: commandHelp},
		{name: "delete", arguments: []string{"DEL", "Steve"}, kind: commandDelete, playerName: "Steve"},
		{name: "delete missing", arguments: []string{"del"}, kind: commandHelp, usage: messageUsageDelete},
		{name: "help", arguments: []string{"HeLp"}, kind: commandHelp},
		{name: "unknown", arguments: []string{"menu"}, kind: commandHelp},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := parseMCWLCommand(test.arguments)
			if got.Kind != test.kind || got.PlayerName != test.playerName || got.UsageKey != test.usage {
				t.Fatalf("parseMCWLCommand(%v) = %#v", test.arguments, got)
			}
		})
	}
}

func TestPlayerNameValidation(t *testing.T) {
	for _, name := range []string{"abc", "Steve", "player_name_1234", "A123456789012345"} {
		if !validPlayerName(name) {
			t.Errorf("validPlayerName(%q) = false", name)
		}
	}
	for _, name := range []string{"", "ab", "A1234567890123456", "has space", "semi;colon", "line\nbreak", "玩家"} {
		if validPlayerName(name) {
			t.Errorf("validPlayerName(%q) = true", name)
		}
	}
}

func TestCallbackDataRoundTripAndOwnerBinding(t *testing.T) {
	tests := []struct {
		value string
		want  callbackAction
	}{
		{"mcwl|123|add", callbackAction{OwnerID: 123, Action: "add"}},
		{"mcwl|123|list", callbackAction{OwnerID: 123, Action: "list"}},
		{"mcwl|123|delete", callbackAction{OwnerID: 123, Action: "delete"}},
		{"mcwl|123|rm|456", callbackAction{OwnerID: 123, Action: "rm", BindingID: 456}},
		{"mcwl|123|back", callbackAction{OwnerID: 123, Action: "back"}},
		{"mcwl|123|close", callbackAction{OwnerID: 123, Action: "close"}},
	}
	for _, test := range tests {
		got, err := parseCallbackData(test.value)
		if err != nil || got != test.want {
			t.Errorf("parseCallbackData(%q) = %#v, %v; want %#v", test.value, got, err, test.want)
		}
	}
	for _, invalid := range []string{
		"", "mcwl", "other|1|add", "mcwl|0|add", "mcwl|-1|add", "mcwl|abc|add",
		"mcwl|1|unknown", "mcwl|1|rm", "mcwl|1|rm|0", "mcwl|1|add|4", "mcwl|1|rm|4|extra",
	} {
		if _, err := parseCallbackData(invalid); err == nil {
			t.Errorf("parseCallbackData(%q) unexpectedly succeeded", invalid)
		}
	}
	longest := callbackData(math.MaxInt64, "rm", math.MaxInt64)
	if len(longest) > 64 {
		t.Fatalf("callback data is %d bytes, Telegram maximum is 64: %q", len(longest), longest)
	}
}

func TestTranslationCatalogsHaveMatchingNonemptyKeys(t *testing.T) {
	english := messages[config.LanguageEnglish]
	chinese := messages[config.LanguageChinese]
	if len(english) != len(chinese) {
		t.Fatalf("catalog sizes differ: en=%d zh=%d", len(english), len(chinese))
	}
	for key, englishText := range english {
		if strings.TrimSpace(englishText) == "" {
			t.Errorf("English key %q is empty", key)
		}
		if strings.TrimSpace(chinese[key]) == "" {
			t.Errorf("Chinese key %q is missing or empty", key)
		}
	}
	for key := range chinese {
		if _, ok := english[key]; !ok {
			t.Errorf("Chinese catalog has extra key %q", key)
		}
	}
}

func TestLocalizedInstructionsAndPendingStatus(t *testing.T) {
	for _, language := range []config.Language{config.LanguageEnglish, config.LanguageChinese} {
		p := &plugin{language: language}
		help := p.text(messageHelp)
		for _, command := range []string{"/mcwl", "/mcwl add", "/mcwl list", "/mcwl del"} {
			if !strings.Contains(help, command) {
				t.Errorf("%s help does not include %q", language, command)
			}
		}
		list := p.formatList([]binding{{PlayerName: "Steve", State: statePendingAdd}})
		if !strings.Contains(list, "Steve") || !strings.Contains(list, p.text(messageStatusAdding)) {
			t.Errorf("%s pending list = %q", language, list)
		}
	}
}
