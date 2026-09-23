package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/getsentry/sentry-go"
	sentryhttp "github.com/getsentry/sentry-go/http"
	"github.com/mtgo-labs/mtgo-bot-api/internal/client"
	"github.com/mtgo-labs/mtgo-bot-api/internal/gatewaypb"
	apiresponse "github.com/mtgo-labs/mtgo-bot-api/internal/response"
	"github.com/mtgo-labs/mtgo-bot-api/internal/server"
	"github.com/mtgo-labs/mtgo/tg"
	"github.com/mtgo-labs/mtgo/tgerr"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

const defaultSentryDSN = "https://5a3e9e0c99d14e368f28fe572711b81f@glitch.macasic.cc/8"

type invokePayload struct {
	SessionPeerID int64                      `json:"session_peer_id"`
	Method        string                     `json:"method"`
	Parameters    map[string]json.RawMessage `json:"parameters"`
	Files         map[string]string          `json:"files"`
}

type converter struct {
	gateway gatewaypb.TelegramGatewayClient
	params  client.Params
	clients sync.Map
	updates sync.Map
}

type gatewayInvoker struct {
	gateway       gatewaypb.TelegramGatewayClient
	token         string
	sessionPeerID int64
}

func captureError(operation string, err error) {
	log.Printf("%s: %v", operation, err)
	sentry.WithScope(func(scope *sentry.Scope) {
		scope.SetTag("operation", operation)
		sentry.CaptureException(err)
	})
}

func writeBotAPIError(response http.ResponseWriter, code int, description string) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(code)
	if _, err := response.Write(apiresponse.Fail(code, description, nil)); err != nil {
		captureError("write Bot API error", err)
	}
}

func (service *converter) botClient(token string, sessionPeerID int64) (*client.Client, error) {
	digest := sha256.Sum256([]byte(token))
	key := hex.EncodeToString(digest[:]) + ":" + strconv.FormatInt(sessionPeerID, 10)
	if value, ok := service.clients.Load(key); ok {
		return value.(*client.Client), nil
	}
	created, err := client.NewExternalClient(service.params, strconv.FormatInt(sessionPeerID, 10), &gatewayInvoker{gateway: service.gateway, token: token, sessionPeerID: sessionPeerID})
	if err != nil {
		return nil, err
	}
	value, loaded := service.clients.LoadOrStore(key, created)
	if loaded {
		created.Stop()
	}
	return value.(*client.Client), nil
}

func (invoker *gatewayInvoker) RPCInvoke(ctx context.Context, input tg.TLObject, decode func(*tg.Reader) (tg.TLObject, error)) (tg.TLObject, error) {
	body, err := invoker.RPCInvokeRaw(ctx, input)
	if err != nil {
		return nil, err
	}
	return decode(tg.NewReader(body))
}

func (invoker *gatewayInvoker) RPCInvokeRaw(ctx context.Context, input tg.TLObject) ([]byte, error) {
	var body bytes.Buffer
	if err := input.Encode(&body); err != nil {
		return nil, fmt.Errorf("encode TL request: %w", err)
	}
	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+invoker.token)
	response, err := invoker.gateway.Invoke(ctx, &gatewaypb.InvokeRequest{
		SessionPeerId: invoker.sessionPeerID,
		Body:          body.Bytes(),
		TlLayer:       229,
		Protocol:      gatewaypb.ApiProtocol_API_PROTOCOL_RAW_TL,
	})
	if err != nil {
		return nil, err
	}
	if response.RpcError != nil {
		return nil, tgerr.New(int(response.RpcError.Code), response.RpcError.Name)
	}
	if response.Error != nil {
		return nil, errors.New(*response.Error)
	}
	return response.Body, nil
}

