package service

import (
	"context"
	"fmt"
	"time"

	"github.com/IBM/sarama"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
	ojmodel "github.com/to404hanga/online_judge_common/model"
	ojconstants "github.com/to404hanga/online_judge_common/proto/constants"
	"github.com/to404hanga/online_judge_common/proto/gen/judgetask"
	pbsubmission "github.com/to404hanga/online_judge_common/proto/gen/submission"
	"github.com/to404hanga/online_judge_judger/constants"
	"github.com/to404hanga/online_judge_judger/consumer"
	"github.com/to404hanga/pkg404/cachex/lru"
	"github.com/to404hanga/pkg404/logger"
	loggerv2 "github.com/to404hanga/pkg404/logger/v2"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

const (
	problemKey                    = "problem:%d"
	JudgerMasterSubmissionGroupID = "judger_master_submission_group"
)

var (
	submissionHandleInFlight = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "online_judge_judger",
		Subsystem: "master",
		Name:      "submission_handle_in_flight",
		Help:      "Current number of in-flight submission handle operations.",
	})

	submissionHandleTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "online_judge_judger",
		Subsystem: "master",
		Name:      "submission_handle_total",
		Help:      "Total number of submission handle operations.",
	}, []string{"result", "reason"})

	submissionHandleDurationSeconds = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "online_judge_judger",
		Subsystem: "master",
		Name:      "submission_handle_duration_seconds",
		Help:      "Duration of submission handle operations in seconds.",
		Buckets:   prometheus.ExponentialBuckets(0.005, 2, 16),
	}, []string{"result", "problem_cache"})

	submissionProblemCacheTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "online_judge_judger",
		Subsystem: "master",
		Name:      "problem_cache_total",
		Help:      "Total number of problem cache lookups.",
	}, []string{"result"})
)

func init() {
	prometheus.MustRegister(
		submissionHandleInFlight,
		submissionHandleTotal,
		submissionHandleDurationSeconds,
		submissionProblemCacheTotal,
	)
}

type SubmissionService struct {
	log      loggerv2.Logger
	consumer consumer.Consumer
	rdb      redis.Cmdable
	db       *gorm.DB
	lru      *lru.Cache
}

var (
	_ consumer.Consumer = (*SubmissionService)(nil)
)

func NewSubmissionService(log loggerv2.Logger, cg sarama.ConsumerGroup, rdb redis.Cmdable, db *gorm.DB, lru *lru.Cache) *SubmissionService {
	s := &SubmissionService{
		log: log,
		rdb: rdb,
		lru: lru,
		db:  db,
	}
	handler := consumer.NewGroupHandler(s.handleSubmission, log)
	c := consumer.NewSaramaConsumer(cg, ojconstants.SubmissionTopic, handler, log)
	s.consumer = c
	return s
}

func (s *SubmissionService) Start(ctx context.Context) error {
	return s.consumer.Start(ctx)
}

func (s *SubmissionService) handleSubmission(ctx context.Context, msg *sarama.ConsumerMessage) (err error) {
	startTime := time.Now()
	result := "success"
	reason := "ok"
	problemCache := "na"

	submissionHandleInFlight.Inc()
	defer func() {
		submissionHandleInFlight.Dec()
		submissionHandleTotal.WithLabelValues(result, reason).Inc()
		submissionHandleDurationSeconds.WithLabelValues(result, problemCache).Observe(time.Since(startTime).Seconds())
	}()

	var pbs pbsubmission.Submission
	err = proto.Unmarshal(msg.Value, &pbs)
	if err != nil {
		result = "error"
		reason = "unmarshal_submission"
		s.log.ErrorContext(ctx, "failed to unmarshal submission", logger.Error(err))
		return fmt.Errorf("failed to unmarshal submission: %w", err)
	}
	ctx = loggerv2.ContextWithFields(ctx, logger.String("RequestID", pbs.RequestId))

	var submission ojmodel.Submission
	err = s.db.WithContext(ctx).Model(&ojmodel.Submission{}).
		Where("id = ?", pbs.SubmissionId).
		Select("problem_id", "code", "language").
		First(&submission).Error
	if err != nil {
		result = "error"
		reason = "db_get_submission"
		s.log.ErrorContext(ctx, "failed to get submission", logger.Error(err))
		return fmt.Errorf("failed to get submission: %w", err)
	}

	var problem ojmodel.Problem
	lruKey := fmt.Sprintf(problemKey, submission.ProblemID)
	if problemAny, ok := s.lru.Get(lruKey); ok {
		problemCache = "hit"
		submissionProblemCacheTotal.WithLabelValues("hit").Inc()
		problem = problemAny.(ojmodel.Problem)
	} else {
		problemCache = "miss"
		submissionProblemCacheTotal.WithLabelValues("miss").Inc()
		err = s.db.WithContext(ctx).Model(&ojmodel.Problem{}).
			Where("id = ?", submission.ProblemID).
			Select("time_limit", "memory_limit").
			First(&problem).Error
		if err != nil {
			result = "error"
			reason = "db_get_problem"
			s.log.ErrorContext(ctx, "failed to get problem", logger.Error(err))
			return fmt.Errorf("failed to get problem: %w", err)
		}
		s.lru.Add(lruKey, problem)
	}

	task := &judgetask.JudgeTask{
		SubmissionId: pbs.SubmissionId,
		ProblemId:    submission.ProblemID,
		Code:         submission.Code,
		Language:     int32(submission.Language.Int8()),
		TimeLimit:    int32(problem.TimeLimit),
		MemoryLimit:  int32(problem.MemoryLimit),
		RequestId:    pbs.RequestId,
	}
	taskBytes, err := proto.Marshal(task)
	if err != nil {
		result = "error"
		reason = "marshal_judge_task"
		s.log.ErrorContext(ctx, "failed to marshal judge task", logger.Error(err))
		return fmt.Errorf("failed to marshal judge task: %w", err)
	}

	err = s.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: constants.JudgeTaskKey,
		Values: map[string]any{
			"task": taskBytes,
		},
	}).Err()
	if err != nil {
		result = "error"
		reason = "redis_xadd"
		s.log.ErrorContext(ctx, "failed to add judge task to stream", logger.Error(err))
		return fmt.Errorf("failed to add judge task to stream: %w", err)
	}
	return nil
}
