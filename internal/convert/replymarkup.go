package convert

import (
	"encoding/json"
	"fmt"

	"github.com/mtgo-labs/mtgo/tg"

	apitypes "github.com/mtgo-labs/mtgo-bot-api/internal/types"
)

// ReplyMarkup parses a Bot API reply_markup JSON string into a raw
// tg.ReplyMarkupClass suitable for RPC calls. Currently supports
// InlineKeyboardMarkup (the most common type for bots). Returns nil for an
// empty string so callers can omit the field.
func ReplyMarkup(raw string) (tg.ReplyMarkupClass, error) {
	if raw == "" {
		return nil, nil
	}
	var markup struct {
		InlineKeyboard [][]apitypes.InlineKeyboardButton `json:"inline_keyboard"`
	}
	if err := json.Unmarshal([]byte(raw), &markup); err != nil {
		return nil, fmt.Errorf("invalid reply_markup JSON: %w", err)
	}
	if markup.InlineKeyboard == nil {
		return nil, nil
	}
	rows := make([]*tg.KeyboardInlineButtonRow, 0, len(markup.InlineKeyboard))
	for _, botRow := range markup.InlineKeyboard {
		buttons := make([]*tg.KeyboardInlineButton, 0, len(botRow))
		for _, btn := range botRow {
			buttons = append(buttons, convertInlineButton(btn))
		}
		rows = append(rows, &tg.KeyboardInlineButtonRow{Buttons: buttons})
	}
	return &tg.ReplyInlineMarkup{Rows: rows}, nil
}

// convertInlineButton maps a Bot API InlineKeyboardButton to a
// tg.KeyboardInlineButton with the matching InlineButtonType.
func convertInlineButton(btn apitypes.InlineKeyboardButton) *tg.KeyboardInlineButton {
	button := &tg.KeyboardInlineButton{Text: btn.Text}
	switch {
	case btn.CallbackData != "":
		button.Type = &tg.InlineButtonTypeCallback{Data: []byte(btn.CallbackData)}
	case btn.URL != "":
		button.Type = &tg.InlineButtonTypeURL{URL: btn.URL}
	case btn.WebApp != nil && btn.WebApp.URL != "":
		button.Type = &tg.InlineButtonTypeWebView{URL: btn.WebApp.URL}
	case btn.SwitchInlineQueryChosenChat != nil:
		button.Type = &tg.InlineButtonTypeSwitchInline{
			Query: btn.SwitchInlineQueryChosenChat.Query,
		}
	case btn.SwitchInlineQueryCurrentChat != "":
		button.Type = &tg.InlineButtonTypeSwitchInline{
			Query:    btn.SwitchInlineQueryCurrentChat,
			SamePeer: true,
		}
	case btn.SwitchInlineQuery != "":
		button.Type = &tg.InlineButtonTypeSwitchInline{Query: btn.SwitchInlineQuery}
	case btn.Pay:
		button.Type = &tg.InlineButtonTypeBuy{}
	}
	return button
}
