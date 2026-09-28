package client

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/mtgo-labs/mtgo-bot-api/internal/convert"
	"github.com/mtgo-labs/mtgo-bot-api/internal/server"
	apitypes "github.com/mtgo-labs/mtgo-bot-api/internal/types"
	"github.com/mtgo-labs/mtgo/tg"
)

func TestUploadedDocumentKeepsOriginalFileName(t *testing.T) {
	content := []byte("name,count\nTelefeeds,1\n")
	path := t.TempDir() + "/report.csv"
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}

	var sentAttributes []tg.DocumentAttributeClass
	client := &Client{ready: true, botID: "123", rpc: &fakeRPC{
		UploadSaveFilePartFn: func(_ context.Context, request *tg.UploadSaveFilePartRequest) (bool, error) {
			if string(request.Bytes) != string(content) {
				t.Fatalf("uploaded bytes = %q, want %q", request.Bytes, content)
			}
			return true, nil
		},
		MessagesSendMediaFn: func(_ context.Context, request *tg.MessagesSendMediaRequest) (tg.UpdatesClass, error) {
			media, ok := request.Media.(*tg.InputMediaUploadedDocument)
			if !ok {
				t.Fatalf("sent media = %T, want uploaded document", request.Media)
			}
			sentAttributes = media.Attributes
			return &tg.UpdateShortSentMessage{
				ID:   1,
				Date: 1,
				Media: &tg.MessageMediaDocument{Document: &tg.Document{
					ID:       1,
					DCID:     2,
					Size:     int64(len(content)),
					MimeType: "application/octet-stream",
				}},
			}, nil
		},
	}}
	query := server.NewQuery()
	query.Args["chat_id"] = "456"
	query.Files["document"] = server.File{
		FieldName: "document",
		FileName:  "report.csv",
		TempPath:  path,
		Size:      int64(len(content)),
	}
	media, err := client.docMediaInput(context.Background(), query, "document", nil, "application/octet-stream")
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.doSendMediaResult(context.Background(), &tg.InputPeerUser{UserID: 456}, media, query)
	if err != nil {
		t.Fatal(err)
	}
	if len(sentAttributes) != 1 {
		t.Fatalf("sent attributes = %v, want one filename attribute", sentAttributes)
	}
	filename, ok := sentAttributes[0].(*tg.DocumentAttributeFilename)
	if !ok || filename.FileName != "report.csv" {
		t.Fatalf("recipient filename attribute = %v, want report.csv", sentAttributes[0])
	}
	received := convert.Document(&tg.Document{ID: 1, DCID: 2, Attributes: sentAttributes})
	if received.FileName != "report.csv" {
		t.Fatalf("recipient document filename = %q, want report.csv", received.FileName)
	}
	message, ok := result.(*apitypes.Message)
	if !ok || message.Document == nil || message.Document.FileName != "report.csv" {
		t.Fatalf("Bot API document response = %#v, want report.csv", result)
	}
	response, err := json.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(response), `"file_name":"report.csv"`) {
		t.Fatalf("Bot API JSON = %s, want file_name report.csv", response)
	}
}

func TestSingleMediaAttachReferenceUsesUploadedFile(t *testing.T) {
	path := t.TempDir() + "/report.csv"
	content := []byte("name,count\nTelefeeds,1\n")
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	for _, paramName := range []string{"document", "photo"} {
		t.Run(paramName, func(t *testing.T) {
			var uploaded []byte
			client := &Client{ready: true, rpc: &fakeRPC{
				UploadSaveFilePartFn: func(_ context.Context, request *tg.UploadSaveFilePartRequest) (bool, error) {
					uploaded = append(uploaded, request.Bytes...)
					return true, nil
				},
			}}
			query := server.NewQuery()
			query.Args[paramName] = "attach://report"
			query.Files["report"] = server.File{FieldName: "report", FileName: "report.csv", TempPath: path, Size: int64(len(content))}
			var media tg.InputMediaClass
			var err error
			if paramName == "document" {
				media, err = client.docMediaInput(context.Background(), query, paramName, nil, "text/csv")
				if _, ok := media.(*tg.InputMediaUploadedDocument); !ok {
					t.Fatalf("media = %T, want uploaded document", media)
				}
			} else {
				media, err = client.photoMediaInput(context.Background(), query, paramName)
				if _, ok := media.(*tg.InputMediaUploadedPhoto); !ok {
					t.Fatalf("media = %T, want uploaded photo", media)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			if string(uploaded) != string(content) {
				t.Fatalf("uploaded = %q, want %q", uploaded, content)
			}
		})
	}
}

func TestSingleMediaAttachReferenceRequiresMatchingFile(t *testing.T) {
	client := &Client{ready: true}
	for _, paramName := range []string{"document", "photo"} {
		query := server.NewQuery()
		query.Args[paramName] = "attach://missing"
		var err error
		if paramName == "document" {
			_, err = client.docMediaInput(context.Background(), query, paramName, nil, "")
		} else {
			_, err = client.photoMediaInput(context.Background(), query, paramName)
		}
		if err == nil || !strings.Contains(err.Error(), `attached file "missing" not found`) {
			t.Fatalf("%s: error = %v", paramName, err)
		}
	}
}
