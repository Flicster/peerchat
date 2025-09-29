package model

import "time"

type ChatMessage struct {
	Message    string    `json:"message"`
	SenderID   string    `json:"senderId"`
	SenderName string    `json:"senderName"`
	CreatedAt  time.Time `json:"createdAt"`
}

type LogMessage struct {
	Prefix  string `json:"prefix"`
	Message string `json:"message"`
}
