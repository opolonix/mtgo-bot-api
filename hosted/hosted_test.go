package hosted

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/mtgo-labs/mtgo/tg"
)

type emptyPeers struct{}

func (emptyPeers) SavePeer(context.Context, Peer) error         { return nil }
func (emptyPeers) GetPeer(context.Context, int64) (Peer, error) { return Peer{}, sql.ErrNoRows }
func (emptyPeers) GetPeerByUsername(context.Context, string) (Peer, error) {
	return Peer{}, sql.ErrNoRows
}
func (emptyPeers) SaveChatFlags(context.Context, int64, bool, bool, string) error { return nil }
func (emptyPeers) SaveBotMemberStatus(context.Context, int64, string) error       { return nil }

func TestConvertWithoutGoDeliveryState(t *testing.T) {
	invoker := tg.InvokerFunc(func(context.Context, tg.TLObject, func(*tg.Reader) (tg.TLObject, error)) (tg.TLObject, error) {
		t.Fatal("conversion must not invoke Telegram")
		return nil, nil
	})
	converter, err := New("42", invoker, emptyPeers{})
	if err != nil {
		t.Fatal(err)
	}
	var raw bytes.Buffer
	if err := (&tg.UpdateShortMessage{ID: 7, UserID: 11, Message: "hello", Date: 1_700_000_000, PTS: 1, PTSCount: 1}).Encode(&raw); err != nil {
		t.Fatal(err)
	}
	updates, peers, err := converter.ConvertRawUpdates(raw.Bytes())
	if err != nil || len(updates) != 1 {
		t.Fatalf("updates=%d err=%v", len(updates), err)
	}
	if len(peers) != 0 {
		t.Fatalf("unexpected peers: %v", peers)
	}
	var update map[string]any
	if err := json.Unmarshal(updates[0], &update); err != nil {
		t.Fatal(err)
	}
	if _, ok := update["update_id"]; ok {
		t.Fatal("update_id must be assigned by the host")
	}
}
