package main

import (
	"log"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
)

type Client struct {
	isClosing bool
	username  string
	mu        sync.Mutex
}

type Message struct {
	Username string    `json:"username"`
	Content  string    `json:"content"`
	Time     time.Time `json:"time"`
}

var clients = make(map[*websocket.Conn]*Client)
var register = make(chan *websocket.Conn)
var broadcast = make(chan Message)
var unregister = make(chan *websocket.Conn)

func runHub() {
	for {
		select {
		case connection := <-register:
			clients[connection] = &Client{}
			log.Println("connection registered")

		case message := <-broadcast:
			log.Printf("message received from %s: %s", message.Username, message.Content)
			for connection, c := range clients {
				go func(connection *websocket.Conn, c *Client) {
					c.mu.Lock()
					defer c.mu.Unlock()
					if c.isClosing {
						return
					}
					if err := connection.WriteJSON(message); err != nil {
						c.isClosing = true
						log.Println("write error:", err)

						connection.WriteMessage(websocket.CloseMessage, []byte{})
						connection.Close()
						unregister <- connection
					}
				}(connection, c)
			}

		case connection := <-unregister:
			delete(clients, connection)
			log.Println("connection unregistered")
		}
	}
}

func main() {
	app := fiber.New()

	app.Static("/", "./public")

	app.Use("/ws", func(c *fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			c.Locals("allowed", true)
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	})

	go runHub()

	app.Get("/ws", websocket.New(func(c *websocket.Conn) {
		defer func() {
			unregister <- c
			c.Close()
		}()

		register <- c

		var username string
		if err := c.ReadJSON(&username); err != nil {
			log.Println("error reading username:", err)
			return
		}
		clients[c].username = username
		log.Printf("User %s joined the chat", username)

		for {
			var message Message
			err := c.ReadJSON(&message)
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("Unexpected close error: %v", err)
				} else {
					log.Printf("Expected close: %v", err)
				}
				break
			}

			message.Username = username
			message.Time = time.Now()
			broadcast <- message
		}
	}))

	log.Fatal(app.Listen(":8080"))
}
