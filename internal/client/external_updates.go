package client

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/mtgo-labs/mtgo/telegram"
	mtgotypes "github.com/mtgo-labs/mtgo/telegram/types"
	"github.com/mtgo-labs/mtgo/tg"
)

// ConvertRawUpdates converts the canonical layer-229 Updates object supplied by
// Telefeeds into webhook-shaped Bot API updates. It deliberately does not use
// getUpdates or the upstream queue: delivery and backpressure belong to the
// Telefeeds gRPC stream.
func (c *Client) ConvertRawUpdates(body []byte) ([][]byte, error) {
	object, err := tg.ReadTLObject(tg.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("decode TL update: %w", err)
	}
	updates, ok := object.(tg.UpdatesClass)
	if !ok {
		return nil, fmt.Errorf("expected UpdatesClass, got %T", object)
	}
	users, chats, rawUpdates := flattenExternalUpdates(updates, c.botID)
	userMap := make(map[int64]*mtgotypes.User, len(users))
	for _, user := range users {
		parsed := mtgotypes.ParseUser(user)
		if parsed != nil {
			userMap[parsed.ID] = parsed
		}
	}
	chatMap := make(map[int64]*mtgotypes.Chat, len(chats))
	for _, chat := range chats {
		parsed := mtgotypes.ParseChatFromChat(chat)
		if parsed != nil {
			chatMap[parsed.ID] = parsed
		}
	}
	peers := mtgotypes.NewPeerMapFromClasses(users, chats)
	selfID, _ := strconv.ParseInt(c.botID, 10, 64)
	result := make([][]byte, 0, len(rawUpdates))
	for _, raw := range rawUpdates {
		update := externalUpdate(raw, userMap, chatMap, peers)
		object := buildUpdateObject(update, selfID, c.msgs)
		if object == nil {
			continue
		}
		object["update_id"] = c.nextUpdateID.Add(1)
		encoded, err := json.Marshal(object)
		if err != nil {
			return nil, fmt.Errorf("encode Bot API update: %w", err)
		}
		result = append(result, encoded)
	}
	return result, nil
}

func flattenExternalUpdates(updates tg.UpdatesClass, botID string) ([]tg.UserClass, []tg.ChatClass, []tg.UpdateClass) {
	switch value := updates.(type) {
	case *tg.Updates:
		return value.Users, value.Chats, value.Updates
	case *tg.UpdatesCombined:
		return value.Users, value.Chats, value.Updates
	case *tg.UpdateShort:
		return nil, nil, []tg.UpdateClass{value.Update}
	case *tg.UpdateShortMessage:
		var fromID tg.PeerClass = &tg.PeerUser{UserID: value.UserID}
		if value.Out {
			selfID, _ := strconv.ParseInt(botID, 10, 64)
			fromID = &tg.PeerUser{UserID: selfID}
		}
		message := &tg.Message{ID: value.ID, Message: value.Message, Date: value.Date, Out: value.Out, Mentioned: value.Mentioned, Silent: value.Silent, FromID: fromID, PeerID: &tg.PeerUser{UserID: value.UserID}, ReplyTo: value.ReplyTo, Entities: value.Entities, FwdFrom: value.FwdFrom, ViaBotID: value.ViaBotID}
		return nil, nil, []tg.UpdateClass{&tg.UpdateNewMessage{Message: message, PTS: value.PTS, PTSCount: value.PTSCount}}
	case *tg.UpdateShortChatMessage:
		message := &tg.Message{ID: value.ID, Message: value.Message, Date: value.Date, Out: value.Out, Mentioned: value.Mentioned, Silent: value.Silent, FromID: &tg.PeerUser{UserID: value.FromID}, PeerID: &tg.PeerChat{ChatID: value.ChatID}, ReplyTo: value.ReplyTo, Entities: value.Entities, FwdFrom: value.FwdFrom, ViaBotID: value.ViaBotID}
		return nil, nil, []tg.UpdateClass{&tg.UpdateNewMessage{Message: message, PTS: value.PTS, PTSCount: value.PTSCount}}
	default:
		return nil, nil, nil
	}
}

