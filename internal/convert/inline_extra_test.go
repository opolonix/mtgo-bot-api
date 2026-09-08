package convert

import (
	"encoding/json"
	"testing"

	"github.com/mtgo-labs/mtgo/tg"
)

// --- InlineMessageID encode / decode round trips ----------------------------

func TestInlineMessageIDFromTL_64RoundTrip(t *testing.T) {
	orig := &tg.InputBotInlineMessageID64{DCID: 2, OwnerID: 123, ID: 456, AccessHash: 789}
	s := InlineMessageIDFromTL(orig)
	// Observed: 32-char raw-URL base64.
	if len(s) != 32 {
		t.Fatalf("encoded length = %d, want 32 (got %q)", len(s), s)
	}
	if s != "AgAAAHsAAAAAAAAAyAEAABUDAAAAAAAA" {
		t.Errorf("encoded = %q, want AgAAAHsAAAAAAAAAyAEAABUDAAAAAAAA", s)
	}
	back, err := InlineMessageIDFromStr(s)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	got, ok := back.(*tg.InputBotInlineMessageID64)
	if !ok {
		t.Fatalf("decoded type = %T, want *InputBotInlineMessageID64", back)
	}
	if got.DCID != orig.DCID || got.OwnerID != orig.OwnerID ||
		got.ID != orig.ID || got.AccessHash != orig.AccessHash {
		t.Errorf("round trip mismatch: got %#v, want %#v", got, orig)
	}
}

func TestInlineMessageIDFromTL_Nil(t *testing.T) {
	if got := InlineMessageIDFromTL(nil); got != "" {
		t.Errorf("nil should encode to empty string, got %q", got)
	}
}

func TestInlineMessageIDFromTL_16BitNotDecodable(t *testing.T) {
	// The legacy 16-byte InputBotInlineMessageID can be encoded but the parser
	// has no 16-byte decode branch, so it errors (observed reality).
	orig := &tg.InputBotInlineMessageID{DCID: 1, ID: 100, AccessHash: 200}
	s := InlineMessageIDFromTL(orig)
	if len(s) != 22 {
		t.Fatalf("encoded length = %d, want 22 (got %q)", len(s), s)
	}
	if _, err := InlineMessageIDFromStr(s); err == nil {
		t.Error("16-byte inline_message_id should fail to decode (unsupported format)")
	}
}

func TestInlineMessageIDFromStr_Empty(t *testing.T) {
	if _, err := InlineMessageIDFromStr(""); err == nil {
		t.Error("empty string should error")
	}
}

func TestInlineMessageIDFromStr_InvalidBase64(t *testing.T) {
	if _, err := InlineMessageIDFromStr("@@@@"); err == nil {
		t.Error("invalid base64 should error")
	}
}

func TestDecodeInlineMessageID_DashUnderscoreAndPadding(t *testing.T) {
	// base64 RawURL uses - and _; decodeInlineMessageID must convert them and pad.
	s := "AgAAAHsAAAAAAAAAyAEAABUDAAAAAAAA"
	raw, err := decodeInlineMessageID(s)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(raw) != 24 {
		t.Errorf("decoded length = %d, want 24", len(raw))
	}
}

func TestParseInlineMessageID_TooShort(t *testing.T) {
	if _, err := parseInlineMessageID(make([]byte, 10)); err == nil {
		t.Error("10-byte payload should error (too short)")
	}
}

// --- convertInlineResult dispatch for location/contact/game -----------------

func TestConvertInlineResult_Location(t *testing.T) {
	raw := json.RawMessage(`{"latitude":1.5,"longitude":2.5,"title":"Here","thumb_url":"https://t/thumb.jpg","input_message_content":{"latitude":1.5,"longitude":2.5}}`)
	r, err := convertInlineResult("location", "id1", raw)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	res, ok := r.(*tg.InputBotInlineResult)
	if !ok {
		t.Fatalf("type = %T, want *InputBotInlineResult", r)
	}
	if res.ID != "id1" || res.Type != "location" || res.Title != "Here" {
		t.Errorf("result = %+v", res)
	}
	if res.SendMessage == nil {
		t.Error("SendMessage should be defaulted (ensureInlineMessage)")
	}
	if res.Thumb == nil || res.Thumb.URL != "https://t/thumb.jpg" {
		t.Errorf("Thumb = %+v", res.Thumb)
	}
}

