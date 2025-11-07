package consumer

import (
	"context"
	"fmt"

	"github.com/IBM/sarama"
	"github.com/to404hanga/pkg404/logger"
	loggerv2 "github.com/to404hanga/pkg404/logger/v2"
)

type MessageHandler func(ctx context.Context, msg *sarama.ConsumerMessage) error

type GroupHandler struct {
	handler MessageHandler
	log     loggerv2.Logger
}

func NewGroupHandler(handler MessageHandler, log loggerv2.Logger) sarama.ConsumerGroupHandler {
	return &GroupHandler{
		handler: handler,
		log:     log,
	}
}

func (h *GroupHandler) Setup(session sarama.ConsumerGroupSession) error {
	h.log.InfoContext(session.Context(), "Consumer group session setup", logger.String("claims", fmt.Sprintf("%v", session.Claims())))
	return nil
}

func (h *GroupHandler) Cleanup(session sarama.ConsumerGroupSession) error {
	h.log.InfoContext(session.Context(), "Consumer group session cleanup")
	return nil
}

func (h *GroupHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for msg := range claim.Messages() {
		if err := h.handler(session.Context(), msg); err != nil {
			h.log.ErrorContext(session.Context(), "Failed to process message", logger.Error(err), logger.String("partition", fmt.Sprintf("%d", msg.Partition)), logger.String("offset", fmt.Sprintf("%d", msg.Offset)))
		}
		session.MarkMessage(msg, "")
	}
	return nil
}
