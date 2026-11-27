package models

import (
	"encoding/json"
	"time"
)

type MessageStatus int16

const (
	MessagePending MessageStatus = iota
	MessageProcessing
	MessageDone
	MessageFailed
)

type Message struct {
	ID           int64
	QueueName    string
	Handler      string
	Payload      json.RawMessage
	Status       MessageStatus
	Priority     int16
	Attempts     int16
	MaxAttempts  int16
	AvailableAt  time.Time
	ErrorMessage *string
	CreatedAt    time.Time
}
