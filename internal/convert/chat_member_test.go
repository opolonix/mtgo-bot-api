package convert

import (
	"encoding/json"
	"testing"

	"github.com/mtgo-labs/mtgo/tg"

	apitypes "github.com/mtgo-labs/mtgo-bot-api/internal/types"
)

// --- ChatMemberFromParticipant ---------------------------------------------

func TestChatMemberFromParticipant_Nil(t *testing.T) {
	if ChatMemberFromParticipant(nil, nil) != nil {
		t.Error("nil should return nil")
	}
}

func TestChatMemberFromParticipant_AllVariants(t *testing.T) {
	users := map[int64]*tg.User{10: {ID: 10, FirstName: "A"}}
	tests := []struct {
		name     string
		part     tg.ChannelParticipantClass
		status   string
		isMember bool
	}{
		{"normal", &tg.ChannelParticipant{UserID: 10}, "member", true},
		{"self", &tg.ChannelParticipantSelf{UserID: 10}, "member", true},
		{"left", &tg.ChannelParticipantLeft{Peer: &tg.PeerUser{UserID: 10}}, "left", false},
		{"creator", &tg.ChannelParticipantCreator{UserID: 10}, "creator", true},
		{"admin", &tg.ChannelParticipantAdmin{UserID: 10}, "administrator", true},
		{
			"restricted",
			&tg.ChannelParticipantBanned{
				Left:         false,
				Peer:         &tg.PeerUser{UserID: 10},
				BannedRights: &tg.ChatBannedRights{},
			},
			"restricted", true,
		},
		{
			"kicked",
			&tg.ChannelParticipantBanned{
				Left:         true,
				Peer:         &tg.PeerUser{UserID: 10},
				BannedRights: &tg.ChatBannedRights{},
			},
			"kicked", false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := ChatMemberFromParticipant(tt.part, users)
			if m == nil {
				t.Fatal("nil member")
			}
			if m.Status != tt.status {
				t.Errorf("Status = %q, want %q", m.Status, tt.status)
			}
			if m.IsMember != tt.isMember {
				t.Errorf("IsMember = %v, want %v", m.IsMember, tt.isMember)
			}
			if m.User == nil || m.User.ID != 10 {
				t.Errorf("User = %+v", m.User)
			}
		})
	}
}

func TestChatMemberFromParticipant_AdminRights(t *testing.T) {
	part := &tg.ChannelParticipantAdmin{
		UserID:      5,
		CanEdit:     true,
		Rank:        "Owner",
		AdminRights: &tg.ChatAdminRights{DeleteMessages: true, Anonymous: true},
	}
	m := ChatMemberFromParticipant(part, nil)
	if m.Status != "administrator" {
		t.Errorf("Status = %q", m.Status)
	}
	if !m.CanBeEdited {
		t.Error("CanBeEdited should be true")
	}
	if m.CustomTitle != "Owner" {
		t.Errorf("CustomTitle = %q", m.CustomTitle)
	}
	if !m.CanDeleteMessages {
		t.Error("CanDeleteMessages should be true")
	}
	if !m.IsAnonymous {
		t.Error("IsAnonymous should be true")
	}
}

func TestChatMemberFromParticipant_WelcomeMessages(t *testing.T) {
	for _, allowed := range []bool{false, true} {
		member := ChatMemberFromParticipant(&tg.ChannelParticipantAdmin{
			UserID: 5, AdminRights: &tg.ChatAdminRights{ManageWelcomeMessages: allowed},
		}, nil)
		if member.CanSendWelcomeMessages != allowed {
			t.Fatalf("CanSendWelcomeMessages = %v, want %v", member.CanSendWelcomeMessages, allowed)
		}
		encoded, err := json.Marshal(member)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]any
		if err := json.Unmarshal(encoded, &fields); err != nil {
			t.Fatal(err)
		}
		if got, ok := fields["can_send_welcome_messages"]; !ok || got != allowed {
			t.Errorf("can_send_welcome_messages = %v (present: %v), want %v: %s", got, ok, allowed, encoded)
		}
	}
}

