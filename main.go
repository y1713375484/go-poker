package main

import (
	"github.com/joho/godotenv"
	"log"
	"os"
	"porker/game"
	"porker/redis"
	"porker/router"
	"strconv"
)

func main() {
	err := godotenv.Load()
	game.GameNumber, _ = strconv.Atoi(os.Getenv("GAMENUM"))
	game.NextTime, _ = strconv.Atoi(os.Getenv("NEXTTIME"))
	if err != nil {
		log.Fatal("Error loading .env file")
	}
	r := router.InitRouter()
	redis.InitRedis()
	defer redis.CloseRedis()
	r.Run(":" + os.Getenv("PORT")) // 监听并在 0.0.0.0:8080 上启动服务
}
