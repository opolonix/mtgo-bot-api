package client

import (
	"context"
	"strings"
	"testing"

	"github.com/mtgo-labs/mtgo/tg"
	"github.com/mtgo-labs/mtgo/tgerr"

	apitypes "github.com/mtgo-labs/mtgo-bot-api/internal/types"
)

func TestGetChatMemberChannelMembership(t *testing.T) {
	missing := tgerr.New(400, "USER_NOT_PARTICIPANT")
	tests := []struct {
		name          string
		target        tg.ChannelParticipantClass
		targetErr     error
		self          tg.ChannelParticipantClass
		listErr       error
		wantStatus    string
		wantError     bool
		wantSelfCalls int
		wantListCalls int
	}{
		{
			name: "subscriber", target: &tg.ChannelParticipant{UserID: 99},
			wantStatus: "member",
		},
		{
			name: "explicit left", target: &tg.ChannelParticipantLeft{Peer: &tg.PeerUser{UserID: 99}},
			wantStatus: "left",
		},
		{
			name: "missing user with bot access", targetErr: missing,
			self:       &tg.ChannelParticipantSelf{UserID: 1},
			wantStatus: "left", wantSelfCalls: 1, wantListCalls: 1,
		},
		{
			name: "bot outside channel", targetErr: missing,
			self:      &tg.ChannelParticipantLeft{Peer: &tg.PeerUser{UserID: 1}},
			wantError: true, wantSelfCalls: 1,
		},
		{
			name: "bot cannot inspect participants", targetErr: missing,
			self: &tg.ChannelParticipantAdmin{UserID: 1}, listErr: tgerr.New(403, "CHAT_ADMIN_REQUIRED"),
			wantError: true, wantSelfCalls: 1, wantListCalls: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			selfCalls, listCalls := 0, 0
			client := &Client{rpc: &fakeRPC{
				ChannelsGetParticipantFn: func(ctx context.Context, req *tg.ChannelsGetParticipantRequest) (tg.ChannelParticipantClass, error) {
					if _, ok := req.Participant.(*tg.InputPeerSelf); ok {
						selfCalls++
						return &tg.ChannelsChannelParticipant{Participant: test.self}, nil
					}
					return &tg.ChannelsChannelParticipant{
						Participant: test.target,
						Users:       []tg.UserClass{&tg.User{ID: 99, FirstName: "Target"}},
					}, test.targetErr
				},
				ChannelsGetParticipantsFn: func(ctx context.Context, req *tg.ChannelsGetParticipantsRequest) (tg.ChannelParticipantsClass, error) {
					listCalls++
					return &tg.ChannelsChannelParticipants{}, test.listErr
				},
			}}
			result, err := client.getChatMemberChannel(context.Background(), -100123, 99)
			if test.wantError {
				if err == nil || !strings.Contains(err.Error(), "USER_NOT_PARTICIPANT") {
					t.Fatalf("error = %v, want original USER_NOT_PARTICIPANT", err)
				}
				if result != nil {
					t.Fatalf("result = %v, want nil", result)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				member, ok := result.(*apitypes.ChatMember)
				if !ok || member.Status != test.wantStatus || member.User == nil || member.User.ID != 99 {
					t.Fatalf("member = %+v, want status %q and user 99", result, test.wantStatus)
				}
			}
			if selfCalls != test.wantSelfCalls || listCalls != test.wantListCalls {
				t.Errorf("probe calls: self=%d, list=%d; want self=%d, list=%d", selfCalls, listCalls, test.wantSelfCalls, test.wantListCalls)
			}
		})
	}
}