func (service *converter) invoke(response http.ResponseWriter, request *http.Request) {
	token := strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")
	if token == "" || token == request.Header.Get("Authorization") {
		writeBotAPIError(response, http.StatusUnauthorized, "Unauthorized: integration token required")
		return
	}
	var payload invokePayload
	decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 16<<20))
	if err := decoder.Decode(&payload); err != nil {
		writeBotAPIError(response, http.StatusBadRequest, "Bad Request: invalid JSON request")
		return
	}
	if payload.SessionPeerID <= 0 || payload.Method == "" {
		writeBotAPIError(response, http.StatusBadRequest, "Bad Request: session_peer_id and method are required")
		return
	}
	botClient, err := service.botClient(token, payload.SessionPeerID)
	if err != nil {
		captureError("create bot client", err)
		writeBotAPIError(response, http.StatusServiceUnavailable, "Service Unavailable: Bot API converter unavailable")
		return
	}
	query := server.NewQuery()
	query.Method = strings.ToLower(payload.Method)
	query.Token = strconv.FormatInt(payload.SessionPeerID, 10) + ":telefeeds"
	temporaryFiles := make([]string, 0, len(payload.Files))
	defer func() {
		for _, path := range temporaryFiles {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				log.Printf("remove temporary upload: %v", err)
			}
		}
	}()
	for name, uploadID := range payload.Files {
		ctx := metadata.AppendToOutgoingContext(request.Context(), "authorization", "Bearer "+token)
		stream, err := service.gateway.DownloadBotApiFile(ctx, &gatewaypb.BotApiFileRequest{
			SessionPeerId: payload.SessionPeerID,
			Source:        &gatewaypb.BotApiFileRequest_UploadId{UploadId: uploadID},
		})
		if err != nil {
			writeBotAPIError(response, http.StatusBadRequest, "Bad Request: uploaded file is unavailable")
			return
		}
		file, err := os.CreateTemp(service.params.TempDir, "telefeeds-upload-*")
		if err != nil {
			writeBotAPIError(response, http.StatusServiceUnavailable, "Service Unavailable: file staging failed")
			return
		}
		path := file.Name()
		temporaryFiles = append(temporaryFiles, path)
		var header *gatewaypb.BotApiFileHeader
		for {
			chunk, recvErr := stream.Recv()
			if errors.Is(recvErr, io.EOF) {
				break
			}
			if recvErr != nil {
				file.Close()
				writeBotAPIError(response, http.StatusServiceUnavailable, "Service Unavailable: uploaded file download failed")
				return
			}
			switch value := chunk.Payload.(type) {
			case *gatewaypb.BotApiFileChunk_Header:
				header = value.Header
			case *gatewaypb.BotApiFileChunk_Data:
				if _, err := file.Write(value.Data); err != nil {
					file.Close()
					writeBotAPIError(response, http.StatusServiceUnavailable, "Service Unavailable: file staging failed")
					return
				}
			}
		}
		if err := file.Close(); err != nil || header == nil {
			writeBotAPIError(response, http.StatusServiceUnavailable, "Service Unavailable: file staging failed")
			return
		}
		query.Files[name] = server.File{
			FieldName: name,
			FileName:  filepath.Base(header.FileName),
			TempPath:  path,
			MimeType:  header.GetContentType(),
			Size:      int64(header.GetContentLength()),
		}
	}
	for name, raw := range payload.Parameters {
		if bytes.Equal(raw, []byte("null")) {
			continue
		}
		var text string
		if err := json.Unmarshal(raw, &text); err == nil {
			query.Args[name] = text
		} else {
			query.Args[name] = string(raw)
		}
	}
	status, body := botClient.Dispatch(request.Context(), query)
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	if _, err := response.Write(body); err != nil {
		log.Printf("write invoke response: %v", err)
	}
}

func (service *converter) download(response http.ResponseWriter, request *http.Request) {
	token := strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")
	if token == "" || token == request.Header.Get("Authorization") {
		http.Error(response, "authorization required", http.StatusUnauthorized)
		return
	}
	peerID, err := strconv.ParseInt(request.URL.Query().Get("session_peer_id"), 10, 64)
	fileID := request.URL.Query().Get("file_id")
	if err != nil || peerID <= 0 || fileID == "" {
		http.Error(response, "valid session_peer_id and file_id required", http.StatusBadRequest)
		return
	}
	botClient, err := service.botClient(token, peerID)
	if err != nil {
		http.Error(response, "converter unavailable", http.StatusServiceUnavailable)
		return
	}
	path, err := botClient.DownloadFile(request.Context(), fileID)
	if err != nil {
		captureError("download bot file", err)
		http.Error(response, "file download failed", http.StatusBadGateway)
		return
	}
	defer func() {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Printf("remove downloaded file: %v", err)
		}
	}()
	response.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(path)))
	http.ServeFile(response, request, path)
}

