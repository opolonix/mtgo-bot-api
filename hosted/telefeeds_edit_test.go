package hosted

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mtgo-labs/mtgo/tg"
)

func TestEditMessageReplyMarkupRemovesInlineKeyboard(t *testing.T) {
	const button = `{"inline_keyboard":[[{"text":"Open","url":"https://example.com"}]]}`
	var markup tg.ReplyMarkupClass
	var editCount int
	invoker := tg.InvokerFunc(func(_ context.Context, input tg.TLObject, _ func(*tg.Reader) (tg.TLObject, error)) (tg.TLObject, error) {
		switch request := input.(type) {
		case *tg.MessagesSendMessageRequest:
			if request.ReplyMarkup == nil {
				t.Fatal("send must contain an inline keyboard")
			}
			markup = request.ReplyMarkup
			message := &tg.Message{ID: 7, PeerID: &tg.PeerUser{UserID: 42}, Date: 1_700_000_000, Message: "hello", ReplyMarkup: markup}
			return &tg.Updates{Updates: []tg.UpdateClass{&tg.UpdateNewMessage{Message: message}}}, nil
		case *tg.MessagesEditMessageRequest:
			editCount++
			if editCount == 2 && request.ReplyMarkup == nil {
				t.Fatal("nonempty keyboard edit lost its markup")
			}
			if editCount != 2 && request.ReplyMarkup != nil {
				t.Fatal("keyboard removal must omit the TL reply_markup field")
			}
			markup = request.ReplyMarkup
			message := &tg.Message{ID: request.ID, PeerID: &tg.PeerUser{UserID: 42}, Date: 1_700_000_000, Message: "hello", ReplyMarkup: markup}
			return &tg.UpdateShort{Update: &tg.UpdateEditMessage{Message: message}, Date: message.Date}, nil
		default:
			t.Fatalf("unexpected TL request %T", input)
			return nil, nil
		}
	})
	converter, err := New("123", invoker, emptyPeers{})
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct {
		method string
		args   map[string]string
		button bool
	}{
		{"sendMessage", map[string]string{"chat_id": "42", "text": "hello", "reply_markup": button}, true},
		{"editMessageReplyMarkup", map[string]string{"chat_id": "42", "message_id": "7"}, false},
		{"editMessageReplyMarkup", map[string]string{"chat_id": "42", "message_id": "7", "reply_markup": button}, true},
		{"editMessageReplyMarkup", map[string]string{"chat_id": "42", "message_id": "7", "reply_markup": `{"inline_keyboard":[]}`}, false},
	} {
		status, body := converter.Invoke(context.Background(), step.method, step.args, nil)
		if status != 200 {
			t.Fatalf("%s status=%d body=%s", step.method, status, body)
		}
		var response struct {
			OK     bool `json:"ok"`
			Result struct {
				MessageID   int32           `json:"message_id"`
				Text        string          `json:"text"`
				ReplyMarkup json.RawMessage `json:"reply_markup"`
			} `json:"result"`
		}
		if err := json.Unmarshal(body, &response); err != nil {
			t.Fatal(err)
		}
		if !response.OK || response.Result.MessageID != 7 || response.Result.Text != "hello" || (len(response.Result.ReplyMarkup) != 0) != step.button {
			t.Fatalf("%s returned wrong message: %s", step.method, body)
		}
	}
	if editCount != 3 {
		t.Fatalf("expected 3 edits, got %d", editCount)
	}
}
