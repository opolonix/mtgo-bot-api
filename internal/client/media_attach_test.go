package client

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/mtgo-labs/mtgo-bot-api/internal/server"
	"github.com/mtgo-labs/mtgo/tg"
)

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
