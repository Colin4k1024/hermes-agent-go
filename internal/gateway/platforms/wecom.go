package platforms

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/gateway"
)

// WeComAdapter implements PlatformAdapter for WeCom (企业微信).
type WeComAdapter struct {
	BasePlatformAdapter
	corpID      string
	agentID     string
	secret      string
	token       string // verification token for callback signature
	encodingKey string // AES key for callback decryption
	accessToken string
	webhookPort int
	server      *http.Server
	ctx         context.Context
	cancel      context.CancelFunc
}

func NewWeComAdapter(corpID, agentID, secret, token, encodingKey string, webhookPort int) *WeComAdapter {
	if webhookPort == 0 {
		webhookPort = 8078
	}
	return &WeComAdapter{
		BasePlatformAdapter: NewBasePlatformAdapter(gateway.PlatformWeCom),
		corpID:              corpID,
		agentID:             agentID,
		secret:              secret,
		token:               token,
		encodingKey:         encodingKey,
		webhookPort:         webhookPort,
	}
}

func (w *WeComAdapter) Connect(ctx context.Context) error {
	w.ctx, w.cancel = context.WithCancel(ctx)

	if err := w.refreshAccessToken(); err != nil {
		slog.Warn("WeCom token refresh failed, will retry", "error", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/wecom/callback", w.handleCallback)

	w.server = &http.Server{
		Addr:         fmt.Sprintf("127.0.0.1:%d", w.webhookPort),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	w.connected = true
	slog.Info("WeCom adapter starting", "port", w.webhookPort)

	go func() {
		if err := w.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("WeCom server error", "error", err)
		}
	}()

	return nil
}

func (w *WeComAdapter) Disconnect() error {
	w.connected = false
	if w.cancel != nil {
		w.cancel()
	}
	if w.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return w.server.Shutdown(ctx)
	}
	return nil
}

func (w *WeComAdapter) Send(ctx context.Context, chatID string, text string, metadata map[string]string) (*gateway.SendResult, error) {
	payload := map[string]any{
		"touser":  chatID,
		"msgtype": "text",
		"agentid": w.agentID,
		"text":    map[string]string{"content": text},
	}
	return w.postAPI(ctx, "/cgi-bin/message/send", payload)
}

func (w *WeComAdapter) SendTyping(_ context.Context, _ string) error { return nil }

func (w *WeComAdapter) SendImage(ctx context.Context, chatID string, _ string, caption string, metadata map[string]string) (*gateway.SendResult, error) {
	return w.Send(ctx, chatID, caption+" [image]", metadata)
}

func (w *WeComAdapter) SendVoice(ctx context.Context, chatID string, _ string, metadata map[string]string) (*gateway.SendResult, error) {
	return w.Send(ctx, chatID, "[voice]", metadata)
}

func (w *WeComAdapter) SendDocument(ctx context.Context, chatID string, _ string, metadata map[string]string) (*gateway.SendResult, error) {
	return w.Send(ctx, chatID, "[file]", metadata)
}

func (w *WeComAdapter) postAPI(ctx context.Context, path string, payload map[string]any) (*gateway.SendResult, error) {
	body, _ := json.Marshal(payload)
	url := fmt.Sprintf("https://qyapi.weixin.qq.com%s?access_token=%s", path, w.accessToken)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return &gateway.SendResult{Error: err.Error()}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return &gateway.SendResult{Error: err.Error(), Retryable: true}, err
	}
	defer resp.Body.Close()

	return &gateway.SendResult{Success: true}, nil
}

func (w *WeComAdapter) refreshAccessToken() error {
	url := fmt.Sprintf("https://qyapi.weixin.qq.com/cgi-bin/gettoken?corpid=%s&corpsecret=%s", w.corpID, w.secret)
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken string `json:"access_token"`
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	if result.ErrCode != 0 {
		return fmt.Errorf("wecom token error: %s", result.ErrMsg)
	}
	w.accessToken = result.AccessToken
	return nil
}

type wecomXMLCallback struct {
	XMLName    xml.Name `xml:"xml"`
	ToUserName string   `xml:"ToUserName"`
	Encrypt    string   `xml:"Encrypt"`
}

type wecomDecryptedMsg struct {
	XMLName      xml.Name `xml:"xml"`
	FromUserName string   `xml:"FromUserName"`
	MsgType      string   `xml:"MsgType"`
	Content      string   `xml:"Content"`
	AgentID      string   `xml:"AgentID"`
}

