package client

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/mtgo-labs/mtgo/tg"

	"github.com/mtgo-labs/mtgo-bot-api/internal/server"
	"github.com/mtgo-labs/mtgo-bot-api/internal/storage"
	apitypes "github.com/mtgo-labs/mtgo-bot-api/internal/types"
)

// NewHostedClient delegates peer persistence and raw TL calls to its host.
// It neither opens SQLite nor retains a message cache or Telegram connection.
func NewHostedClient(params Params, botID string, invoker tg.Invoker, peers storage.PeerBackend) *Client {
	return NewHostedClientWithStore(params, botID, invoker, peers, nil)
}

type HostedMessageStore interface {
	GetMessage(context.Context, int64, int64) (*apitypes.Message, bool, error)
	PutMessage(context.Context, *apitypes.Message) error
	DeleteMessage(context.Context, int64, int64) error
}

func NewHostedClientWithStore(params Params, botID string, invoker tg.Invoker, peers storage.PeerBackend, messages HostedMessageStore) *Client {
	return &Client{
		Token:          botID + ":telefeeds",
		params:         params,
		botID:          botID,
		startTime:      time.Now(),
		rpc:            tg.NewRPCClient(invoker),
		store:          storage.NewExternalPeerStore(peers),
		hostedMessages: messages,
		ready:          true,
		hosted:         true,
	}
}

func (c *Client) rememberHostedMessage(ctx context.Context, message *apitypes.Message) {
	if c.hostedMessages == nil || message == nil || message.Date == 0 {
		return
	}
	if err := c.hostedMessages.PutMessage(ctx, message); err != nil {
		slog.Warn("hosted message cache write failed", "bot_id", c.botID, "error", err)
	}
}

// DownloadFile resolves a Bot API file_id through the Telefeeds-backed RPC
// transport and returns the path of the staged file.
func (c *Client) DownloadFile(ctx context.Context, fileID string) (string, error) {
	result, err := c.getFile(ctx, &server.Query{Args: map[string]string{"file_id": fileID}})
	if err != nil {
		return "", err
	}
	file, ok := result.(*apitypes.File)
	if !ok || file.FilePath == "" {
		return "", errors.New("download did not produce a file path")
	}
	return file.FilePath, nil
}
