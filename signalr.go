package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"golang.org/x/net/websocket"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
	"os"
	"os/signal"
	"time"
)

type NegotiationParams struct {
	Url                     string `json:"Url"`
	ConnectionToken         string
	ConnectionId            string
	KeepAliveTimeout        float32
	DisconnectTimeout       float32
	ConnectionTimeout       float32
	TryWebSockets           bool
	ProtocolVersion         string
	TransportConnectTimeout float32
	LogPollDelay            float32
}

var scheme, addr, origin, hubname, logFile string

var connectionProtocol = "webSockets" // currently no support for server sent events

var msgSize = make([]byte, 512)

func main() {
	flag.StringVar(&scheme, "scheme", "https", "protocol for connecting - http(s)")
	flag.StringVar(&addr, "addr", "", "endpoint to connect to")
	flag.StringVar(&hubname, "hubname", "", "SignalR hub to connect to")
	flag.StringVar(&logFile, "logFile", "", "Output file for logs")

	flag.Parse()

	if addr == "" {
		log.Println("Address is empty")
		return
	}

	if hubname == "" {
		log.Println("Hubname is empty")
		return
	}

	SetupLogging()

	log.Println("Starting SignalR Connection")

	origin = fmt.Sprintf("%s://%s", scheme, addr)

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	ws, err := ConnectToSignalR()
	if err != nil {
		log.Println(err)
		if strings.Contains(err.Error(), "no such host"){
			os.Exit(1)
		}
		connectionTimer := time.NewTicker(time.Second * 30)
		for ok := true; ok; ok = (err != nil) {
			select {
			case <-connectionTimer.C:
				ws, err = ConnectToSignalR()
				if err != nil {
					if strings.Contains(err.Error(), "no such host"){
						os.Exit(1)
					}
					log.Println(err)
				}
			}
		}

		connectionTimer.Stop()
	}

	ticker := time.NewTicker(time.Second * 10)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			_, err := ws.Write([]byte("data="))
			if err != nil {
				log.Println("ERROR write:", err)
				ws.Close()

				ticker.Stop()
				ws, err = ConnectToSignalR()

				if err != nil {
					log.Println(err)
					connectionTimer := time.NewTicker(time.Second * 30)
					for ok := true; ok; ok = (err != nil) {
						select {
						case <-connectionTimer.C:
							ws, err = ConnectToSignalR()
							if err != nil {
								log.Println(err)
							}
						}
					}

					connectionTimer.Stop()
				}

				ticker = time.NewTicker(time.Second * 10)
				defer ticker.Stop()
			}
		case <-interrupt:
			log.Println("interrupt")
			// To cleanly close a connection, a client should send a close
			// frame and wait for the server to close the connection.
			_, err := ws.Write([]byte("close"))
			if err != nil {
				log.Println("write close:", err)
				return
			}
			select {
			case <-time.After(time.Second):
			}
			ws.Close()
			return
		}
	}
}

func ConnectToSignalR() (*websocket.Conn, error) {
	params, err := Negotiate(addr)
	if err != nil {
		return nil, err
	}

	cli := &http.Client{}

	connectionData := "[%7B%22Name%22:%22" + hubname + "%22%7D]"

	startUrl := BuildStartUrl(params.ProtocolVersion, connectionProtocol, connectionData, params.ConnectionToken)

	_, err = cli.Get(startUrl.String())
	if err != nil {
		return nil, err
	}

	connectUrl := BuildConnectUrl(params.ProtocolVersion, connectionProtocol, connectionData, params.ConnectionToken)

	ws, err := websocket.Dial(connectUrl.String(), "", origin)
	if err != nil {
		return nil, err
	}

	log.Printf("Connected to %s", addr)

	go func() {
		defer ws.Close()
		for {
			n, err := ws.Read(msgSize)
			if err != nil {
				log.Println("ERROR read:", err)
				return
			}
			if n > 2 {
				log.Printf("Received: %s.\n", msgSize[:n])
			}
		}
	}()

	return ws, nil
}

func Negotiate(addr string) (params NegotiationParams, err error) {
	params = NegotiationParams{}

	u := url.URL{Scheme: scheme, Host: addr, Path: "/signalr/negotiate"}

	client := &http.Client{}

	resp, err := client.Get(u.String())
	if err != nil {
		return
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}

	err = json.Unmarshal(body, &params)

	return
}

func BuildSendUrl(protocol, transport, connectionData, connectionToken string) url.URL {
	u := url.URL{Scheme: "wss", Host: addr, Path: "/signalr/send"}
	query := u.Query()
	query = AppendCommonParameters(query, protocol, transport, connectionData, connectionToken)
	u.RawQuery = query.Encode()
	u.RawQuery += "&connectionData=" + connectionData

	return u
}

func BuildStartUrl(protocol, transport, connectionData, connectionToken string) url.URL {
	u := url.URL{Scheme: scheme, Host: addr, Path: "/signalr/start"}
	query := u.Query()
	query = AppendCommonParameters(query, protocol, transport, connectionData, connectionToken)
	u.RawQuery = query.Encode()
	u.RawQuery = query.Encode()
	u.RawQuery += "&connectionData=" + connectionData

	return u
}

func BuildConnectUrl(protocol, transport, connectionData, connectionToken string) url.URL {
	u := url.URL{Scheme: "wss", Host: addr, Path: "/signalr/connect"}
	query := u.Query()
	query = AppendCommonParameters(query, protocol, transport, connectionData, connectionToken)
	u.RawQuery = query.Encode()
	u.RawQuery = query.Encode()
	u.RawQuery += "&connectionData=" + connectionData

	return u
}

func AppendCommonParameters(query url.Values,
	protocol, transport, connectionData, connectionToken string) url.Values {
	query = AppendProtocol(query, protocol)
	query = AppendTransport(query, transport)
	query = AppendConnectionToken(query, connectionToken)

	return query
}

func AppendProtocol(query url.Values, protocol string) url.Values {
	query.Set("clientProtocol", protocol)
	return query
}

func AppendTransport(query url.Values, transport string) url.Values {
	query.Set("transport", transport)
	return query
}

func AppendConnectionData(query url.Values, connectionData string) url.Values {
	query.Set("connectionData", connectionData)
	return query
}

func AppendConnectionToken(query url.Values, connectionToken string) url.Values {
	query.Set("connectionToken", connectionToken)
	return query
}

func SetupLogging() {
	if logFile == "" {
		return
	}

	if !FileExists(logFile) {
		nf, err := os.Create(logFile)
		if err != nil {
			fmt.Println("Error - Cannot create log file at " + logFile)
		}
		nf.Close()
	}
	f, err := os.OpenFile(logFile, os.O_RDWR|os.O_APPEND, 0660)
	if err != nil {
		fmt.Println("Error - Cannot get to log file at " + logFile)
		panic(err)
	}
	log.SetOutput(f)
	log.Print("---")
}

func FileExists(file string) bool {
	if _, err := os.Stat(file); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}
