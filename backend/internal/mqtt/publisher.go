package mqtt

import (
	"encoding/json"
	"fmt"
	"time"

	pahomqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/micro-trading-for-agent/backend/internal/logger"
)

const (
	// Topic prefixes for published MQTT messages.
	TopicAlert       = "trading/alert" // + "/{stock_code}"
	TopicLiquidation = "trading/liquidation"
	TopicReport      = "trading/report"

	// Alert event types.
	EventTargetHit   = "TARGET_HIT"
	EventStopHit     = "STOP_HIT"
	EventLiquidation = "LIQUIDATION"
	EventDailyReport = "DAILY_REPORT"
)

// AlertPayload is the JSON body published to MQTT topics.
type AlertPayload struct {
	Event        string    `json:"event"`
	StockCode    string    `json:"stock_code"`
	StockName    string    `json:"stock_name"`
	TriggerPrice float64   `json:"trigger_price"`
	TargetPrice  float64   `json:"target_price"`
	StopPrice    float64   `json:"stop_price"`
	ProfitPct    float64   `json:"profit_pct"`
	Timestamp    time.Time `json:"timestamp"`
	IsTest       bool      `json:"is_test"` // true이면 테스트 메시지 (장 외 디버그용)
}

// Publisher wraps a Paho MQTT client for fire-and-forget publishing.
type Publisher struct {
	client pahomqtt.Client
}

// NewPublisher connects to the MQTT broker and returns a Publisher.
// If the broker is unreachable, returns nil + error; the caller should handle
// gracefully (server continues without MQTT).
func NewPublisher(brokerURL, clientID string) (*Publisher, error) {
	opts := pahomqtt.NewClientOptions().
		AddBroker(brokerURL).
		SetClientID(clientID).
		SetConnectTimeout(5 * time.Second).
		SetAutoReconnect(true).
		SetMaxReconnectInterval(30 * time.Second).
		SetOnConnectHandler(func(_ pahomqtt.Client) {
			logger.Info("MQTT connected", map[string]any{"broker": brokerURL})
		}).
		SetConnectionLostHandler(func(_ pahomqtt.Client, err error) {
			logger.Error("MQTT connection lost", map[string]any{"error": err.Error()})
		})

	client := pahomqtt.NewClient(opts)
	token := client.Connect()
	token.Wait()
	if err := token.Error(); err != nil {
		return nil, fmt.Errorf("MQTT connect to %s: %w", brokerURL, err)
	}

	return &Publisher{client: client}, nil
}

// Publish serialises payload as JSON and publishes to topic with QoS 1.
func (p *Publisher) Publish(topic string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal MQTT payload: %w", err)
	}

	token := p.client.Publish(topic, 1, false, data)
	token.Wait()
	if err := token.Error(); err != nil {
		return fmt.Errorf("MQTT publish to %s: %w", topic, err)
	}
	return nil
}

// PublishAlert publishes a position alert (TARGET_HIT / STOP_HIT).
// isTest=true marks the message as a debug test (장 외 테스트용).
func (p *Publisher) PublishAlert(event, stockCode, stockName string,
	triggerPrice, targetPrice, stopPrice, filledPrice float64, isTest bool) {
	profitPct := 0.0
	if filledPrice > 0 {
		profitPct = (triggerPrice - filledPrice) / filledPrice * 100
	}
	payload := AlertPayload{
		Event:        event,
		StockCode:    stockCode,
		StockName:    stockName,
		TriggerPrice: triggerPrice,
		TargetPrice:  targetPrice,
		StopPrice:    stopPrice,
		ProfitPct:    profitPct,
		Timestamp:    time.Now().In(time.FixedZone("KST", 9*3600)),
		IsTest:       isTest,
	}
	topic := fmt.Sprintf("%s/%s", TopicAlert, stockCode)
	if err := p.Publish(topic, payload); err != nil {
		logger.Error("MQTT publish alert failed",
			map[string]any{"topic": topic, "error": err.Error()})
	} else {
		logger.Info("MQTT alert published",
			map[string]any{"event": event, "stock_code": stockCode, "price": triggerPrice})
	}
}

// Close disconnects the MQTT client.
func (p *Publisher) Close() {
	p.client.Disconnect(500)
}
