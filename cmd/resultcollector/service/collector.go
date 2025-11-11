package service

import (
	"context"
	"fmt"
	"time"

	"github.com/IBM/sarama"
	"github.com/gogo/protobuf/proto"
	ojmodel "github.com/to404hanga/online_judge_common/model"
	ojconstants "github.com/to404hanga/online_judge_common/proto/constants"
	pbjudgeresult "github.com/to404hanga/online_judge_common/proto/gen/judgeresult"
	"github.com/to404hanga/online_judge_controller/service"
	"github.com/to404hanga/online_judge_judger/consumer"
	"github.com/to404hanga/pkg404/gotools/retry"
	"github.com/to404hanga/pkg404/logger"
	loggerv2 "github.com/to404hanga/pkg404/logger/v2"
	"gorm.io/gorm"
)

type ResultCollectorService struct {
	log        loggerv2.Logger
	db         *gorm.DB
	consumer   consumer.Consumer
	rankingSvc service.RankingService
}

func NewResultCollectorService(log loggerv2.Logger, cg sarama.ConsumerGroup, db *gorm.DB, rankingSvc service.RankingService) *ResultCollectorService {
	s := &ResultCollectorService{
		log:        log,
		db:         db,
		rankingSvc: rankingSvc,
	}
	handler := consumer.NewGroupHandler(s.handleResult, log)
	c := consumer.NewSaramaConsumer(cg, ojconstants.JudgeResultTopic, handler, log)
	s.consumer = c
	return s
}

func (s *ResultCollectorService) Start(ctx context.Context) error {
	return s.consumer.Start(ctx)
}

func (s *ResultCollectorService) handleResult(ctx context.Context, msg *sarama.ConsumerMessage) error {
	var pbs pbjudgeresult.JudgeResult
	err := proto.Unmarshal(msg.Value, &pbs)
	if err != nil {
		return fmt.Errorf("failed to unmarshal judge result: %w", err)
	}

	updates := map[string]any{
		"result":      int8(pbs.Result),
		"status":      2, // 防止重复判题
		"time_used":   int(pbs.TimeUsed),
		"memory_used": int(pbs.MemoryUsed),
	}
	if pbs.Stderr != nil {
		updates["stderr"] = *pbs.Stderr
	}

	collectorCtx := loggerv2.ContextWithFields(ctx, logger.Uint64("submission_id", pbs.SubmissionId))

	err = retry.Do(collectorCtx, func() error {
		errInternal := s.db.WithContext(collectorCtx).
			Model(&ojmodel.Submission{}).
			Where("id = ?", pbs.SubmissionId).
			Updates(updates).Error
		if errInternal != nil {
			return fmt.Errorf("failed to update submission: %w", errInternal)
		}
		return nil
	}, retry.WithBaseInterval(time.Second))
	if err != nil {
		return fmt.Errorf("failed to update submission: %w", err)
	}

	var submission ojmodel.Submission
	err = retry.Do(collectorCtx, func() error {
		errInternal := s.db.WithContext(collectorCtx).
			Where("id = ?", pbs.SubmissionId).
			Select("competition_id", "problem_id", "user_id", "created_at").
			First(&submission).Error
		if errInternal != nil {
			return fmt.Errorf("failed to get submission: %w", errInternal)
		}
		return nil
	}, retry.WithBaseInterval(time.Second))
	if err != nil {
		return fmt.Errorf("failed to get submission: %w", err)
	}

	err = retry.Do(collectorCtx, func() error {
		errInternal := s.rankingSvc.UpdateUserScore(collectorCtx, submission.CompetitionID, submission.ProblemID, submission.UserID, ojmodel.SubmissionResult(pbs.Result) == ojmodel.SubmissionResultAccepted, submission.CreatedAt)
		if errInternal != nil {
			return fmt.Errorf("failed to update user score: %w", errInternal)
		}
		return nil
	}, retry.WithBaseInterval(time.Second))
	if err != nil {
		return fmt.Errorf("failed to update user score: %w", err)
	}

	return nil
}
