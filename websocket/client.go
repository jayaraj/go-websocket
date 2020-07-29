package websocket

import (
	"fmt"

	"github.com/Knetic/govaluate"
	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
)

type Client struct {
	Conn *websocket.Conn
}

type Message struct {
	Error bool   `json:"error"`
	Msg   string `json:"msg"`
}

func (c *Client) listen() {
	defer func() {
		if response := recover(); response != nil {
			log.WithField("Error", response).Error("Panic recovered")
		}
		ws.Unregister <- c
		c.Conn.Close()
	}()

	for {
		msgType, p, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.WithField("Error", err).Error("Unexpected error from client")
			}
			return
		}
		if msgType == websocket.TextMessage {

			expression, err := govaluate.NewEvaluableExpression(string(p))
			parameters := make(map[string]interface{})
			result, err := expression.Evaluate(parameters)
			if err == nil {
				ws.Broadcast <- fmt.Sprintf("%s = %v", string(p), result)
			} else {
				c.Conn.WriteJSON(Message{Error: true, Msg: "Expression is not valid"})
				log.WithField("Error", err).Error("Expression evaluation failed")
			}
		}
	}
}
