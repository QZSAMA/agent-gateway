package util

import (
	"crypto/rand"
	"fmt"
	"time"
)

func NewID(prefix string) string {
	b := make([]byte, 8)
	rand.Read(b)
	return fmt.Sprintf("%s_%x_%d", prefix, b, time.Now().UnixMilli())
}

func NewSessionID() string {
	return NewID("sess")
}

func NewMessageID() string {
	return NewID("msg")
}

func NewTaskID() string {
	return NewID("task")
}

func NewApprovalID() string {
	return NewID("appr")
}
