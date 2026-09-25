package client

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	apitypes "github.com/mtgo-labs/mtgo-bot-api/internal/types"
	"github.com/mtgo-labs/mtgo/tg"
)

type callbackMessageStore struct {
	active   bool
	messages map[[2]int64]*apitypes.Message
}

func (store *callbackMessageStore) GetMessage(_ context.Context, chatID, messageID int64) (*apitypes.Message, bool, error) {
	return store.messages[[2]int64{chatID, messageID}], store.active, nil
}

func (store *callbackMessageStore) PutMessage(_ context.Context, message *apitypes.Message) error {
	store.messages[[2]int64{message.Chat.ID, message.MessageID}] = message
	return nil
}

func (store *callbackMessageStore) DeleteMessage(_ context.Context, chatID, messageID int64) error {
	if chatID == 0 {
		for key := range store.messages {
			if key[1] == messageID {
				delete(store.messages, key)
			}
		}
	} else {
		delete(store.messages, [2]int64{chatID, messageID})
	}
	return nil
}

func TestHostedCallbackMessageLifecycle(t *testing.T) {
	store := &callbackMessageStore{active: true, messages: make(map[[2]int64]*apitypes.Message)}
	client := &Client{botID: "42", hostedMessages: store}
	original := &tg.Message{
		ID: 7, Date: 1_700_000_000, Message: "before", PeerID: &tg.PeerUser{UserID: 99},
		Entities:    []tg.MessageEntityClass{&tg.MessageEntityBold{Offset: 0, Length: 6}},
		ReplyMarkup: &tg.ReplyInlineMarkup{Rows: []*tg.KeyboardInlineButtonRow{{Buttons: []*tg.KeyboardInlineButton{{Text: "Before", Type: &tg.InlineButtonTypeCallback{Data: []byte("before")}}}}}},
	}
	if _, err := client.finishSend(context.Background(), &tg.Updates{Updates: []tg.UpdateClass{&tg.UpdateNewMessage{Message: original}}}, &tg.InputPeerUser{UserID: 99}, "before"); err != nil {
		t.Fatal(err)
	}
	callback := &tg.UpdateBotCallbackQuery{QueryID: 11, UserID: 99, Peer: &tg.PeerUser{UserID: 99}, MsgID: 7, Data: []byte("before")}
	read := func(update tg.UpdateClass) map[string]any {
		t.Helper()
		var body bytes.Buffer
		if err := tg.EncodeTLObject(&body, &tg.Updates{Updates: []tg.UpdateClass{update}}); err != nil {
			t.Fatal(err)
		}
		encoded, _, err := client.ConvertRawUpdatesWithPeers(body.Bytes())
		if err != nil || len(encoded) != 1 {
			t.Fatalf("convert callback: %v, %d updates", err, len(encoded))
		}
		var output map[string]any
		if err := json.Unmarshal(encoded[0], &output); err != nil {
			t.Fatal(err)
		}
		if update == callback {
			t.Logf("callback_json=%s", encoded[0])
		}
		return output
	}
	message := read(callback)["callback_query"].(map[string]any)["message"].(map[string]any)
	if message["date"] != float64(original.Date) || message["text"] != "before" || message["reply_markup"] == nil || message["entities"] == nil {
		t.Fatalf("sent message was not restored: %#v", message)
	}
	buttons := message["reply_markup"].(map[string]any)["inline_keyboard"].([]any)[0].([]any)
	if buttons[0].(map[string]any)["text"] != "Before" {
		t.Fatalf("sent keyboard is wrong: %#v", buttons)
	}

	client.rpc = &fakeRPC{MessagesEditMessageFn: func(context.Context, *tg.MessagesEditMessageRequest) (tg.UpdatesClass, error) {
		return &tg.Updates{Updates: []tg.UpdateClass{&tg.UpdateEditMessage{Message: &tg.Message{
			ID: 7, Date: original.Date, EditDate: original.Date + 10, Message: "after", PeerID: &tg.PeerUser{UserID: 99},
			ReplyMarkup: &tg.ReplyInlineMarkup{Rows: []*tg.KeyboardInlineButtonRow{{Buttons: []*tg.KeyboardInlineButton{{Text: "After", Type: &tg.InlineButtonTypeCallback{Data: []byte("after")}}}}}},
		}}}}, nil
	}}
	if _, err := client.invokeEdit(context.Background(), &tg.MessagesEditMessageRequest{}, ""); err != nil {
		t.Fatal(err)
	}
	message = read(callback)["callback_query"].(map[string]any)["message"].(map[string]any)
	if message["text"] != "after" || message["edit_date"] != float64(original.Date+10) {
		t.Fatalf("edited message was not restored: %#v", message)
	}
	buttons = message["reply_markup"].(map[string]any)["inline_keyboard"].([]any)[0].([]any)
	if buttons[0].(map[string]any)["text"] != "After" {
		t.Fatalf("edited keyboard is wrong: %#v", buttons)
	}

	delete(store.messages, [2]int64{99, 7})
	client.rpc = &fakeRPC{MessagesGetMessagesFn: func(context.Context, *tg.MessagesGetMessagesRequest) (tg.MessagesClass, error) {
		return &tg.MessagesMessages{Messages: []tg.MessageClass{original}}, nil
	}}
	message = read(callback)["callback_query"].(map[string]any)["message"].(map[string]any)
	if message["text"] != "before" {
		t.Fatalf("cache miss was not restored from TL: %#v", message)
	}
	delete(store.messages, [2]int64{99, 7})
	client.rpc = &fakeRPC{MessagesGetMessagesFn: func(context.Context, *tg.MessagesGetMessagesRequest) (tg.MessagesClass, error) {
		return &tg.MessagesMessages{Messages: []tg.MessageClass{&tg.MessageEmpty{ID: 7}}}, nil
	}}
	message = read(callback)["callback_query"].(map[string]any)["message"].(map[string]any)
	if message["date"] != float64(0) || message["text"] != nil {
		t.Fatalf("deleted message must be inaccessible: %#v", message)
	}
	store.active = false
	client.rpc = &fakeRPC{MessagesGetMessagesFn: func(context.Context, *tg.MessagesGetMessagesRequest) (tg.MessagesClass, error) {
		t.Fatal("inactive Bot API subscription must not fetch TL message")
		return nil, nil
	}}
	message = read(callback)["callback_query"].(map[string]any)["message"].(map[string]any)
	if message["date"] != float64(0) {
		t.Fatalf("inactive subscription must return inaccessible message: %#v", message)
	}
	inline := read(&tg.UpdateInlineBotCallbackQuery{QueryID: 12, UserID: 99, MsgID: &tg.InputBotInlineMessageID{DCID: 1, ID: 2, AccessHash: 3}})["callback_query"].(map[string]any)
	if inline["inline_message_id"] == nil || inline["message"] != nil {
		t.Fatalf("inline callback was modified: %#v", inline)
	}
}