func TestChatMemberFromParticipant_CreatorAnonymous(t *testing.T) {
	part := &tg.ChannelParticipantCreator{
		UserID:      1,
		Rank:        "Founder",
		AdminRights: &tg.ChatAdminRights{Anonymous: true},
	}
	m := ChatMemberFromParticipant(part, nil)
	if m.Status != "creator" {
		t.Errorf("Status = %q", m.Status)
	}
	if !m.IsAnonymous {
		t.Error("creator should be anonymous")
	}
	if m.CustomTitle != "Founder" {
		t.Errorf("CustomTitle = %q", m.CustomTitle)
	}
}

func TestChatMemberFromParticipant_BannedUntilDate(t *testing.T) {
	part := &tg.ChannelParticipantBanned{
		Left:         false,
		Peer:         &tg.PeerUser{UserID: 3},
		BannedRights: &tg.ChatBannedRights{UntilDate: 9999},
	}
	m := ChatMemberFromParticipant(part, nil)
	if m.UntilDate != 9999 {
		t.Errorf("UntilDate = %d, want 9999", m.UntilDate)
	}
}


// --- ChatMemberFromChatParticipant -----------------------------------------

func TestChatMemberFromChatParticipant(t *testing.T) {
	users := map[int64]*tg.User{10: {ID: 10, FirstName: "A"}}
	tests := []struct {
		name   string
		part   tg.ChatParticipantClass
		status string
	}{
		{"nil", nil, ""},
		{"member", &tg.ChatParticipant{UserID: 10}, "member"},
		{"creator", &tg.ChatParticipantCreator{UserID: 10, Rank: "F"}, "creator"},
		{"admin", &tg.ChatParticipantAdmin{UserID: 10, Rank: "X"}, "administrator"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := ChatMemberFromChatParticipant(tt.part, users)
			if tt.status == "" {
				if m != nil {
					t.Errorf("expected nil, got %+v", m)
				}
				return
			}
			if m == nil {
				t.Fatal("nil member")
			}
			if m.Status != tt.status {
				t.Errorf("Status = %q, want %q", m.Status, tt.status)
			}
			if !m.IsMember {
				t.Error("basic group members should be IsMember=true")
			}
		})
	}
}

// --- chatReactionsToBotAPI -------------------------------------------------

func TestChatReactionsToBotAPI(t *testing.T) {
	if out := chatReactionsToBotAPI(nil); out != nil {
		t.Error("nil should return nil")
	}
	if out := chatReactionsToBotAPI(&tg.ChatReactionsNone{}); out != nil {
		t.Error("ChatReactionsNone should return nil")
	}
	if out := chatReactionsToBotAPI(&tg.ChatReactionsAll{}); out != nil {
		t.Error("ChatReactionsAll should return nil")
	}
	out := chatReactionsToBotAPI(&tg.ChatReactionsSome{
		Reactions: []tg.ReactionClass{
			&tg.ReactionEmoji{Emoticon: "🔥"},
			&tg.ReactionCustomEmoji{DocumentID: 42},
		},
	})
	if len(out) != 2 {
		t.Fatalf("len = %d, want 2", len(out))
	}
	if out[0].Type != "emoji" || out[0].Emoji != "🔥" {
		t.Errorf("reaction0 = %+v", out[0])
	}
	if out[1].Type != "custom_emoji" || out[1].CustomEmojiID != "42" {
		t.Errorf("reaction1 = %+v", out[1])
	}
}

// --- channelLocationToBotAPI -----------------------------------------------

