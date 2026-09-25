package client

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/mtgo-labs/mtgo-bot-api/internal/storage"
	apitypes "github.com/mtgo-labs/mtgo-bot-api/internal/types"
	"github.com/mtgo-labs/mtgo/telegram"
	mtgotypes "github.com/mtgo-labs/mtgo/telegram/types"
	"github.com/mtgo-labs/mtgo/tg"
)

// ConvertRawUpdatesWithPeers also returns the complete peers carried by the
// update, so an external host can persist them in one batch.
func (c *Client) ConvertRawUpdatesWithPeers(body []byte) ([][]byte, []storage.Peer, error) {
	return c.convertRawUpdates(body)
}

func (c *Client) convertRawUpdates(body []byte) ([][]byte, []storage.Peer, error) {
	object, err := tg.ReadTLObject(tg.NewReader(body))
	if err != nil {
		return nil, nil, fmt.Errorf("decode TL update: %w", err)
	}
	updates, ok := object.(tg.UpdatesClass)
	if !ok {
		return nil, nil, fmt.Errorf("expected UpdatesClass, got %T", object)
	}
	users, chats, rawUpdates := flattenExternalUpdates(updates, c.botID)
	peerRecords := make([]storage.Peer, 0, len(users)+len(chats))
	userMap := make(map[int64]*mtgotypes.User, len(users))
	for _, user := range users {
		parsed := mtgotypes.ParseUser(user)
		if parsed != nil {
			userMap[parsed.ID] = parsed
			peerRecords = append(peerRecords, storage.Peer{
				ID: parsed.ID, AccessHash: parsed.AccessHash, Type: storage.PeerTypeUser,
				Username: parsed.Username, FirstName: parsed.FirstName,
			})
		}
	}
	chatMap := make(map[int64]*mtgotypes.Chat, len(chats))
	for _, chat := range chats {
		parsed := mtgotypes.ParseChatFromChat(chat)
		if parsed != nil {
			chatMap[parsed.ID] = parsed
			switch value := chat.(type) {
			case *tg.Channel:
				peerRecords = append(peerRecords, storage.Peer{
					ID: value.ID, AccessHash: value.AccessHash, Type: storage.PeerTypeChannel,
					Username: value.Username, IsMegagroup: value.Megagroup,
				})
			case *tg.ChannelForbidden:
				peerRecords = append(peerRecords, storage.Peer{
					ID: value.ID, AccessHash: value.AccessHash, Type: storage.PeerTypeChannel,
					IsMegagroup: value.Megagroup, BotMemberStatus: "kicked",
				})
			case *tg.Chat:
				peerRecords = append(peerRecords, storage.Peer{
					ID: value.ID, Type: storage.PeerTypeChat, IsDeactivated: value.Deactivated,
				})
			case *tg.ChatForbidden:
				peerRecords = append(peerRecords, storage.Peer{
					ID: value.ID, Type: storage.PeerTypeChat, BotMemberStatus: "kicked",
				})
			}
		}
	}
	peers := mtgotypes.NewPeerMapFromClasses(users, chats)
	selfID, _ := strconv.ParseInt(c.botID, 10, 64)
	result := make([][]byte, 0, len(rawUpdates))
	for _, raw := range rawUpdates {
		update := externalUpdate(raw, userMap, chatMap, peers)
		object := buildUpdateObject(update, selfID, c.msgs)
		if c.hostedMessages != nil {
			for _, name := range []string{"message", "edited_message", "channel_post", "edited_channel_post", "business_message", "edited_business_message"} {
				if message, ok := object[name].(*apitypes.Message); ok {
					c.rememberHostedMessage(context.Background(), message)
				}
			}
			if callback, ok := object["callback_query"].(*apitypes.CallbackQuery); ok && callback.Message != nil {
				message, active, err := c.hostedMessages.GetMessage(context.Background(), callback.Message.Chat.ID, callback.Message.MessageID)
				if err != nil {
					slog.Warn("hosted callback message cache read failed", "bot_id", c.botID, "error", err)
				}
				if message != nil && message.Date > 0 {
					callback.Message = message
				} else if active {
					ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
					if callback.Message.Chat.ID <= dialogChannelThreshold {
						channelID := -callback.Message.Chat.ID - dialogChannelMark
						for _, peer := range peerRecords {
							if peer.ID == channelID && peer.Type == storage.PeerTypeChannel {
								if err := c.store.SavePeer(ctx, peer); err != nil {
									slog.Debug("callback channel peer could not be saved", "bot_id", c.botID, "error", err)
								}
								break
							}
						}
					}
					messages, fetchErr := c.fetchSourceMessages(ctx, callback.Message.Chat.ID, callback.Message.MessageID)
					cancel()
					if fetchErr != nil {
						slog.Debug("callback message lookup unavailable", "bot_id", c.botID, "error", fetchErr)
					}
					for _, candidate := range messages {
						original, ok := candidate.(*tg.Message)
						if !ok || int64(original.ID) != callback.Message.MessageID {
							continue
						}
						converted := c.botMessage(context.Background(), original, nil)
						if converted.Date > 0 && converted.Chat.ID == callback.Message.Chat.ID {
							callback.Message = converted
							c.rememberHostedMessage(context.Background(), converted)
						}
						break
					}
				}
			}
			if deleted, ok := raw.(*tg.UpdateDeleteMessages); ok {
				for _, id := range deleted.Messages {
					if err := c.hostedMessages.DeleteMessage(context.Background(), 0, int64(id)); err != nil {
						slog.Warn("hosted deleted message cache removal failed", "bot_id", c.botID, "error", err)
					}
				}
			}
			if deleted, ok := raw.(*tg.UpdateDeleteChannelMessages); ok {
				for _, id := range deleted.Messages {
					if err := c.hostedMessages.DeleteMessage(context.Background(), -1_000_000_000_000-deleted.ChannelID, int64(id)); err != nil {
						slog.Warn("hosted deleted channel message cache removal failed", "bot_id", c.botID, "error", err)
					}
				}
			}
		}
		if object == nil {
			continue
		}
		encoded, err := json.Marshal(object)
		if err != nil {
			return nil, nil, fmt.Errorf("encode Bot API update: %w", err)
		}
		result = append(result, encoded)
	}
	return result, peerRecords, nil
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
	case *tg.UpdateInlineBotCallbackQuery:
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