func TestHostedCallbackMessagesStayInBotSession(t *testing.T) {
	first := &callbackMessageStore{active: true, messages: make(map[[2]int64]*apitypes.Message)}
	second := &callbackMessageStore{active: true, messages: make(map[[2]int64]*apitypes.Message)}
	first.PutMessage(context.Background(), &apitypes.Message{MessageID: 7, Chat: apitypes.Chat{ID: 99}, Date: 1, Text: "first"})
	second.PutMessage(context.Background(), &apitypes.Message{MessageID: 7, Chat: apitypes.Chat{ID: 99}, Date: 1, Text: "second"})
	for _, test := range []struct {
		botID string
		store *callbackMessageStore
		want  string
	}{{"42", first, "first"}, {"43", second, "second"}} {
		client := &Client{botID: test.botID, hostedMessages: test.store}
		var body bytes.Buffer
		if err := tg.EncodeTLObject(&body, &tg.Updates{Updates: []tg.UpdateClass{&tg.UpdateBotCallbackQuery{QueryID: 1, UserID: 99, Peer: &tg.PeerUser{UserID: 99}, MsgID: 7}}}); err != nil {
			t.Fatal(err)
		}
		encoded, _, err := client.ConvertRawUpdatesWithPeers(body.Bytes())
		if err != nil {
			t.Fatal(err)
		}
		var output struct {
			CallbackQuery struct {
				Message apitypes.Message `json:"message"`
			} `json:"callback_query"`
		}
		if err := json.Unmarshal(encoded[0], &output); err != nil {
			t.Fatal(err)
		}
		if output.CallbackQuery.Message.Text != test.want {
			t.Fatalf("bot %s saw %q, want %q", test.botID, output.CallbackQuery.Message.Text, test.want)
		}
	}
}