func TestConvertInlineResult_Venue(t *testing.T) {
	raw := json.RawMessage(`{"latitude":3.0,"longitude":4.0,"title":"Spot","input_message_content":{"latitude":3.0,"longitude":4.0,"title":"Spot","address":"Addr"}}`)
	r, err := convertInlineResult("venue", "id2", raw)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if _, ok := r.(*tg.InputBotInlineResult); !ok {
		t.Fatalf("type = %T, want *InputBotInlineResult", r)
	}
}

func TestConvertInlineResult_Contact(t *testing.T) {
	raw := json.RawMessage(`{"phone_number":"+123","first_name":"Bob","last_name":"Lee","thumb_url":"https://t/c.jpg","input_message_content":{"phone_number":"+123","first_name":"Bob"}}`)
	r, err := convertInlineResult("contact", "id3", raw)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	res, ok := r.(*tg.InputBotInlineResult)
	if !ok {
		t.Fatalf("type = %T, want *InputBotInlineResult", r)
	}
	// Title is first_name + " " + last_name.
	if res.Title != "Bob Lee" {
		t.Errorf("Title = %q, want Bob Lee", res.Title)
	}
	if res.Thumb == nil {
		t.Error("Thumb should be set")
	}
}

func TestConvertInlineResult_Game(t *testing.T) {
	raw := json.RawMessage(`{"game_short_name":"mygame"}`)
	r, err := convertInlineResult("game", "id4", raw)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	g, ok := r.(*tg.InputBotInlineResultGame)
	if !ok {
		t.Fatalf("type = %T, want *InputBotInlineResultGame", r)
	}
	if g.ID != "id4" || g.ShortName != "mygame" {
		t.Errorf("game = %+v", g)
	}
}

func TestConvertInlineResult_UnsupportedType(t *testing.T) {
	if _, err := convertInlineResult("mystery", "x", json.RawMessage(`{}`)); err == nil {
		t.Error("unsupported type should error")
	}
}

// --- convertInput*Message ---------------------------------------------------

func TestConvertInputLocationMessage_Full(t *testing.T) {
	raw := json.RawMessage(`{"latitude":1.0,"longitude":2.0,"horizontal_accuracy":10,"live_period":60,"heading":90,"proximity_alert_radius":100}`)
	msg, err := convertInputLocationMessage(raw)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	m, ok := msg.(*tg.InputBotInlineMessageMediaGeo)
	if !ok {
		t.Fatalf("type = %T, want *InputBotInlineMessageMediaGeo", msg)
	}
	if m.GeoPoint == nil {
		t.Fatal("GeoPoint nil")
	}
	gp := m.GeoPoint.(*tg.InputGeoPoint)
	if gp.Lat != 1.0 || gp.Long != 2.0 {
		t.Errorf("GeoPoint = %+v", gp)
	}
	if m.Period != 60 {
		t.Errorf("Period = %d, want 60", m.Period)
	}
	if m.Heading != 90 {
		t.Errorf("Heading = %d, want 90", m.Heading)
	}
	if m.ProximityNotificationRadius != 100 {
		t.Errorf("Proximity = %d, want 100", m.ProximityNotificationRadius)
	}
}

