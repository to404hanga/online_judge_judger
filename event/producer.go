package event

import (
	"context"

	"github.com/IBM/sarama"
)

type Producer interface {
	Produce(ctx context.Context, msg *sarama.ProducerMessage) (int32, int64, error)
}