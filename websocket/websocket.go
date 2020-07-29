package websocket

import (
	"context"
	"fmt"
	"go-websocket/server"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

var ws *WebSocket

var upgrader = websocket.Upgrader{
	ReadBufferSize:  2048,
	WriteBufferSize: 2048,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

type WebSocket struct {
	HttpPort   int
	MaxMsgs    int
	Register   chan *Client
	Unregister chan *Client
	Broadcast  chan string
	Clients    map[*Client]bool
	Msgs       []string
	Count      int
}

func init() {
	ws = &WebSocket{
		Count:      0,
		Msgs:       make([]string, 0),
		Clients:    make(map[*Client]bool),
		Register:   make(chan *Client, 100),
		Unregister: make(chan *Client, 100),
		Broadcast:  make(chan string, 100),
	}
	//Set it High or low based on your requirement
	server.RegisterService(ws, server.Low)
}

func (c *WebSocket) Init() (err error) {
	viper.SetDefault("http", 9000)
	c.HttpPort = viper.GetInt("http")

	viper.SetDefault("maxmsg", 10)
	c.MaxMsgs = viper.GetInt("maxmsg")
	return err
}

func (c *WebSocket) Run(ctx context.Context) error {
	address := fmt.Sprintf("0.0.0.0:%d", c.HttpPort)
	server := &http.Server{
		Addr: address,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/ws") {
				c.NewClient(w, r)
				return
			}
			http.ServeFile(w, r, "public/index.html")
		}),
	}
	go c.Start(ctx, server)
	log.WithField("Port", c.HttpPort).Info("websocket listening...")
	return server.ListenAndServe()
}

func upgrade(w http.ResponseWriter, r *http.Request) (*websocket.Conn, error) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func (c *WebSocket) NewClient(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrade(w, r)
	if err != nil {
		log.WithField("Error", err).Error("New client connection failed")
		return
	}
	client := &Client{
		Conn: conn,
	}
	c.Register <- client
	go client.listen()
}

func (c *WebSocket) Start(ctx context.Context, server *http.Server) {
	for {
		select {
		case <-ctx.Done():
			for client := range c.Clients {
				client.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				delete(ws.Clients, client)
			}
			log.Info("Stopping Websocket")
			server.Shutdown(ctx)
			return
		case client := <-c.Register:
			c.Clients[client] = true
			for _, m := range c.Msgs {
				client.Conn.WriteJSON(Message{Error: false, Msg: m})
			}
			break
		case client := <-c.Unregister:
			client.Conn.WriteMessage(websocket.CloseMessage, []byte{})
			delete(c.Clients, client)
			break
		case message := <-c.Broadcast:
			c.InsertMsg(message)
			for client := range c.Clients {
				if err := client.Conn.WriteJSON(Message{Error: false, Msg: message}); err != nil {
					log.WithField("Error", err).Error("Failed to broadcast")
					return
				}
			}
		}
	}
}

func (c *WebSocket) InsertMsg(msg string) {
	if c.Count > c.MaxMsgs {
		c.Msgs = append(c.Msgs[1:c.MaxMsgs], msg)
	} else {
		c.Msgs = append(c.Msgs, msg)
	}
	if c.Count < c.MaxMsgs {
		c.Count++
	}
}
