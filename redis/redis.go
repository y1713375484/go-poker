package redis

import (
	"github.com/go-redis/redis/v8"
	"os"
)

type RedisDao struct {
}

var RClient *redis.Client

func InitRedis() {
	RClient = redis.NewClient(&redis.Options{
		Addr:     "127.0.0.1:" + os.Getenv("RPORT"), // Redis服务器地址
		Password: os.Getenv("RPASS"),                // 密码，如果没有设置密码则为空
		DB:       0,                                 // 默认数据库
	})
}

func CloseRedis() {
	err := RClient.Close()
	if err != nil {
		panic(err)
	}
}
