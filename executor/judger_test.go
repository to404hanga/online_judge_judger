package executor

import (
	"context"
	"testing"

	ojmodel "github.com/to404hanga/online_judge_common/model"
	"github.com/to404hanga/online_judge_judger/executor/service"
	"github.com/to404hanga/pkg404/logger"
	loggerv2 "github.com/to404hanga/pkg404/logger/v2"
)

func TestRun(t *testing.T) {
	l := loggerv2.GetGlobalLogger()
	ctx := context.Background()
	judger := NewDockerJudger(l, 2, 10, 128, ".")
	defer judger.Close()

	task := &service.JudgeTask{
		ProblemID:   1,
		Language:    ojmodel.SubmissionLanguageCPP,
		TimeLimit:   200,
		MemoryLimit: 128,
		SourceCode: `#include <iostream>
using namespace std;
int main() {
	int n;
	cin >> n;
	cout << n << endl;
}`,
	}
	finalResult, err := judger.Run(ctx, task)
	if err != nil {
		l.ErrorContext(ctx, "failed to run", logger.Error(err))
	}
	l.InfoContext(ctx, "run result",
		logger.Any("result", finalResult),
	)
}

// const (
// 	SubmissionResultUnjudged            SubmissionResult = iota // 未判题 0
// 	SubmissionResultAccepted                                    // Accepted 1
// 	SubmissionResultWrongAnswer                                 // Wrong Answer 2
// 	SubmissionResultCompileError                                // Compile Error 3
// 	SubmissionResultRuntimeError                                // Runtime Error 4
// 	SubmissionResultTimeLimitExceeded                           // Time Limit Exceeded 5
// 	SubmissionResultMemoryLimitExceeded                         // Memory Limit Exceeded 6
// 	SubmissionResultOutputLimitExceeded                         // Output Limit Exceeded 7
// )