func TestChannelLocationToBotAPI(t *testing.T) {
	if out := channelLocationToBotAPI(nil); out != nil {
		t.Error("nil should return nil")
	}
	// With GeoPoint.
	out := channelLocationToBotAPI(&tg.ChannelLocation{
		Address: "123 St",
		GeoPoint: &tg.GeoPoint{Lat: 1.5, Long: 2.5},
	})
	if out == nil || out.Address != "123 St" {
		t.Errorf("location with geo = %+v", out)
	}
	if out.Location == nil {
		t.Error("Location should be set")
	}
	// Without GeoPoint.
	out2 := channelLocationToBotAPI(&tg.ChannelLocation{Address: "NoGeo"})
	if out2 == nil || out2.Address != "NoGeo" || out2.Location != nil {
		t.Errorf("location without geo = %+v", out2)
	}
}

// --- CustomEmojiStickers ---------------------------------------------------

func TestCustomEmojiStickers(t *testing.T) {
	// nil → empty sticker list.
	if out := CustomEmojiStickers(nil); out == nil {
		t.Error("nil should return []Sticker")
	}
	// Non-vector → empty.
	if out := CustomEmojiStickers(&tg.Photo{}); out == nil {
		t.Error("non-vector should return []Sticker")
	}
	// Vector of documents → stickers.
	vec := &tg.GenericVector{Items: []tg.TLObject{
		&tg.Document{ID: 1, Attributes: []tg.DocumentAttributeClass{&tg.DocumentAttributeCustomEmoji{}}},
		&tg.Document{ID: 2, Attributes: []tg.DocumentAttributeClass{&tg.DocumentAttributeCustomEmoji{}}},
	}}
	result := CustomEmojiStickers(vec)
	stickers, ok := result.([]apitypes.Sticker)
	if !ok {
		t.Fatalf("result type = %T, want []Sticker", result)
	}
	if len(stickers) != 2 {
		t.Errorf("len = %d, want 2", len(stickers))
	}
}

// --- StarTransactions ------------------------------------------------------

func TestStarTransactions(t *testing.T) {
	// nil → empty transactions map.
	out := StarTransactions(nil)
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("nil result type = %T", out)
	}
	txns, _ := m["transactions"].([]any)
	if len(txns) != 0 {
		t.Errorf("nil transactions len = %d, want 0", len(txns))
	}

	// With history.
	status := &tg.PaymentsStarsStatus{
		History: []*tg.StarsTransaction{
			{ID: "tx1", Amount: &tg.StarsAmount{Amount: 100}, Date: 5},
			{ID: "tx2", Amount: &tg.StarsAmount{Amount: 50}},
		},
	}
	out = StarTransactions(status)
	m = out.(map[string]any)
	txns = m["transactions"].([]any)
	if len(txns) != 2 {
		t.Fatalf("transactions len = %d, want 2", len(txns))
	}
	first := txns[0].(map[string]any)
	if first["id"] != "tx1" || first["date"] != int32(5) {
		t.Errorf("tx0 = %+v", first)
	}
}

// --- StarGifts -------------------------------------------------------------

func TestStarGifts(t *testing.T) {
	// nil → empty gifts.
	if out := StarGifts(nil, nil); out == nil {
		t.Error("nil should return Gifts struct")
	}

	result := StarGifts(&tg.PaymentsStarGifts{
		Gifts: []tg.StarGiftClass{
			// Available gift with a sticker.
			&tg.StarGift{
				ID: 1, Stars: 50, UpgradeStars: 10,
				AvailabilityTotal: 100, AvailabilityRemains: 25,
				RequirePremium: true, PeerColorAvailable: true, UpgradeVariants: 4,
				Sticker: &tg.Document{ID: 9, Attributes: []tg.DocumentAttributeClass{&tg.DocumentAttributeSticker{}}},
			},
			// Sold-out gift — excluded.
			&tg.StarGift{ID: 2, Stars: 10, SoldOut: true},
		},
	}, map[int64]string{42: "giftset"})
	gifts, ok := result.(apitypes.Gifts)
	if !ok {
		t.Fatalf("result type = %T, want Gifts", result)
	}
	if len(gifts.Gifts) != 1 {
		t.Fatalf("gifts len = %d, want 1 (sold-out excluded)", len(gifts.Gifts))
	}
	g := gifts.Gifts[0]
	if g.ID != "1" || g.StarCount != 50 || g.UpgradeStarCount != 10 {
		t.Errorf("gift = %+v", g)
	}
	if g.RemainingCount != 25 || g.TotalCount != 100 {
		t.Errorf("availability = %d/%d", g.RemainingCount, g.TotalCount)
	}
	if !g.IsPremium || !g.HasColors {
		t.Errorf("IsPremium=%v HasColors=%v", g.IsPremium, g.HasColors)
	}
	if g.UniqueGiftVariantCount != 4 {
		t.Errorf("UniqueGiftVariantCount = %d, want 4", g.UniqueGiftVariantCount)
	}
	if g.Sticker == nil {
		t.Error("Sticker should be set")
	}
}

