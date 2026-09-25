package convert

import (
	"testing"

	"github.com/mtgo-labs/mtgo/tg"

	apitypes "github.com/mtgo-labs/mtgo-bot-api/internal/types"
)

func TestConvertInlineButton_AllVariants(t *testing.T) {
	tests := []struct {
		name string
		btn  apitypes.InlineKeyboardButton
		want func(*tg.KeyboardInlineButton) bool
	}{
		{
			name: "callback_data",
			btn:  apitypes.InlineKeyboardButton{Text: "T", CallbackData: "cd"},
			want: func(b *tg.KeyboardInlineButton) bool {
				_, ok := b.Type.(*tg.InlineButtonTypeCallback)
				return ok
			},
		},
		{
			name: "url",
			btn:  apitypes.InlineKeyboardButton{Text: "T", URL: "https://x"},
			want: func(b *tg.KeyboardInlineButton) bool {
				_, ok := b.Type.(*tg.InlineButtonTypeURL)
				return ok
			},
		},
		{
			name: "web_app",
			btn:  apitypes.InlineKeyboardButton{Text: "T", WebApp: &apitypes.WebAppInfo{URL: "https://app"}},
			want: func(b *tg.KeyboardInlineButton) bool {
				_, ok := b.Type.(*tg.InlineButtonTypeWebView)
				return ok
			},
		},
		{
			name: "switch_chosen_chat",
			btn:  apitypes.InlineKeyboardButton{Text: "T", SwitchInlineQueryChosenChat: &apitypes.SwitchInlineQueryChosenChat{Query: "q"}},
			want: func(b *tg.KeyboardInlineButton) bool {
				cb, ok := b.Type.(*tg.InlineButtonTypeSwitchInline)
				return ok && !cb.SamePeer
			},
		},
		{
			name: "switch_current_chat",
			btn:  apitypes.InlineKeyboardButton{Text: "T", SwitchInlineQueryCurrentChat: "cur"},
			want: func(b *tg.KeyboardInlineButton) bool {
				cb, ok := b.Type.(*tg.InlineButtonTypeSwitchInline)
				return ok && cb.SamePeer && cb.Query == "cur"
			},
		},
		{
			name: "switch_inline_query",
			btn:  apitypes.InlineKeyboardButton{Text: "T", SwitchInlineQuery: "all"},
			want: func(b *tg.KeyboardInlineButton) bool {
				cb, ok := b.Type.(*tg.InlineButtonTypeSwitchInline)
				return ok && cb.Query == "all" && !cb.SamePeer
			},
		},
		{
			name: "pay",
			btn:  apitypes.InlineKeyboardButton{Text: "T", Pay: true},
			want: func(b *tg.KeyboardInlineButton) bool {
				_, ok := b.Type.(*tg.InlineButtonTypeBuy)
				return ok
			},
		},
		{
			name: "default",
			btn:  apitypes.InlineKeyboardButton{Text: "T"},
			want: func(b *tg.KeyboardInlineButton) bool { return b.Type == nil },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertInlineButton(tt.btn)
			if !tt.want(got) {
				t.Errorf("unexpected type %T (%+v)", got, got)
			}
		})
	}
}

func TestConvertInlineButton_CallbackDataValue(t *testing.T) {
	b := convertInlineButton(apitypes.InlineKeyboardButton{Text: "Go", CallbackData: "payload"})
	cb, ok := b.Type.(*tg.InlineButtonTypeCallback)
	if !ok {
		t.Fatalf("type = %T", b.Type)
	}
	if b.Text != "Go" || string(cb.Data) != "payload" {
		t.Errorf("callback = %+v", b)
	}
}

func TestReplyMarkup_FullMarkup(t *testing.T) {
	// ReplyMarkup now also exercises convertInlineButton via the public path.
	raw := `{"inline_keyboard":[[{"text":"A","callback_data":"x"},{"text":"B","url":"https://b"}]]}`
	rm, err := ReplyMarkup(raw)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	inline, ok := rm.(*tg.ReplyInlineMarkup)
	if !ok {
		t.Fatalf("type = %T, want *ReplyInlineMarkup", rm)
	}
	if len(inline.Rows) != 1 || len(inline.Rows[0].Buttons) != 2 {
		t.Fatalf("rows/buttons = %+v", inline.Rows)
	}
	if _, ok := inline.Rows[0].Buttons[0].Type.(*tg.InlineButtonTypeCallback); !ok {
		t.Errorf("button0 type = %T", inline.Rows[0].Buttons[0].Type)
	}
}

