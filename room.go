package main

import (
	"github.com/gorilla/websocket"
	"log"
	"net/http"
	"trace"
)

type room struct {
	// messages channel
	forward chan []byte
	// channel for clients to join the room
	join chan *client
	// channel for clients to leave the room
	leave chan *client
	// list of all clients in the room.
	clients map[*client]bool
	// tracer with recieve information about all actiovity in the room.
	tracer trace.Tracer
}

func newRoom() *room {
	return &room{
		forward: make(chan []byte),
		join:    make(chan *client),
		leave:   make(chan *client),
		clients: make(map[*client]bool),
		tracer:  trace.Off(),
	}
}

func (r *room) run() {
	for {
		select {
		case client := <-r.join:
			// Joining the room. Add to the client list
			r.clients[client] = true
			r.tracer.Trace("New client joined")
		case client := <-r.leave:
			// Leaving the room. Remove from client list and close the client's
			// "send" channel, to prevent it from getting messages.
			delete(r.clients, client)
			close(client.send)
			r.tracer.Trace("Client left")
		case msg := <-r.forward:
			r.tracer.Trace("Messege recieved: ", string(msg))
			// Forwarm msg to all clients
			for client := range r.clients {
				select {
				case client.send <- msg:
					// Send the message.
					r.tracer.Trace("-- sent to client")
				default:
					// Failed to send the message
					delete(r.clients, client)
					close(client.send)
					r.tracer.Trace("-- failed to send, cleaned up")
				}
			}
		}
	}
}

const (
	socketBufferSize  = 1024
	messageBufferSize = 256
)

var upgrader = &websocket.Upgrader{ReadBufferSize: socketBufferSize, WriteBufferSize: socketBufferSize}

func (r *room) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	socket, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		log.Fatal("ServeHTTP:", err)
		return
	}
	client := &client{
		socket: socket,
		send:   make(chan []byte, messageBufferSize),
		room:   r,
	}
	r.join <- client
	defer func() { r.leave <- client }()
	go client.write()
	client.read()
}
