package notification

import "context"

type Sender interface {
	Send(ctx context.Context, message Message) error
}

type Service struct {
	sender Sender
}

func NewService(sender Sender) *Service {
	return &Service{sender: sender}
}