func TestConvertInputVenueMessage(t *testing.T) {
	raw := json.RawMessage(`{"latitude":1.0,"longitude":2.0,"title":"T","address":"A"}`)
	msg, err := convertInputVenueMessage(raw)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	m, ok := msg.(*tg.InputBotInlineMessageMediaVenue)
	if !ok {
		t.Fatalf("type = %T, want *InputBotInlineMessageMediaVenue", msg)
	}
	if m.Title != "T" || m.Address != "A" {
		t.Errorf("venue = %+v", m)
	}
	if m.GeoPoint == nil {
		t.Fatal("GeoPoint nil")
	}
	gp := m.GeoPoint.(*tg.InputGeoPoint)
	if gp.Lat != 1.0 {
		t.Errorf("GeoPoint = %+v", gp)
	}
}

func TestConvertInputContactMessage_WithVCard(t *testing.T) {
	raw := json.RawMessage(`{"phone_number":"+1","first_name":"F","last_name":"L","vcard":"VCARD"}`)
	msg, err := convertInputContactMessage(raw)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	m, ok := msg.(*tg.InputBotInlineMessageMediaContact)
	if !ok {
		t.Fatalf("type = %T, want *InputBotInlineMessageMediaContact", msg)
	}
	if m.PhoneNumber != "+1" || m.FirstName != "F" || m.LastName != "L" {
		t.Errorf("contact = %+v", m)
	}
	if m.Vcard != "VCARD" {
		t.Errorf("Vcard = %q, want VCARD", m.Vcard)
	}
}

func TestConvertInputMessageContent_BadJSON(t *testing.T) {
	// Non-null raw with garbage should error.
	if _, err := convertInputMessageContent(json.RawMessage(`{bad}`)); err == nil {
		t.Error("bad JSON should error")
	}
}

// --- HighScores -------------------------------------------------------------

func TestHighScores_WithAndWithoutUser(t *testing.T) {
	users := map[int64]*tg.User{
		10: {ID: 10, FirstName: "A"},
	}
	scores := []*tg.HighScore{
		{UserID: 10, Pos: 1, Score: 100},
		{UserID: 20, Pos: 2, Score: 50},
	}
	out := HighScores(scores, users)
	if len(out) != 2 {
		t.Fatalf("len = %d, want 2", len(out))
	}
	if out[0].Position != 1 || out[0].Score != 100 {
		t.Errorf("row0 = %+v", out[0])
	}
	if out[0].User == nil || out[0].User.ID != 10 || out[0].User.FirstName != "A" {
		t.Errorf("row0 user = %+v", out[0].User)
	}
	// Uncached user falls back to a stub with just the ID.
	if out[1].User == nil || out[1].User.ID != 20 || out[1].User.FirstName != "" {
		t.Errorf("row1 user = %+v", out[1].User)
	}
}

func TestHighScores_Empty(t *testing.T) {
	out := HighScores(nil, nil)
	if len(out) != 0 {
		t.Errorf("len = %d, want 0", len(out))
	}
}

// --- KeyboardButtonFromJSON -------------------------------------------------

func TestKeyboardButtonFromJSON(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantErr bool
		typeOf  func(*tg.KeyboardButton) bool
	}{
		{
			name: "plain",
			json: `{"text":"Hi"}`,
			typeOf: func(b *tg.KeyboardButton) bool {
				_, ok := b.Type.(*tg.ButtonTypeDefault)
				return ok
			},
		},
		{
			name: "url",
			json: `{"text":"Open","url":"https://x"}`,
			typeOf: func(b *tg.KeyboardButton) bool {
				_, ok := b.Type.(*tg.ButtonTypeDefault)
				return ok
			},
		},
		{
			name: "web_app",
			json: `{"text":"App","web_app":{"url":"https://app"}}`,
			typeOf: func(b *tg.KeyboardButton) bool {
				_, ok := b.Type.(*tg.ButtonTypeSimpleWebView)
				return ok
			},
		},
		{
			name:    "missing_text",
			json:    `{"url":"https://x"}`,
			wantErr: true,
		},
		{
			name:    "bad_json",
			json:    `{bad}`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := KeyboardButtonFromJSON(tt.json)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if !tt.typeOf(b) {
				t.Errorf("unexpected type %T", b)
			}
		})
	}
}