func (service *converter) convert(response http.ResponseWriter, request *http.Request) {
	peerID, err := strconv.ParseInt(request.Header.Get("X-Telefeeds-Session-Peer-ID"), 10, 64)
	if err != nil || peerID <= 0 {
		http.Error(response, "valid session peer id required", http.StatusBadRequest)
		return
	}
	if request.ContentLength <= 0 || request.ContentLength > 16<<20 {
		http.Error(response, "invalid update size", http.StatusBadRequest)
		return
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, 16<<20))
	if err != nil || int64(len(body)) != request.ContentLength {
		http.Error(response, "invalid update body", http.StatusBadRequest)
		return
	}
	value, ok := service.updates.Load(peerID)
	if !ok {
		created, err := client.NewExternalClient(service.params, strconv.FormatInt(peerID, 10), tg.InvokerFunc(func(context.Context, tg.TLObject, func(*tg.Reader) (tg.TLObject, error)) (tg.TLObject, error) {
			return nil, errors.New("RPC invocation is unavailable in update converter")
		}))
		if err != nil {
			captureError("create update converter", err)
			http.Error(response, "converter unavailable", http.StatusServiceUnavailable)
			return
		}
		value, _ = service.updates.LoadOrStore(peerID, created)
	}
	updates, err := value.(*client.Client).ConvertRawUpdates(body)
	if err != nil {
		captureError("convert Bot API update", err)
		http.Error(response, "update conversion failed", http.StatusUnprocessableEntity)
		return
	}
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(http.StatusOK)
	response.Write([]byte("["))
	for index, update := range updates {
		if index > 0 {
			response.Write([]byte(","))
		}
		response.Write(update)
	}
	response.Write([]byte("]"))
}

func main() {
	sentryDSN := os.Getenv("TELEFEEDS_BOTAPI_SENTRY_DSN")
	if sentryDSN == "" {
		sentryDSN = defaultSentryDSN
	}
	if err := sentry.Init(sentry.ClientOptions{Dsn: sentryDSN, EnableTracing: true}); err != nil {
		log.Fatalf("initialize Sentry: %v", err)
	}
	defer sentry.Flush(2 * time.Second)
	target := os.Getenv("TELEFEEDS_GATEWAY_TARGET")
	if target == "" {
		target = "telefeeds-clientshub:50057"
	}
	connection, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatal(err)
	}
	defer connection.Close()
	workingDir := os.Getenv("TELEFEEDS_BOTAPI_WORKING_DIR")
	if workingDir == "" {
		workingDir = "/var/lib/telefeeds-botapi"
	}
	tempDir := os.Getenv("TELEFEEDS_BOTAPI_TEMP_DIR")
	if tempDir == "" {
		tempDir = os.TempDir()
	}
	service := &converter{
		gateway: gatewaypb.NewTelegramGatewayClient(connection),
		params:  client.Params{Dir: workingDir, TempDir: tempDir, LocalMode: true, StartTime: time.Now()},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/invoke", service.invoke)
	mux.HandleFunc("POST /v1/convert", service.convert)
	mux.HandleFunc("GET /v1/download", service.download)
	mux.HandleFunc("GET /health", func(response http.ResponseWriter, request *http.Request) {
		response.WriteHeader(http.StatusNoContent)
	})
	address := os.Getenv("TELEFEEDS_BOTAPI_LISTEN")
	if address == "" {
		address = ":8082"
	}
	log.Printf("telefeeds bot api converter listening on %s", address)
	sentryMiddleware := sentryhttp.New(sentryhttp.Options{Repanic: true})
	server := &http.Server{
		Addr:              address,
		Handler:           sentryMiddleware.Handle(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}
	if err := server.ListenAndServe(); err != nil {
		captureError("serve Bot API converter", err)
		log.Fatal(err)
	}
}
