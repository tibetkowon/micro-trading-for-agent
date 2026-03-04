package kis

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/micro-trading-for-agent/backend/internal/logger"
)

const (
	kisWSURL = "wss://openapiwss.koreainvestment.com:21000"

	// TrIDPrice is the TR_ID for real-time stock execution price (국내주식 실시간체결가).
	TrIDPrice = "H0STCNT0"
	// TrIDExecNotice is the TR_ID for real-time execution notice (국내주식 실시간체결통보).
	TrIDExecNotice = "H0STCNI0"
)

// PriceEvent carries a real-time price update from KIS WebSocket.
type PriceEvent struct {
	StockCode string
	Price     float64
	Timestamp time.Time
}

// ExecEvent carries a real-time execution (체결) notice from KIS WebSocket.
type ExecEvent struct {
	KISOrderID  string
	StockCode   string
	FilledQty   int
	FilledPrice float64
	SellBuyDiv  string // "01"=매도, "02"=매수
	CntgYN      string // "2"=체결, "1"=주문접수
	Timestamp   time.Time
}

// WebSocketClient manages the KIS WebSocket connection for real-time data.
type WebSocketClient struct {
	approvalKey string
	htsID       string // HTS ID — tr_key for 체결통보

	mu            sync.RWMutex
	conn          *websocket.Conn
	aesKeys       map[string]aesKey // trID → aes credentials per subscription
	subscriptions map[string]string // trID → trKey

	PriceCh chan PriceEvent
	ExecCh  chan ExecEvent

	closed chan struct{}
}

type aesKey struct {
	key string
	iv  string
}

// NewWebSocketClient creates a KIS WebSocket client.
// approvalKey: from GetApprovalKey(). htsID: KIS HTS 아이디 (for 체결통보).
func NewWebSocketClient(approvalKey, htsID string) *WebSocketClient {
	return &WebSocketClient{
		approvalKey:   approvalKey,
		htsID:         htsID,
		aesKeys:       make(map[string]aesKey),
		subscriptions: make(map[string]string),
		PriceCh:       make(chan PriceEvent, 256),
		ExecCh:        make(chan ExecEvent, 64),
		closed:        make(chan struct{}),
	}
}

// Connect opens the WebSocket connection and starts the read loop.
func (c *WebSocketClient) Connect(ctx context.Context) error {
	dialer := websocket.DefaultDialer
	conn, _, err := dialer.DialContext(ctx, kisWSURL, nil)
	if err != nil {
		return fmt.Errorf("ws dial: %w", err)
	}
	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()

	logger.Info("KIS WebSocket connected", map[string]any{"url": kisWSURL})
	return nil
}

// IsConnected reports whether the WebSocket connection is currently open.
func (c *WebSocketClient) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.conn != nil
}

// SetApprovalKey updates the approval key (used when re-fetching before market open).
func (c *WebSocketClient) SetApprovalKey(key string) {
	c.mu.Lock()
	c.approvalKey = key
	c.mu.Unlock()
}

// Disconnect closes the WebSocket connection gracefully.
func (c *WebSocketClient) Disconnect() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		c.conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		c.conn.Close()
		c.conn = nil
		logger.Info("KIS WebSocket disconnected", nil)
	}
}

// Subscribe sends a subscription request for the given TR_ID and TR_KEY.
// trType: "1"=등록, "2"=해제
func (c *WebSocketClient) Subscribe(trID, trKey string) error {
	return c.sendSubscription("1", trID, trKey)
}

// Unsubscribe cancels a subscription.
func (c *WebSocketClient) Unsubscribe(trID, trKey string) error {
	return c.sendSubscription("2", trID, trKey)
}

func (c *WebSocketClient) sendSubscription(trType, trID, trKey string) error {
	msg := map[string]any{
		"header": map[string]string{
			"approval_key": c.approvalKey,
			"custtype":     "P",
			"tr_type":      trType,
			"content-type": "utf-8",
		},
		"body": map[string]any{
			"input": map[string]string{
				"tr_id":  trID,
				"tr_key": trKey,
			},
		},
	}
	data, _ := json.Marshal(msg)

	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()
	if conn == nil {
		return fmt.Errorf("WebSocket not connected")
	}

	if trType == "1" {
		c.mu.Lock()
		c.subscriptions[trID] = trKey
		c.mu.Unlock()
	} else {
		c.mu.Lock()
		delete(c.subscriptions, trID)
		c.mu.Unlock()
	}

	return conn.WriteMessage(websocket.TextMessage, data)
}

