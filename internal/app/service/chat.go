package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Flicster/peerchat/internal/app/model"
	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
)

const (
	defaultUser = "incognito"
	defaultRoom = "lobby"
)

type chatlog struct {
	logPrefix string
	logMsg    string
}

type ChatRoom struct {
	Host     *P2P
	Inbound  chan model.ChatMessage
	Outbound chan model.ChatMessage
	Logs     chan chatlog
	RoomName string
	UserName string

	peerId peer.ID
	ctx    context.Context
	cancel context.CancelFunc
	topic  *pubsub.Topic
	sub    *pubsub.Subscription
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

	ctx, cancel := context.WithCancel(context.Background())
	chatroom := &ChatRoom{
		Host:     p2phost,
		Inbound:  make(chan model.ChatMessage),
		Outbound: make(chan model.ChatMessage),
		Logs:     make(chan chatlog),

		ctx:    ctx,
		cancel: cancel,
		topic:  topic,
		sub:    sub,

		RoomName: room,
		UserName: username,
		peerId:   p2phost.Host.ID(),
	}

	go chatroom.SubLoop()
	go chatroom.PubLoop()

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
				cr.Logs <- chatlog{logPrefix: "puberr", logMsg: "could not marshal JSON"}
				continue
			}

			err = cr.topic.Publish(cr.ctx, messagebytes)
			if err != nil {
				cr.Logs <- chatlog{logPrefix: "puberr", logMsg: "could not publish to topic"}
				continue
			}
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
				cr.Logs <- chatlog{logPrefix: "suberr", logMsg: "subscription has closed"}
				return
			}
			if message.ReceivedFrom == cr.peerId {
				continue
			}
			cm := &model.ChatMessage{}
			err = json.Unmarshal(message.Data, cm)
			if err != nil {
				cr.Logs <- chatlog{logPrefix: "suberr", logMsg: "could not unmarshal JSON"}
				continue
			}
			cr.Inbound <- *cm
		}
	}
}

func (cr *ChatRoom) PeerList() []peer.ID {
	return cr.topic.ListPeers()
}

func (cr *ChatRoom) Exit() {
	defer cr.cancel()

	cr.sub.Cancel()
	_ = cr.topic.Close()
}

func (cr *ChatRoom) UpdateUser(username string) {
	cr.UserName = username
}