// --- UserBoosts ------------------------------------------------------------

func TestUserBoosts(t *testing.T) {
	out := UserBoosts(nil)
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("nil result type = %T", out)
	}
	boosts, _ := m["boosts"].([]any)
	if len(boosts) != 0 {
		t.Errorf("nil boosts len = %d, want 0", len(boosts))
	}

	result := UserBoosts(&tg.PremiumBoostsList{
		Boosts: []*tg.Boost{
			{ID: "b1", Date: 10, UserID: 7},
			{ID: "b2", Date: 20},
		},
		NextOffset: "abc",
	})
	m = result.(map[string]any)
	boosts = m["boosts"].([]any)
	if len(boosts) != 2 {
		t.Fatalf("boosts len = %d, want 2", len(boosts))
	}
	first := boosts[0].(map[string]any)
	if first["boost_id"] != "b1" || first["date"] != int32(10) || first["user_id"] != int64(7) {
		t.Errorf("boost0 = %+v", first)
	}
	if m["next_offset"] != "abc" {
		t.Errorf("next_offset = %v", m["next_offset"])
	}
}

// --- ChatFullInfoFromUserFull ----------------------------------------------

func TestChatFullInfoFromUserFull_Nil(t *testing.T) {
	if ChatFullInfoFromUserFull(nil, nil) != nil {
		t.Error("nil should return nil")
	}
	if ChatFullInfoFromUserFull(&tg.UserFull{}, nil) != nil {
		t.Error("nil user should return nil")
	}
}

func TestChatFullInfoFromUserFull_Regular(t *testing.T) {
	uf := &tg.UserFull{
		About: "My bio",
		Birthday: &tg.Birthday{Day: 1, Month: 6, Year: 2000},
		PersonalChannelID: 42,
		TTLPeriod: 3600,
		PinnedMsgID: 99,
	}
	u := &tg.User{ID: 7, FirstName: "Al", Username: "al"}
	info := ChatFullInfoFromUserFull(uf, u)
	if info == nil {
		t.Fatal("nil info")
	}
	if info.ID != 7 || info.FirstName != "Al" || info.Username != "al" {
		t.Errorf("identity = %+v", info.Chat)
	}
	if info.Bio != "My bio" {
		t.Errorf("Bio = %q", info.Bio)
	}
	if !info.CanSendGift {
		t.Error("regular user should CanSendGift")
	}
	if info.MaxReactionCount != 11 {
		t.Errorf("MaxReactionCount = %d, want 11", info.MaxReactionCount)
	}
	if info.AccentColorID != 7%7 {
		t.Errorf("AccentColorID = %d", info.AccentColorID)
	}
	if info.Birthdate == nil || info.Birthdate.Day != 1 {
		t.Errorf("Birthdate = %+v", info.Birthdate)
	}
	if info.PersonalChat == nil || info.PersonalChat.ID != -(1000000000000+42) {
		t.Errorf("PersonalChat = %+v", info.PersonalChat)
	}
	if info.MessageAutoDeleteTime != 3600 {
		t.Errorf("MessageAutoDeleteTime = %d", info.MessageAutoDeleteTime)
	}
	if info.PinnedMessage == nil || info.PinnedMessage.MessageID != 99 {
		t.Errorf("PinnedMessage = %+v", info.PinnedMessage)
	}
}

