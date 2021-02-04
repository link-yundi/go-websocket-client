# go-websocket-client
>  golang simply implement of  websocket client base on **gorilla/websocket**

## install

```sh
go get -u github.com/link-yundi/go-websocket-client
```

## example

### configuration

```go
import github.com/link-yundi/go-websocket-client
func main() {
    ...
   	config := Config{
		Url:             "ws host",
		ProxyUrl:        "proxy host",
		PingPeriod:      15 * time.Second,
		IsAutoReconnect: true,
	}
	client := NewClient(config) 
    ...
}

```

### start client

```go
...
client.Start()
...
```

### subscribe

```go
...
client.Subscribe([]byte(`some event string`))
...
```

### auto reconnect

```go
...
config := Config{
    	...
    	IsAutoReconnect: true,
    	...
	}
...
```

### close client

```go
...
client.Terminate()
...
```

### customize message-handler

```go
func onMessage(msg []byte) {
    // do somthing you want
	fmt.Println("OnMessage: ", string(msg))
}

func onConnect() {
    // do somthing you want
	fmt.Println("OnConnect")
}

func onClose(code int, text string) error {
    // do somthing you want
	fmt.Println("OnClose", code, text)
	return nil
}

func main() {
    config := Config{
		...
		OnMessage:       onMessage,
		OnClose:         onClose,
		OnConnect:       onConnect,
		PingPeriod:      15 * time.Second,
		IsAutoReconnect: true,
        ...
	}
    client := NewClient(config)
    go client.Start()
    // or client.Start()
}
```

