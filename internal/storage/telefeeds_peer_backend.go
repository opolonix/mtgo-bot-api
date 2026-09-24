package storage

import (
	"context"
	"errors"
)

var ErrExternalWebhookUnsupported = errors.New("webhooks are not available for externally managed Bot API clients")

// PeerBackend lets an externally managed Bot API client use its host's peer
// storage instead of creating another per-bot SQLite database or cache.
type PeerBackend interface {
	SavePeer(context.Context, Peer) error
	GetPeer(context.Context, int64) (Peer, error)
	GetPeerByUsername(context.Context, string) (Peer, error)
	SaveChatFlags(context.Context, int64, bool, bool, string) error
	SaveBotMemberStatus(context.Context, int64, string) error
}

func NewExternalPeerStore(backend PeerBackend) *Store {
	return &Store{peerBackend: backend}
}