func TestChatFullInfoFromUserFull_Bot(t *testing.T) {
	uf := &tg.UserFull{About: "botdesc"}
	u := &tg.User{ID: 8, Bot: true}
	info := ChatFullInfoFromUserFull(uf, u)
	// Bots: no bio, no CanSendGift, AcceptedGiftTypes all false.
	if info.Bio != "" {
		t.Errorf("Bot Bio should be empty, got %q", info.Bio)
	}
	if info.CanSendGift {
		t.Error("bot should not CanSendGift")
	}
	if info.AcceptedGiftTypes == nil {
		t.Error("AcceptedGiftTypes should be present")
	}
}

func TestChatFullInfoFromUserFull_PrivacyFlags(t *testing.T) {
	uf := &tg.UserFull{
		PrivateForwardName:       "hidden",
		VoiceMessagesForbidden:   true,
	}
	u := &tg.User{ID: 1}
	info := ChatFullInfoFromUserFull(uf, u)
	if !info.HasPrivateForwards {
		t.Error("HasPrivateForwards should be true")
	}
	if !info.HasRestrictedVoiceAndVideo {
		t.Error("HasRestrictedVoiceAndVideo should be true")
	}
}

func TestChatFullInfoFromUserFull_ActiveUsernames(t *testing.T) {
	uf := &tg.UserFull{}
	u := &tg.User{
		ID:       1,
		Username: "primary",
		Usernames: []*tg.Username{
			{Username: "collect1", Active: true},
			{Username: "collect2", Active: false},
		},
	}
	info := ChatFullInfoFromUserFull(uf, u)
	// Primary username is prepended, only active collectible included.
	if len(info.ActiveUsernames) != 2 {
		t.Fatalf("ActiveUsernames = %v", info.ActiveUsernames)
	}
	if info.ActiveUsernames[0] != "primary" {
		t.Errorf("primary should be first, got %v", info.ActiveUsernames)
	}
}

// --- ChatFullInfoFromChatFull ----------------------------------------------

func TestChatFullInfoFromChatFull(t *testing.T) {
	if ChatFullInfoFromChatFull(nil, nil) != nil {
		t.Error("nil should return nil")
	}
	cf := &tg.ChatFull{
		About:         "Group desc",
		ReactionsLimit: 3,
		PinnedMsgID:   5,
		TTLPeriod:     60,
		AvailableReactions: &tg.ChatReactionsSome{
			Reactions: []tg.ReactionClass{&tg.ReactionEmoji{Emoticon: "👍"}},
		},
	}
	chat := &tg.Chat{
		ID: 100, Title: "My Group",
		DefaultBannedRights: &tg.ChatBannedRights{},
	}
	info := ChatFullInfoFromChatFull(cf, chat)
	if info == nil {
		t.Fatal("nil info")
	}
	if info.ID != -100 {
		t.Errorf("ID = %d, want -100", info.ID)
	}
	if info.Title != "My Group" {
		t.Errorf("Title = %q", info.Title)
	}
	if info.Description != "Group desc" {
		t.Errorf("Description = %q", info.Description)
	}
	if info.MaxReactionCount != 3 {
		t.Errorf("MaxReactionCount = %d, want 3", info.MaxReactionCount)
	}
	if len(info.AvailableReactions) != 1 {
		t.Errorf("AvailableReactions = %v", info.AvailableReactions)
	}
	if info.AllMembersAreAdministrators == nil {
		t.Error("AllMembersAreAdministrators should be set for basic groups")
	}
	if info.AccentColorID != int32(100%7) {
		t.Errorf("AccentColorID = %d", info.AccentColorID)
	}
}

// --- ChatFullInfoFromChannelFull -------------------------------------------