// StartReadLoop reads messages from the WebSocket and dispatches events.
// Blocks until the connection is closed or ctx is cancelled.
func (c *WebSocketClient) StartReadLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		c.mu.RLock()
		conn := c.conn
		c.mu.RUnlock()
		if conn == nil {
			return
		}

		_, msg, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				logger.Info("KIS WebSocket closed normally", nil)
			} else {
				logger.Error("KIS WebSocket read error", map[string]any{"error": err.Error()})
			}
			return
		}

		c.handleMessage(msg)
	}
}

// StartWithReconnect runs the WebSocket lifecycle (connect → subscribe → read loop)
// and automatically reconnects on failure until ctx is cancelled.
func (c *WebSocketClient) StartWithReconnect(ctx context.Context) {
	backoff := 5 * time.Second
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if err := c.Connect(ctx); err != nil {
			logger.Error("KIS WebSocket connect failed, retrying",
				map[string]any{"error": err.Error(), "backoff": backoff.String()})
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			continue
		}

		// Re-subscribe all previously registered subscriptions.
		c.mu.RLock()
		subs := make(map[string]string, len(c.subscriptions))
		for k, v := range c.subscriptions {
			subs[k] = v
		}
		c.mu.RUnlock()
		for trID, trKey := range subs {
			if err := c.Subscribe(trID, trKey); err != nil {
				logger.Error("re-subscribe failed", map[string]any{"tr_id": trID, "error": err.Error()})
			}
		}

		c.StartReadLoop(ctx)

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
	}
}

// SubscribePrice adds a stock to real-time price updates.
func (c *WebSocketClient) SubscribePrice(stockCode string) error {
	return c.Subscribe(TrIDPrice, stockCode)
}

// UnsubscribePrice removes a stock from real-time price updates.
func (c *WebSocketClient) UnsubscribePrice(stockCode string) error {
	return c.Unsubscribe(TrIDPrice, stockCode)
}

// SubscribeExecNotice subscribes to execution notices using the HTS ID.
func (c *WebSocketClient) SubscribeExecNotice() error {
	if c.htsID == "" {
		return fmt.Errorf("KIS_HTS_ID not configured — cannot subscribe to execution notices")
	}
	return c.Subscribe(TrIDExecNotice, c.htsID)
}

// --- Internal message parsing ---

func (c *WebSocketClient) handleMessage(raw []byte) {
	text := string(raw)

	// JSON 형태 응답 (구독 성공/실패 통보)
	if strings.HasPrefix(strings.TrimSpace(text), "{") {
		c.handleJSONMessage(raw)
		return
	}

	// Pipe-delimited 데이터 응답: encFlag|trID|count|data
	parts := strings.SplitN(text, "|", 4)
	if len(parts) < 4 {
		return
	}

	encFlag := parts[0] // "0"=plain, "1"=AES256 encrypted
	trID := parts[1]
	data := parts[3]

	if encFlag == "1" {
		decrypted, err := c.decrypt(trID, data)
		if err != nil {
			logger.Error("ws decrypt error", map[string]any{"tr_id": trID, "error": err.Error()})
			return
		}
		data = decrypted
	}

	switch trID {
	case TrIDPrice:
		c.parsePriceData(data)
	case TrIDExecNotice:
		c.parseExecData(data)
	}
}

