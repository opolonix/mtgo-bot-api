package client

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/mtgo-labs/mtgo/tg"
)

func TestConvertRawUpdates(t *testing.T) {
	converter := &Client{botID: "42", msgs: newMsgCache(10)}
	raw := &tg.UpdateShortMessage{
		ID:       7,
		UserID:   11,
		Message:  "hello",
		Date:     1_700_000_000,
		PTS:      1,
		PTSCount: 1,
	}
	var encoded bytes.Buffer
	if err := raw.Encode(&encoded); err != nil {
		t.Fatal(err)
	}
	updates, err := converter.ConvertRawUpdates(encoded.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(updates) != 1 {
		t.Fatalf("got %d updates, want 1", len(updates))
	}
	var update map[string]any
	if err := json.Unmarshal(updates[0], &update); err != nil {
		t.Fatal(err)
	}
	if update["update_id"] != float64(1) {
		t.Fatalf("update_id = %v, want 1", update["update_id"])
	}
	message := update["message"].(map[string]any)
	if message["text"] != "hello" {
		t.Fatalf("text = %v, want hello", message["text"])
	}
}