func TestChatFullInfoFromChannelFull(t *testing.T) {
	if ChatFullInfoFromChannelFull(nil, nil) != nil {
		t.Error("nil cf should return nil")
	}
	if ChatFullInfoFromChannelFull(&tg.ChannelFull{}, nil) != nil {
		t.Error("nil channel should return nil")
	}
	ch := &tg.Channel{
		ID:       200,
		Title:    "Supergroup",
		Username: "super",
		Megagroup: true,
		Forum:    true,
	}
	cf := &tg.ChannelFull{
		About:             "Desc",
		SlowmodeSeconds:   30,
		HiddenPrehistory:  true,
		ParticipantsHidden: true,
		Antispam:          true,
		ReactionsLimit:    5,
		LinkedChatID:      9,
		Location: &tg.ChannelLocation{Address: "Loc", GeoPoint: &tg.GeoPoint{Lat: 1, Long: 2}},
	}
	info := ChatFullInfoFromChannelFull(cf, ch)
	if info == nil {
		t.Fatal("nil info")
	}
	if info.ID != -(1000000000000 + 200) {
		t.Errorf("ID = %d", info.ID)
	}
	if info.Type != apitypes.ChatTypeSupergroup {
		t.Errorf("Type = %q, want supergroup", info.Type)
	}
	if info.SlowModeDelay != 30 {
		t.Errorf("SlowModeDelay = %d", info.SlowModeDelay)
	}
	if info.HasVisibleHistory {
		t.Error("HasVisibleHistory should be false (HiddenPrehistory=true)")
	}
	if !info.HasHiddenMembers || !info.HasAggressiveAntiSpamEnabled {
		t.Error("hidden members / antispam flags not set")
	}
	if info.LinkedChatID != 9 {
		t.Errorf("LinkedChatID = %d", info.LinkedChatID)
	}
	if info.Location == nil || info.Location.Address != "Loc" {
		t.Errorf("Location = %+v", info.Location)
	}
	if info.MaxReactionCount != 5 {
		t.Errorf("MaxReactionCount = %d", info.MaxReactionCount)
	}
}

func TestChatFullInfoFromChannelFull_Channel(t *testing.T) {
	// Non-megagroup channel.
	ch := &tg.Channel{ID: 5, Title: "Channel"}
	cf := &tg.ChannelFull{PaidMediaAllowed: true, SendPaidMessagesStars: 25}
	info := ChatFullInfoFromChannelFull(cf, ch)
	if info.Type != apitypes.ChatTypeChannel {
		t.Errorf("Type = %q, want channel", info.Type)
	}
	if !info.CanSendPaidMedia {
		t.Error("CanSendPaidMedia should be true for channel")
	}
	if info.PaidMessageStarCount != 25 {
		t.Errorf("PaidMessageStarCount = %d, want 25", info.PaidMessageStarCount)
	}
}

func TestChatFullInfoFromChannelFull_EmojiStatus(t *testing.T) {
	ch := &tg.Channel{
		ID:          1,
		Color:       &tg.PeerColor{Color: 4, BackgroundEmojiID: 99},
		ProfileColor: &tg.PeerColor{Color: 2},
		EmojiStatus: &tg.EmojiStatus{DocumentID: 7, Until: 123},
	}
	cf := &tg.ChannelFull{}
	info := ChatFullInfoFromChannelFull(cf, ch)
	if info.AccentColorID != 4 {
		t.Errorf("AccentColorID = %d, want 4", info.AccentColorID)
	}
	if info.BackgroundCustomEmojiID != "99" {
		t.Errorf("BackgroundCustomEmojiID = %q", info.BackgroundCustomEmojiID)
	}
	if info.ProfileAccentColorID != 2 {
		t.Errorf("ProfileAccentColorID = %d", info.ProfileAccentColorID)
	}
	if info.EmojiStatusCustomEmojiID != "7" {
		t.Errorf("EmojiStatusCustomEmojiID = %q", info.EmojiStatusCustomEmojiID)
	}
	if info.EmojiStatusExpirationDate != 123 {
		t.Errorf("EmojiStatusExpirationDate = %d", info.EmojiStatusExpirationDate)
	}
}
