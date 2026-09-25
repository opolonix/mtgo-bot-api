package convert

import (
	"encoding/json"
	"fmt"

	"github.com/mtgo-labs/mtgo/tg"

	apitypes "github.com/mtgo-labs/mtgo-bot-api/internal/types"
)

// ReplyMarkup parses a Bot API reply_markup JSON string into a raw
// tg.ReplyMarkupClass suitable for RPC calls. Returns nil for an empty string
// so callers can omit the field.
func ReplyMarkup(raw string) (tg.ReplyMarkupClass, error) {
	if raw == "" {
		return nil, nil
	}
	var markup struct {
		InlineKeyboard        [][]apitypes.InlineKeyboardButton `json:"inline_keyboard"`
		Keyboard              [][]json.RawMessage               `json:"keyboard"`
		ResizeKeyboard        bool                              `json:"resize_keyboard"`
		OneTimeKeyboard       bool                              `json:"one_time_keyboard"`
		IsPersistent          bool                              `json:"is_persistent"`
		Selective             bool                              `json:"selective"`
		InputFieldPlaceholder string                            `json:"input_field_placeholder"`
		RemoveKeyboard        bool                              `json:"remove_keyboard"`
		ForceReply            bool                              `json:"force_reply"`
	}
	if err := json.Unmarshal([]byte(raw), &markup); err != nil {
		return nil, fmt.Errorf("invalid reply_markup JSON: %w", err)
	}
	if markup.InlineKeyboard != nil {
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
	if markup.Keyboard != nil {
		rows := make([]*tg.KeyboardButtonRow, 0, len(markup.Keyboard))
		for rowIndex, botRow := range markup.Keyboard {
			buttons := make([]*tg.KeyboardButton, 0, len(botRow))
			for buttonIndex, rawButton := range botRow {
				button, err := KeyboardButtonFromJSON(string(rawButton))
				if err != nil {
					return nil, fmt.Errorf("keyboard row %d button %d: %w", rowIndex, buttonIndex, err)
				}
				buttons = append(buttons, button)
			}
			rows = append(rows, &tg.KeyboardButtonRow{Buttons: buttons})
		}
		return &tg.ReplyKeyboardMarkup{
			Rows:        rows,
			Resize:      markup.ResizeKeyboard,
			SingleUse:   markup.OneTimeKeyboard,
			Persistent:  markup.IsPersistent,
			Selective:   markup.Selective,
			Placeholder: markup.InputFieldPlaceholder,
		}, nil
	}
	if markup.RemoveKeyboard {
		return &tg.ReplyKeyboardHide{Selective: markup.Selective}, nil
	}
	if markup.ForceReply {
		return &tg.ReplyKeyboardForceReply{
			SingleUse:   markup.OneTimeKeyboard,
			Selective:   markup.Selective,
			Placeholder: markup.InputFieldPlaceholder,
		}, nil
	}
	return nil, fmt.Errorf("unsupported reply_markup type")
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