func externalUpdate(raw tg.UpdateClass, users map[int64]*mtgotypes.User, chats map[int64]*mtgotypes.Chat, peers *mtgotypes.PeerMap) *telegram.Update {
	update := &telegram.Update{Users: users, Chats: chats, Raw: raw}
	switch value := raw.(type) {
	case *tg.UpdateNewMessage:
		update.Message = mtgotypes.ParseMessage(value.Message, peers)
	case *tg.UpdateNewChannelMessage:
		update.Message = mtgotypes.ParseMessage(value.Message, peers)
	case *tg.UpdateEditMessage:
		update.EditedMessage = mtgotypes.ParseMessage(value.Message, peers)
	case *tg.UpdateEditChannelMessage:
		update.EditedMessage = mtgotypes.ParseMessage(value.Message, peers)
	case *tg.UpdateDeleteMessages:
		update.DeletedMessages = &mtgotypes.DeletedMessages{Messages: value.Messages}
	case *tg.UpdateDeleteChannelMessages:
		update.DeletedMessages = &mtgotypes.DeletedMessages{Messages: value.Messages, ChatID: value.ChannelID}
	case *tg.UpdateBotCallbackQuery:
		update.CallbackQuery = mtgotypes.ParseCallbackQuery(value)
	case *tg.UpdateBotInlineQuery:
		update.InlineQuery = &mtgotypes.InlineQuery{ID: value.QueryID, UserID: value.UserID, Query: value.Query, Offset: value.Offset}
	case *tg.UpdateBotInlineSend:
		update.ChosenInlineResult = mtgotypes.ParseChosenInlineResult(value)
	case *tg.UpdateUserStatus:
		update.UserStatus = &mtgotypes.UserStatusUpdated{UserID: value.UserID}
	case *tg.UpdateChatParticipant:
		update.ChatMember = mtgotypes.ParseChatMemberUpdated(value, externalUserClasses(peers), peers)
	case *tg.UpdateChannelParticipant:
		update.ChatMember = mtgotypes.ParseChatMemberUpdated(value, externalUserClasses(peers), peers)
	case *tg.UpdateBotMessageReaction:
		update.MessageReaction = mtgotypes.ParseMessageReactionUpdate(value)
	case *tg.UpdateBotMessageReactions:
		update.MessageReactionCount = mtgotypes.ParseMessageReactionCountUpdate(value)
	case *tg.UpdateMessagePoll:
		update.Poll = mtgotypes.ParsePollUpdated(value)
	case *tg.UpdateMessagePollVote:
		update.PollAnswer = mtgotypes.ParsePollAnswerUpdate(value)
	case *tg.UpdateBotPrecheckoutQuery:
		update.PreCheckoutQuery = &mtgotypes.PreCheckoutQuery{ID: value.QueryID, UserID: value.UserID, Currency: value.Currency, TotalAmount: value.TotalAmount, ShippingOptionID: value.ShippingOptionID}
	case *tg.UpdateBotShippingQuery:
		update.ShippingQuery = &mtgotypes.ShippingQuery{ID: value.QueryID, UserID: value.UserID}
	case *tg.UpdateBotBusinessConnect:
		update.BusinessConnection = mtgotypes.ParseBusinessConnection(value.Connection, nil)
	case *tg.UpdateBotNewBusinessMessage:
		update.BusinessMessage = mtgotypes.ParseMessage(value.Message, peers)
	case *tg.UpdateBotEditBusinessMessage:
		update.EditedBusinessMessage = mtgotypes.ParseMessage(value.Message, peers)
	case *tg.UpdateBotDeleteBusinessMessage:
		update.DeletedBusinessMessages = &mtgotypes.DeletedMessages{Messages: value.Messages}
	case *tg.UpdateBotChatInviteRequester:
		update.ChatJoinRequest = mtgotypes.ParseChatJoinRequest(value, users, chats)
	case *tg.UpdateStory:
		update.Story = mtgotypes.ParseStory(value.Story, peers)
	case *tg.UpdateBotChatBoost:
		var chatID int64
		switch peer := value.Peer.(type) {
		case *tg.PeerChat:
			chatID = -peer.ChatID
		case *tg.PeerChannel:
			chatID = -1_000_000_000_000 - peer.ChannelID
		case *tg.PeerUser:
			chatID = peer.UserID
		}
		update.ChatBoost = mtgotypes.ParseChatBoostUpdated(chats[chatID], mtgotypes.ParseChatBoost(value.Boost, peers), value.Boost.Stars)
	}
	return update
}

func externalUserClasses(peers *mtgotypes.PeerMap) map[int64]tg.UserClass {
	if peers == nil || len(peers.Users) == 0 {
		return nil
	}
	users := make(map[int64]tg.UserClass, len(peers.Users))
	for id, user := range peers.Users {
		users[id] = user
	}
	return users
}
