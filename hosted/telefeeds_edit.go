package hosted

import (
	"context"
	"encoding/json"
	"maps"

	"github.com/mtgo-labs/mtgo/tg"
)

// An empty inline keyboard means removal, not an empty ReplyInlineMarkup TL object.
func normalizeEditReplyMarkup(args map[string]string) map[string]string {
	markup := args["reply_markup"]
	if markup == "" {
		return args
	}
	var keyboard struct {
		InlineKeyboard [][]json.RawMessage `json:"inline_keyboard"`
	}
	if err := json.Unmarshal([]byte(markup), &keyboard); err != nil || keyboard.InlineKeyboard == nil {
		return args
	}
	for _, row := range keyboard.InlineKeyboard {
		if len(row) != 0 {
			return args
		}
	}
	clean := maps.Clone(args)
	delete(clean, "reply_markup")
	return clean
}

type editResultInvoker struct{ tg.Invoker }

// The converter extracts edited messages from Updates; Telegram may send the
// same update in the short or combined envelope.
func (invoker editResultInvoker) RPCInvoke(ctx context.Context, input tg.TLObject, decode func(*tg.Reader) (tg.TLObject, error)) (tg.TLObject, error) {
	result, err := invoker.Invoker.RPCInvoke(ctx, input, decode)
	if err != nil {
		return nil, err
	}
	if _, ok := input.(*tg.MessagesEditMessageRequest); !ok {
		return result, nil
	}
	switch updates := result.(type) {
	case *tg.UpdateShort:
		return &tg.Updates{Updates: []tg.UpdateClass{updates.Update}, Date: updates.Date}, nil
	case *tg.UpdatesCombined:
		return &tg.Updates{Updates: updates.Updates, Users: updates.Users, Chats: updates.Chats, Date: updates.Date, Seq: updates.Seq}, nil
	default:
		return result, nil
	}
}
