package go_websocket_client

import (
	"fmt"
	"testing"
	"time"
)

var (
	connected = make(chan bool, 1)
	block     = make(chan bool, 1)
)

func TestReconnect(t *testing.T) {
	config := Config{
		Url:             "wss://ws.okex.com:8443/ws/v5/public?brokerId=9999",
		ProxyUrl:        "socks5://127.0.0.1:1086",
		OnMessage:       _onMessage,
		OnClose:         _onClose,
		OnConnect:       _onConnect,
		PingPeriod:      15 * time.Second,
		IsAutoReconnect: true,
	}
	client := NewClient(config)
	go client.Start()
	<-connected
	client.Subscribe([]byte(`{"op": "subscribe", "args": [{"channel": "tickers", "instId": "BTC-USDT", "instType": "SPOT"}]}`))
	// first time, manually close
	client.SendClose()
	// second time, manually close
	<-connected
	client.SendClose()
	time.Sleep(20 * time.Second)
	// test terminate
	client.Terminate()
	<-block
}

func _onMessage(msg []byte) {
	fmt.Println("OnMessage: ", string(msg))
}

func _onConnect() {
	fmt.Println("OnConnect")
	connected <- true
}

func _onClose(code int, text string) error {
	fmt.Println("OnClose", code, text)
	return nil
}