func TestReplyMarkup_InvalidJSON(t *testing.T) {
	if _, err := ReplyMarkup("{bad}"); err == nil {
		t.Error("invalid JSON should error")
	}
}

func TestReplyMarkup_ContactKeyboard(t *testing.T) {
	raw := `{"keyboard":[[{"text":"Поделиться своим контактом","request_contact":true}]],"resize_keyboard":true,"one_time_keyboard":true,"input_field_placeholder":"+79991234567"}`
	markup, err := ReplyMarkup(raw)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	keyboard, ok := markup.(*tg.ReplyKeyboardMarkup)
	if !ok {
		t.Fatalf("type = %T, want *ReplyKeyboardMarkup", markup)
	}
	if !keyboard.Resize || !keyboard.SingleUse || keyboard.Placeholder != "+79991234567" {
		t.Fatalf("keyboard options = %+v", keyboard)
	}
	if len(keyboard.Rows) != 1 || len(keyboard.Rows[0].Buttons) != 1 {
		t.Fatalf("keyboard rows = %+v", keyboard.Rows)
	}
	if _, ok := keyboard.Rows[0].Buttons[0].Type.(*tg.ButtonTypeRequestPhone); !ok {
		t.Fatalf("button type = %T, want *ButtonTypeRequestPhone", keyboard.Rows[0].Buttons[0].Type)
	}
	request := &tg.MessagesSendMessageRequest{ReplyMarkup: markup}
	request.SetFlags()
	if !request.Flags.Has(2) {
		t.Fatal("messages.sendMessage must include reply_markup flag")
	}
}

func TestReplyMarkup_KeyboardVariants(t *testing.T) {
	markup, err := ReplyMarkup(`{"keyboard":[["Текст",{"text":"Геопозиция","request_location":true},{"text":"Опрос","request_poll":{"type":"quiz"}},{"text":"Сайт","web_app":{"url":"https://example.com"}}]],"is_persistent":true,"selective":true}`)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	keyboard := markup.(*tg.ReplyKeyboardMarkup)
	if !keyboard.Persistent || !keyboard.Selective {
		t.Fatalf("keyboard options = %+v", keyboard)
	}
	buttons := keyboard.Rows[0].Buttons
	if _, ok := buttons[0].Type.(*tg.ButtonTypeDefault); !ok {
		t.Errorf("plain button type = %T", buttons[0].Type)
	}
	if _, ok := buttons[1].Type.(*tg.ButtonTypeRequestGeoLocation); !ok {
		t.Errorf("location button type = %T", buttons[1].Type)
	}
	poll, ok := buttons[2].Type.(*tg.ButtonTypeRequestPoll)
	if !ok || !poll.Quiz || !poll.Flags.Has(0) {
		t.Errorf("poll button type = %T", buttons[2].Type)
	}
	if _, ok := buttons[3].Type.(*tg.ButtonTypeSimpleWebView); !ok {
		t.Errorf("web app button type = %T", buttons[3].Type)
	}
}

func TestReplyMarkup_RemoveAndForceReply(t *testing.T) {
	removed, err := ReplyMarkup(`{"remove_keyboard":true,"selective":true}`)
	if err != nil {
		t.Fatalf("remove keyboard: %v", err)
	}
	hide, ok := removed.(*tg.ReplyKeyboardHide)
	if !ok || !hide.Selective {
		t.Fatalf("remove keyboard type = %T, value = %+v", removed, removed)
	}
	forced, err := ReplyMarkup(`{"force_reply":true,"selective":true,"input_field_placeholder":"Ответ"}`)
	if err != nil {
		t.Fatalf("force reply: %v", err)
	}
	force, ok := forced.(*tg.ReplyKeyboardForceReply)
	if !ok || !force.Selective || force.Placeholder != "Ответ" {
		t.Fatalf("force reply type = %T, value = %+v", forced, forced)
	}
}

func TestReplyMarkup_UnsupportedKeyboardFails(t *testing.T) {
	if _, err := ReplyMarkup(`{"keyboard":[[{"text":"Пользователи","request_users":{"request_id":1}}]]}`); err == nil {
		t.Fatal("unsupported button must not be sent as a plain text button")
	}
	if _, err := ReplyMarkup(`{"unknown_markup":true}`); err == nil {
		t.Fatal("unsupported reply markup must not be silently dropped")
	}
}
