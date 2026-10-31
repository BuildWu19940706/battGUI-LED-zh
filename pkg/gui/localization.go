package gui

import (
	_ "embed"
	"encoding/json"
	"strings"
)

// English message templates are the stable keys shared with Objective-C.
//
//go:embed locales/zh-Hans.json
var chineseResource []byte
var chineseMessages = func() map[string]string {
	var messages map[string]string
	if err := json.Unmarshal(chineseResource, &messages); err != nil {
		panic(err)
	}
	return messages
}()
var activeLocale = "en"

func selectLocale(language string) string {
	language = strings.ToLower(strings.ReplaceAll(language, "_", "-"))
	if strings.HasPrefix(language, "zh-hant") {
		return "en"
	}
	if language == "zh" || strings.HasPrefix(language, "zh-hans") || language == "zh-cn" || language == "zh-sg" {
		return "zh-Hans"
	}
	return "en"
}
func translate(locale, message string) string {
	if locale == "zh-Hans" {
		if value, ok := chineseMessages[message]; ok {
			return value
		}
	}
	return message
}
func tr(message string) string { return translate(activeLocale, message) }

func configureLocalization(language string) {
	activeLocale = selectLocale(language)
	resource := []byte("{}")
	if activeLocale == "zh-Hans" {
		resource = chineseResource
	}
	configureNativeLocalization(string(resource))
}
