package client

import (
	"context"
	"errors"
	"time"

	"github.com/mtgo-labs/mtgo/tg"

	"github.com/mtgo-labs/mtgo-bot-api/internal/server"
	"github.com/mtgo-labs/mtgo-bot-api/internal/storage"
	apitypes "github.com/mtgo-labs/mtgo-bot-api/internal/types"
)

// NewHostedClient delegates peer persistence and raw TL calls to its host.
// It neither opens SQLite nor retains a message cache or Telegram connection.
func NewHostedClient(params Params, botID string, invoker tg.Invoker, peers storage.PeerBackend) *Client {
	return &Client{
		Token:     botID + ":telefeeds",
		params:    params,
		botID:     botID,
		startTime: time.Now(),
		rpc:       tg.NewRPCClient(invoker),
		store:     storage.NewExternalPeerStore(peers),
		ready:     true,
		hosted:    true,
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
