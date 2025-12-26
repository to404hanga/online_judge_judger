package service

import (
	"context"
	"fmt"
	"time"

	"github.com/IBM/sarama"
	"github.com/prometheus/client_golang/prometheus"
	ojmodel "github.com/to404hanga/online_judge_common/model"
	ojconstants "github.com/to404hanga/online_judge_common/proto/constants"
	pbjudgeresult "github.com/to404hanga/online_judge_common/proto/gen/judgeresult"
	"github.com/to404hanga/online_judge_controller/service"
	ojcservice "github.com/to404hanga/online_judge_controller/service"
	"github.com/to404hanga/online_judge_judger/consumer"
	"github.com/to404hanga/pkg404/gotools/retry"
	"github.com/to404hanga/pkg404/logger"
	loggerv2 "github.com/to404hanga/pkg404/logger/v2"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

const (
	ResultCollectorGroupID = "result_collector_group"
)

var (
	resultCollectorHandleInFlight = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "online_judge",
		Subsystem: "resultcollector",
		Name:      "handle_result_in_flight",
		Help:      "Current number of in-flight handleResult operations.",
	})

	resultCollectorHandleTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "online_judge",
		Subsystem: "resultcollector",
		Name:      "handle_result_total",
		Help:      "Total number of handleResult operations.",
	}, []string{"result", "reason"})

	resultCollectorHandleDurationSeconds = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "online_judge",
		Subsystem: "resultcollector",
		Name:      "handle_result_duration_seconds",
		Help:      "Duration of handleResult operations in seconds.",
		Buckets:   prometheus.ExponentialBuckets(0.005, 2, 16),
	}, []string{"result"})
)

func init() {
	prometheus.MustRegister(
		resultCollectorHandleInFlight,
		resultCollectorHandleTotal,
		resultCollectorHandleDurationSeconds,
	)
}

type ResultCollectorService struct {
	log        loggerv2.Logger
	db         *gorm.DB
	consumer   consumer.Consumer
	rankingSvc ojcservice.RankingService
}

func NewResultCollectorService(log loggerv2.Logger, cg sarama.ConsumerGroup, db *gorm.DB, rankingSvc ojcservice.RankingService) *ResultCollectorService {
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

func (s *ResultCollectorService) handleResult(ctx context.Context, msg *sarama.ConsumerMessage) (err error) {
	opStartTime := time.Now()
	result := "success"
	reason := "ok"

	resultCollectorHandleInFlight.Inc()
	defer func() {
		resultCollectorHandleInFlight.Dec()
		resultCollectorHandleTotal.WithLabelValues(result, reason).Inc()
		resultCollectorHandleDurationSeconds.WithLabelValues(result).Observe(time.Since(opStartTime).Seconds())
	}()

	var pbs pbjudgeresult.JudgeResult
	err = proto.Unmarshal(msg.Value, &pbs)
	if err != nil {
		result = "error"
		reason = "unmarshal_judge_result"
		s.log.ErrorContext(ctx, "failed to unmarshal judge result", logger.Error(err))
		return fmt.Errorf("failed to unmarshal judge result: %w", err)
	}
	ctx = loggerv2.ContextWithFields(ctx, logger.String("RequestID", pbs.RequestId))

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
			s.log.ErrorContext(collectorCtx, "failed to update submission", logger.Error(errInternal))
			return fmt.Errorf("failed to update submission: %w", errInternal)
		}
		return nil
	}, retry.WithBaseInterval(time.Second))
	if err != nil {
		result = "error"
		reason = "db_update_submission"
		s.log.ErrorContext(ctx, "failed to update submission", logger.Error(err))
		return fmt.Errorf("failed to update submission: %w", err)
	}

	var submission ojmodel.Submission
	err = retry.Do(collectorCtx, func() error {
		errInternal := s.db.WithContext(collectorCtx).
			Where("id = ?", pbs.SubmissionId).
			Select("competition_id", "problem_id", "user_id", "created_at").
			First(&submission).Error
		if errInternal != nil {
			s.log.ErrorContext(collectorCtx, "failed to get submission", logger.Error(errInternal))
			return fmt.Errorf("failed to get submission: %w", errInternal)
		}
		return nil
	}, retry.WithBaseInterval(time.Second))
	if err != nil {
		result = "error"
		reason = "db_get_submission"
		s.log.ErrorContext(ctx, "failed to get submission", logger.Error(err))
		return fmt.Errorf("failed to get submission: %w", err)
	}

	var startTime time.Time
	err = retry.Do(collectorCtx, func() error {
		t, errInternal := s.updateCompetitionUser(collectorCtx, submission.CompetitionID, submission.UserID, ojmodel.SubmissionResult(pbs.Result) == ojmodel.SubmissionResultAccepted, submission.CreatedAt)
		if errInternal != nil {
			s.log.ErrorContext(collectorCtx, "failed to update competition user", logger.Error(errInternal))
			return fmt.Errorf("failed to update competition user: %w", errInternal)
		}
		startTime = t
		return nil
	}, retry.WithBaseInterval(time.Second))
	if err != nil {
		result = "error"
		reason = "update_competition_user"
		s.log.ErrorContext(ctx, "failed to update competition user", logger.Error(err))
		return fmt.Errorf("failed to update competition user: %w", err)
	}

	err = retry.Do(collectorCtx, func() error {
		errInternal := s.rankingSvc.UpdateUserScore(
			collectorCtx,
			submission.CompetitionID,
			submission.ProblemID,
			submission.UserID,
			ojmodel.SubmissionResult(pbs.Result) == ojmodel.SubmissionResultAccepted,
			submission.CreatedAt,
			startTime,
		)
		if errInternal != nil {
			s.log.ErrorContext(collectorCtx, "ffailed to update user score", logger.Error(errInternal))
			return fmt.Errorf("failed to update user score: %w", errInternal)
		}
		return nil
	}, retry.WithBaseInterval(time.Second))
	if err != nil {
		result = "error"
		reason = "update_user_score"
		s.log.ErrorContext(ctx, "failed to update user score", logger.Error(err))
		return fmt.Errorf("failed to update user score: %w", err)
	}

	return nil
}

func (s *ResultCollectorService) updateCompetitionUser(ctx context.Context, competitionID, userID uint64, isAccepted bool, acceptedTime time.Time) (startTime time.Time, err error) {
	tx := s.db.WithContext(ctx).Begin()
	defer func() {
		if err != nil {
			tx.Rollback()
		}
		err = tx.Commit().Error
		if err != nil {
			err = fmt.Errorf("failed to commit transaction: %w", err)
			tx.Rollback()
		}
	}()

	var cu ojmodel.CompetitionUser
	err = tx.Model(&ojmodel.CompetitionUser{}).
		Where("competition_id = ?", competitionID).
		Where("user_id = ?", userID).
		First(&cu).Error
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to pluck pass count: %w", err)
	}

	updates := map[string]any{}
	if isAccepted {
		updates["pass_count"] = cu.PassCount + 1
		updates["total_time"] = acceptedTime.UnixMilli() + int64(cu.RetryCount)*service.PenaltyTime - cu.StartTime.UnixMilli()
	} else {
		updates["retry_count"] = cu.RetryCount + 1
	}

	err = tx.Model(&ojmodel.CompetitionUser{}).
		Where("competition_id = ?", competitionID).
		Where("user_id = ?", userID).
		Updates(updates).Error
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to update competition user: %w", err)
	}

	return cu.StartTime, nil
}
