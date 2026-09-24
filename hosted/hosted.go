// Package hosted exposes the Bot API converter and method dispatcher to a
// process that already owns Telegram sessions, peers, files and delivery.
// It opens no Telegram connection, SQLite database or update queue.
package hosted

import (
	"context"
	"errors"
	"strings"

	"github.com/mtgo-labs/mtgo-bot-api/internal/client"
	"github.com/mtgo-labs/mtgo-bot-api/internal/server"
	"github.com/mtgo-labs/mtgo-bot-api/internal/storage"
	"github.com/mtgo-labs/mtgo/tg"
)

type Peer = storage.Peer
type PeerType = storage.PeerType
type PeerBackend = storage.PeerBackend
type File = server.File

const (
	PeerTypeUser    = storage.PeerTypeUser
	PeerTypeChat    = storage.PeerTypeChat
	PeerTypeChannel = storage.PeerTypeChannel
)

type Client struct {
	client *client.Client
	botID  string
}

func New(botID string, invoker tg.Invoker, peers PeerBackend) (*Client, error) {
	if !storage.ValidBotID(botID) || invoker == nil || peers == nil {
		return nil, errors.New("valid bot ID, TL invoker and peer backend are required")
	}
	return &Client{
		client: client.NewHostedClient(client.Params{LocalMode: true, DownloadChunkSize: 512 * 1024}, botID, editResultInvoker{Invoker: invoker}, peers),
		botID:  botID,
	}, nil
}

// ConvertRawUpdates returns webhook-shaped updates without update_id together
// with their peers. The host persists peers and assigns IDs before publishing.
func (c *Client) ConvertRawUpdates(body []byte) ([][]byte, []Peer, error) {
	return c.client.ConvertRawUpdatesWithPeers(body)
}

// Invoke performs one Bot API method. Any TL calls made by the method go
// through invoker; file bytes are supplied by the host through File values.
func (c *Client) Invoke(ctx context.Context, method string, args map[string]string, files map[string]File) (int, []byte) {
	query := server.NewQuery()
	query.Method = strings.ToLower(method)
	query.Token = c.botID + ":telefeeds"
	query.Args = args
	query.Files = files
	if query.Method == "editmessagereplymarkup" {
		query.Args = normalizeEditReplyMarkup(args)
	}
	if query.Args == nil {
		query.Args = make(map[string]string)
	}
	if query.Files == nil {
		query.Files = make(map[string]File)
	}
	return c.client.Dispatch(ctx, query)
}

func (c *Client) DownloadFile(ctx context.Context, fileID string) (string, error) {
	return c.client.DownloadFile(ctx, fileID)
}
