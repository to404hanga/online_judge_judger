package config

type JudgerWorkerConfig struct {
	ContainerPoolSize        int    `yaml:"containerPoolSize"`
	CompileTimeoutSeconds    int    `yaml:"compileTimeoutSeconds"`
	DefaultMemoryLimitMB     int    `yaml:"defaultMemoryLimitMB"`
	XAutoClaimTimeoutMinutes int    `yaml:"xAutoClaimTimeoutMinutes"`
	TestcasePathPrefix       string `yaml:"testcasePathPrefix"`
}

func (JudgerWorkerConfig) Key() string {
	return "judgerWorker"
}
