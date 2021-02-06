package go_websocket_client

import (
	"context"
	"errors"
	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"sync"
	"time"
)

var (
	logger *logrus.Logger
	cxt    context.Context
	cancel context.CancelFunc
)

func init() {
	logrus.SetFormatter(&logrus.JSONFormatter{})
	logrus.SetOutput(os.Stderr)
	logger = logrus.New()
	cxt, cancel = context.WithCancel(context.Background())
}

type Config struct {
	Url              string
	ProxyUrl         string
	ReqHeaders       map[string][]string
	PingPeriod       time.Duration
	IsAutoReconnect  bool
	OnConnect        func()
	OnMessage        func([]byte)
	OnClose          func(int, string) error
	OnError          func(err error)
	ReadDeadlineTime time.Duration
	ReconnectPeriod  time.Duration
}

type Client struct {
	Config
	dialer             *websocket.Dialer
	conn               *websocket.Conn
	writeBuffer        chan []byte
	closeMessageBuffer chan []byte
	Subs               [][]byte
	reconnectLock      *sync.Mutex
	waiter             *sync.WaitGroup
	open               chan bool
}

func NewClient(config Config) *Client {
	client := &Client{Config: config}
	client.dialer = &websocket.Dialer{
		Proxy:             http.ProxyFromEnvironment,
		HandshakeTimeout:  30 * time.Second,
		EnableCompression: true,
	}
	if client.ProxyUrl != "" {
		proxy, err := url.Parse(client.ProxyUrl)
		if err != nil {
			logger.Panic(err)
		} else {
			client.dialer.Proxy = http.ProxyURL(proxy)
		}
	}
	if config.PingPeriod == 0 {
		client.PingPeriod = 15 * time.Second
	}
	if config.ReadDeadlineTime == 0 {
		client.ReadDeadlineTime = 2 * client.PingPeriod
	}
	client.reconnectLock = new(sync.Mutex)
	client.waiter = &sync.WaitGroup{}
	return client
}

func (client *Client) dial() error {

	conn, resp, err := client.dialer.Dial(client.Url, http.Header(client.ReqHeaders))
	if err != nil {
		if resp != nil {
			dumpData, _ := httputil.DumpResponse(resp, true)
			err_ := errors.New(string(dumpData))
			logger.Error(err_)
		}
		logger.Errorf("websocket-client dial %s fail", client.Url)
		return err
	}
	client.conn = conn
	client.conn.SetReadDeadline(time.Now().Add(client.ReadDeadlineTime))
	dumpData, _ := httputil.DumpResponse(resp, true)
	logger.Infof("websocket-client connected to %s. response from server: \n %s", client.Url, string(dumpData))
	if client.OnConnect != nil {
		client.OnConnect()
	}
	if client.OnClose != nil {
		client.conn.SetCloseHandler(client.OnClose)
	}

	client.conn.SetPongHandler(func(appData string) error {
		client.conn.SetReadDeadline(time.Now().Add(client.ReadDeadlineTime))
		return nil
	})

	return nil
}

func (client *Client) initBufferChan() {
	client.open = make(chan bool)
	client.writeBuffer = make(chan []byte, 10)
	client.closeMessageBuffer = make(chan []byte, 10)
}

func (client *Client) Start() {
	// buffer channel reset
	client.initBufferChan()
	// dial
	err := client.dial()
	if err != nil {
		logger.Error("websocket-client start error:", err)
		if client.IsAutoReconnect {
			client.reconnect(10)
		}
	}
	// start read goroutine and write goroutine
	for {
		client.waiter.Add(2)
		go client.write()
		go client.read()
		client.waiter.Wait()
		close(client.open)
		if client.IsAutoReconnect {
			client.reconnect(10)
		} else {
			logger.Info("websocket-client closed. bye")
			return
		}
	}
}

func (client *Client) write() {
	var err error
	ctx_w, cancel_w := context.WithCancel(cxt)
	pingTicker := time.NewTicker(client.PingPeriod)
	defer func() {
		pingTicker.Stop()
		cancel_w()
		client.waiter.Done()
	}()

	for {
		select {
		case <-ctx_w.Done():
			logger.Warn("websocket-client connect closing, exit writing progress...")
			return
		case d := <-client.writeBuffer:
			err = client.conn.WriteMessage(websocket.TextMessage, d)
		case d := <-client.closeMessageBuffer:
			err = client.conn.WriteMessage(websocket.CloseMessage, d)
		case <-pingTicker.C:
			err = client.conn.WriteMessage(websocket.PingMessage, []byte("ping"))
		default:
			err = nil
		}
		if err != nil {
			logger.Errorf("write error: %v", err)
			return
		}
	}
}

func (client *Client) read() {
	ctx_r, cancel_r := context.WithCancel(cxt)
	defer func() {
		client.conn.Close()
		cancel_r()
		client.waiter.Done()
	}()
	for {
		select {
		case <-ctx_r.Done():
			logger.Warn("websocket-client connect closing, exit receiving progress...")
			return
		default:
			client.conn.SetReadDeadline(time.Now().Add(client.ReadDeadlineTime))
			_, msg, err := client.conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure) {
					logger.Errorf("read error: %v", err)
				}
				return
			}
			if client.OnMessage != nil {
				client.OnMessage(msg)
			}
		}
	}
}

func (client *Client) Send(msg []byte) {
	client.writeBuffer <- msg
}

func (client *Client) SendClose() {
	client.closeMessageBuffer <- websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")
}

func (client *Client) Subscribe(data []byte) {
	client.writeBuffer <- data
	client.Subs = append(client.Subs, data)
}

func (client *Client) Terminate() {

	client.reconnectLock.Lock()
	client.IsAutoReconnect = false
	defer client.reconnectLock.Unlock()
	cancel()
	// waiting for read_goroutine and write_goroutine are both closed
	<-client.open
}

func (client *Client) reconnect(retry int) {
	client.reconnectLock.Lock()
	defer client.reconnectLock.Unlock()
	var err error
	client.initBufferChan()
	for i := 0; i < retry; i++ {
		err = client.dial()
		if err != nil {
			logger.Errorf("websocket-client reconnect fail: %d", i)
		} else {
			break
		}
		time.Sleep(client.ReconnectPeriod)
	}
	if err != nil {
		logger.Error("websocket-client retry reconnect fail. exiting....")
		client.Terminate()
	} else {
		for _, sub := range client.Subs {
			logger.Info("re-subscribe: ", string(sub))
			client.Send(sub)
		}
	}
}
