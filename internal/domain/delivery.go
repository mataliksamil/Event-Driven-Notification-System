package domain

import "context"

type DeliveryClient interface {
	Send(ctx context.Context, recipient string, channel Channel, content string) error
}