package consumer

import (
	"context"
	"errors"

	"github.com/IBM/sarama"
	"github.com/to404hanga/pkg404/logger"
	loggerv2 "github.com/to404hanga/pkg404/logger/v2"
)

type SaramaConsumer struct {
	group   sarama.ConsumerGroup
	topic   string
	handler sarama.ConsumerGroupHandler
	log     loggerv2.Logger
}

func NewSaramaConsumer(group sarama.ConsumerGroup, topic string, handler sarama.ConsumerGroupHandler, log loggerv2.Logger) Consumer {
	return &SaramaConsumer{
		group:   group,
		topic:   topic,
		handler: handler,
		log:     log,
	}
}

func (c *SaramaConsumer) Start(ctx context.Context) error {
	c.log.InfoContext(ctx, "Consumer starting", logger.String("topic", c.topic))
	for {
		if err := c.group.Consume(ctx, []string{c.topic}, c.handler); err != nil {
			if errors.Is(err, sarama.ErrClosedConsumerGroup) {
				return err
			}
			c.log.ErrorContext(ctx, "Error from consumer", logger.Error(err))
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
}
