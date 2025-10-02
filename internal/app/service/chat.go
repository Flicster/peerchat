package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Flicster/peerchat/internal/app/model"
	"github.com/Flicster/peerchat/internal/app/storage"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
)

const (
	defaultUser = "incognito"
	defaultRoom = "lobby"
)

type ChatRoom struct {
	Host     *P2P
	Inbound  chan model.ChatMessage
	Outbound chan model.ChatMessage
	Logs     chan model.LogMessage
	RoomName string
	UserName string
	History  []model.ChatMessage

	peerId  peer.ID
	ctx     context.Context
	cancel  context.CancelFunc
	topic   *pubsub.Topic
	sub     *pubsub.Subscription
	storage *storage.File
}

func NewChatRoom(p2phost *P2P, username string, room string) (*ChatRoom, error) {
	topic, err := p2phost.PubSub.Join(fmt.Sprintf("room-peerchat-%s", room))
	if err != nil {
		return nil, fmt.Errorf("join pub sub: %w", err)
	}

	sub, err := topic.Subscribe()
	if err != nil {
		return nil, fmt.Errorf("subscribe room: %w", err)
	}

	if username == "" {
		username = defaultUser
	}
	if room == "" {
		room = defaultRoom
	}
	stor, err := storage.NewFile(room)
	if err != nil {
		return nil, fmt.Errorf("create storage: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	chatroom := &ChatRoom{
		Host:     p2phost,
		Inbound:  make(chan model.ChatMessage),
		Outbound: make(chan model.ChatMessage),
		Logs:     make(chan model.LogMessage),
		ctx:      ctx,
		cancel:   cancel,
		topic:    topic,
		sub:      sub,
		storage:  stor,

		RoomName: room,
		UserName: username,
		peerId:   p2phost.GetPeerID(),
	}

	go chatroom.SubLoop()
	go chatroom.PubLoop()
	err = chatroom.LoadHistory()
	if err != nil {
		return nil, fmt.Errorf("get history: %w", err)
	}
	return chatroom, nil
}

// PubLoop publishes a chatMessage
// to the PubSub topic until the pubsub context closes
func (cr *ChatRoom) PubLoop() {
	for {
		select {
		case <-cr.ctx.Done():
			return

		case message := <-cr.Outbound:
			messagebytes, err := json.Marshal(message)
			if err != nil {
				cr.Logs <- model.LogMessage{Prefix: "system", Message: "could not marshal JSON"}
				continue
			}

			err = cr.topic.Publish(cr.ctx, messagebytes)
			if err != nil {
				cr.Logs <- model.LogMessage{Prefix: "system", Message: "could not publish to topic"}
				continue
			}
			_ = cr.storage.SaveMessage(string(messagebytes))
		}
	}
}

// SubLoop continuously reads from the subscription
// until either the subscription or pubsub context closes.
// The received message is parsed sent into the inbound channel
func (cr *ChatRoom) SubLoop() {
	for {
		select {
		case <-cr.ctx.Done():
			return

		default:
			message, err := cr.sub.Next(cr.ctx)
			if err != nil {
				close(cr.Inbound)
				cr.Logs <- model.LogMessage{Prefix: "system", Message: "subscription has closed"}
				return
			}
			if message.ReceivedFrom == cr.peerId {
				continue
			}
			cm := &model.ChatMessage{}
			err = json.Unmarshal(message.Data, cm)
			if err != nil {
				cr.Logs <- model.LogMessage{Prefix: "system", Message: "could not unmarshal JSON"}
				continue
			}
			cr.Inbound <- *cm
		}
	}
}

func (cr *ChatRoom) PeerList() []peer.ID {
	return cr.topic.ListPeers()
}

func (cr *ChatRoom) LoadHistory() error {
	var err error
	cr.History, err = cr.storage.LoadMessages()
	if err != nil {
		return fmt.Errorf("load history: %w", err)
	}
	return nil
}

func (cr *ChatRoom) ClearHistory() error {
	return cr.storage.Clear()
}

func (cr *ChatRoom) Exit() {
	defer cr.cancel()
	_ = cr.storage.Close()
	cr.sub.Cancel()
	_ = cr.topic.Close()
}

func (cr *ChatRoom) UpdateUser(username string) {
	username = strings.ReplaceAll(username, " ", "")
	username = strings.ReplaceAll(username, "\t", "")
	username = strings.ReplaceAll(username, "\n", "")
	username = strings.ReplaceAll(username, "\r", "")
	cr.UserName = username
}
