package ioc

import (
	"fmt"
	"log"
	"time"

	"github.com/spf13/viper"
	"github.com/to404hanga/online_judge_judger/config"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func InitDB() *gorm.DB {
	var cfg config.DBConfig
	err := viper.UnmarshalKey(cfg.Key(), &cfg)
	if err != nil {
		log.Panicf("unmarshal db config fail, err: %v", err)
	}

	db, err := gorm.Open(mysql.Open(fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Asia%%2FShanghai",
		cfg.Username, cfg.Password, cfg.Host, cfg.Port, cfg.DBName,
	)), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			TablePrefix:   cfg.TablePrefix,
			SingularTable: true,
		},
	})
	if err != nil {
		log.Panicf("init db fail, err: %v", err)
	}

	// 配置连接池
	sqlDB, err := db.DB()
	if err != nil {
		log.Panicf("get sql.DB fail, err: %v", err)
	}

	// 设置连接池参数
	if cfg.MaxOpenConns > 0 {
		sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	} else {
		sqlDB.SetMaxOpenConns(100) // 默认最大连接数
	}

	if cfg.MaxIdleConns > 0 {
		sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	} else {
		sqlDB.SetMaxIdleConns(10) // 默认最大空闲连接数
	}

	if cfg.ConnMaxLifetime > 0 {
		sqlDB.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetime) * time.Minute)
	} else {
		sqlDB.SetConnMaxLifetime(time.Hour) // 默认连接最大生存时间1小时
	}

	if cfg.ConnMaxIdleTime > 0 {
		sqlDB.SetConnMaxIdleTime(time.Duration(cfg.ConnMaxIdleTime) * time.Minute)
	} else {
		sqlDB.SetConnMaxIdleTime(10 * time.Minute) // 默认连接最大空闲时间10分钟
	}

	return db
}