// handleJSONMessage processes subscribe success/failure JSON responses.
// It extracts AES key/iv for encrypted streams.
func (c *WebSocketClient) handleJSONMessage(raw []byte) {
	var resp struct {
		Header struct {
			TrID string `json:"tr_id"`
		} `json:"header"`
		Body struct {
			RtCd   string `json:"rt_cd"`
			MsgCd  string `json:"msg_cd"`
			Msg1   string `json:"msg1"`
			Output struct {
				IV  string `json:"iv"`
				Key string `json:"key"`
			} `json:"output"`
		} `json:"body"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return
	}

	if resp.Body.RtCd != "0" {
		logger.Error("KIS WebSocket subscribe failed",
			map[string]any{"tr_id": resp.Header.TrID, "msg": resp.Body.Msg1})
		return
	}

	logger.Info("KIS WebSocket subscribe success",
		map[string]any{"tr_id": resp.Header.TrID, "msg": resp.Body.Msg1})

	// Store AES credentials for encrypted streams (체결통보).
	if resp.Body.Output.Key != "" && resp.Body.Output.IV != "" {
		c.mu.Lock()
		c.aesKeys[resp.Header.TrID] = aesKey{
			key: resp.Body.Output.Key,
			iv:  resp.Body.Output.IV,
		}
		c.mu.Unlock()
	}
}

// parsePriceData parses a H0STCNT0 pipe-delimited price record.
// Field positions (^-separated): 0=stockCode, 1=time, 2=currentPrice
func (c *WebSocketClient) parsePriceData(data string) {
	// Multiple records may be batched; each is ^-delimited.
	// 데이터 건수(3번째 필드)만큼 반복 처리. 단순화: 종목코드와 현재가만 필요.
	fields := strings.Split(data, "^")
	if len(fields) < 3 {
		return
	}

	stockCode := fields[0]
	priceStr := fields[2]
	if stockCode == "" || priceStr == "" {
		return
	}

	var price float64
	fmt.Sscanf(priceStr, "%f", &price)
	if price <= 0 {
		return
	}

	select {
	case c.PriceCh <- PriceEvent{
		StockCode: stockCode,
		Price:     price,
		Timestamp: time.Now(),
	}:
	default:
		// Drop if channel is full (backpressure protection)
	}
}

// parseExecData parses a H0STCNI0 execution notice record.
// Fields: CUST_ID, ACNT_NO, ODER_NO, OODER_NO, SELN_BYOV_CLS, RCTF_CLS, ODER_KIND,
//
//	ODER_COND, STCK_SHRN_ISCD(8), CNTG_QTY(9), CNTG_UNPR(10), STCK_CNTG_HOUR(11),
//	RFUS_YN(12), CNTG_YN(13), ACPT_YN(14), ...
func (c *WebSocketClient) parseExecData(data string) {
	fields := strings.Split(data, "^")
	if len(fields) < 14 {
		return
	}

	kisOrderID := fields[2]   // ODER_NO
	sellBuyDiv := fields[4]   // SELN_BYOV_CLS: "01"=매도, "02"=매수
	stockCode := fields[8]    // STCK_SHRN_ISCD
	cntgQtyStr := fields[9]   // CNTG_QTY
	cntgUnprStr := fields[10] // CNTG_UNPR
	cntgYN := fields[13]      // CNTG_YN: "2"=체결, "1"=접수

	if cntgYN != "2" {
		return // 체결 이벤트만 처리
	}

	var qty int
	var price float64
	fmt.Sscanf(cntgQtyStr, "%d", &qty)
	fmt.Sscanf(cntgUnprStr, "%f", &price)

	select {
	case c.ExecCh <- ExecEvent{
		KISOrderID:  kisOrderID,
		StockCode:   stockCode,
		FilledQty:   qty,
		FilledPrice: price,
		SellBuyDiv:  sellBuyDiv,
		CntgYN:      cntgYN,
		Timestamp:   time.Now(),
	}:
	default:
	}
}

// decrypt deciphers AES-256-CBC encrypted WebSocket data.
func (c *WebSocketClient) decrypt(trID, encData string) (string, error) {
	c.mu.RLock()
	creds, ok := c.aesKeys[trID]
	c.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("no AES key for tr_id=%s", trID)
	}

	cipherText, err := base64.StdEncoding.DecodeString(encData)
	if err != nil {
		return "", fmt.Errorf("base64 decode: %w", err)
	}

	block, err := aes.NewCipher([]byte(creds.key))
	if err != nil {
		return "", fmt.Errorf("aes cipher: %w", err)
	}

	if len(cipherText)%aes.BlockSize != 0 {
		return "", fmt.Errorf("ciphertext length not a multiple of block size")
	}

	mode := cipher.NewCBCDecrypter(block, []byte(creds.iv))
	mode.CryptBlocks(cipherText, cipherText)

	// Remove PKCS7 padding
	padLen := int(cipherText[len(cipherText)-1])
	if padLen > aes.BlockSize || padLen == 0 {
		return "", fmt.Errorf("invalid PKCS7 padding")
	}
	return string(cipherText[:len(cipherText)-padLen]), nil
}