func (w *WeComAdapter) handleCallback(rw http.ResponseWriter, r *http.Request) {
	// URL verification (GET): WeCom sends msg_signature, timestamp, nonce, echostr.
	if r.Method == http.MethodGet {
		echostr := r.URL.Query().Get("echostr")
		if w.token != "" {
			msgSig := r.URL.Query().Get("msg_signature")
			timestamp := r.URL.Query().Get("timestamp")
			nonce := r.URL.Query().Get("nonce")
			if !verifyWeComSignature(w.token, timestamp, nonce, echostr, msgSig) {
				http.Error(rw, "invalid signature", http.StatusUnauthorized)
				return
			}
		}
		if w.encodingKey != "" {
			if decrypted, err := w.decryptMsg(echostr); err == nil {
				fmt.Fprint(rw, string(decrypted))
				return
			}
		}
		fmt.Fprint(rw, echostr)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(rw, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(rw, "read error", http.StatusBadRequest)
		return
	}

	var callback wecomXMLCallback
	if err := xml.Unmarshal(body, &callback); err != nil {
		http.Error(rw, "invalid xml", http.StatusBadRequest)
		return
	}

	// Verify WeCom signature: sort([token, timestamp, nonce, encrypt]) → SHA1.
	if w.token != "" {
		msgSig := r.URL.Query().Get("msg_signature")
		timestamp := r.URL.Query().Get("timestamp")
		nonce := r.URL.Query().Get("nonce")
		if !verifyWeComSignature(w.token, timestamp, nonce, callback.Encrypt, msgSig) {
			http.Error(rw, "invalid signature", http.StatusUnauthorized)
			return
		}
	}

	// Decrypt message if encoding key is set
	var msg wecomDecryptedMsg
	if w.encodingKey != "" && callback.Encrypt != "" {
		decrypted, err := w.decryptMsg(callback.Encrypt)
		if err != nil {
			slog.Error("WeCom decrypt failed", "error", err)
			http.Error(rw, "decrypt error", http.StatusBadRequest)
			return
		}
		xml.Unmarshal(decrypted, &msg)
	} else {
		xml.Unmarshal(body, &msg)
	}

	if msg.MsgType != "text" {
		rw.WriteHeader(http.StatusOK)
		return
	}

	event := &gateway.MessageEvent{
		Text:        msg.Content,
		MessageType: gateway.MessageTypeText,
		Source: gateway.SessionSource{
			Platform: gateway.PlatformWeCom,
			ChatID:   msg.FromUserName,
			ChatType: "dm",
			UserID:   msg.FromUserName,
		},
	}

	w.EmitMessage(event)
	rw.WriteHeader(http.StatusOK)
}

// verifyWeComSignature validates the WeCom callback signature.
// It sorts [token, timestamp, nonce, encrypt] lexicographically, joins them,
// SHA1-hashes the result, and compares the hex digest with msgSignature.
func verifyWeComSignature(token, timestamp, nonce, encrypt, msgSignature string) bool {
	parts := []string{token, timestamp, nonce, encrypt}
	sort.Strings(parts)
	h := sha1.New()
	h.Write([]byte(strings.Join(parts, "")))
	computed := fmt.Sprintf("%x", h.Sum(nil))
	return computed == msgSignature
}

func (w *WeComAdapter) decryptMsg(encrypted string) ([]byte, error) {
	if w.encodingKey == "" {
		return nil, fmt.Errorf("no encoding key")
	}

	aesKey, err := base64.StdEncoding.DecodeString(w.encodingKey + "=")
	if err != nil {
		return nil, err
	}

	ciphertext, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, err
	}

	iv := aesKey[:16]
	mode := cipher.NewCBCDecrypter(block, iv)
	plaintext := make([]byte, len(ciphertext))
	mode.CryptBlocks(plaintext, ciphertext)

	// Remove PKCS7 padding
	if len(plaintext) == 0 {
		return nil, fmt.Errorf("empty plaintext")
	}
	padLen := int(plaintext[len(plaintext)-1])
	if padLen > len(plaintext) || padLen > aes.BlockSize {
		return nil, fmt.Errorf("invalid padding")
	}

	return plaintext[:len(plaintext)-padLen], nil
}
