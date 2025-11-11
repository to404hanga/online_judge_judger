package config

import (
	loggerv2 "github.com/to404hanga/pkg404/logger/v2"
)

type LoggerConfig struct {
	Development    bool                `yaml:"development"`    // 是否为开发模式
	Type           loggerv2.OutputType `yaml:"type"`           // 日志输出类型
	LogFilePath    string              `yaml:"logFilePath"`    // 日志文件路径
	AutoCreateFile bool                `yaml:"autoCreateFile"` // 是否自动创建文件和目录
}

func (LoggerConfig) Key() string {
	return "log"
}

type DBConfig struct {
	Host        string `yaml:"host"`
	Port        int    `yaml:"port"`
	Username    string `yaml:"username"`
	Password    string `yaml:"password"`
	DBName      string `yaml:"database"`
	TablePrefix string `yaml:"tablePrefix"`
	// 连接池配置
	MaxOpenConns    int `yaml:"maxOpenConns"`    // 最大打开连接数
	MaxIdleConns    int `yaml:"maxIdleConns"`    // 最大空闲连接数
	ConnMaxLifetime int `yaml:"connMaxLifetime"` // 连接最大生存时间（分钟）
	ConnMaxIdleTime int `yaml:"connMaxIdleTime"` // 连接最大空闲时间（分钟）
}

func (DBConfig) Key() string {
	return "db"
}

type RedisConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	DB       int    `yaml:"db"`
	Password string `yaml:"password"`
}

func (RedisConfig) Key() string {
	return "redis"
}

type KafkaConfig struct {
	Brokers []string `yaml:"brokers"`
}

func (KafkaConfig) Key() string {
	return "kafka"
}
