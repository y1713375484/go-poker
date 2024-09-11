package router

import (
	"fmt"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"html/template"
	"net/http"
	"porker/controllers"
	"porker/public"
	"porker/view"
	"porker/websocket"
	"runtime/debug"
)

func InitRouter() *gin.Engine {
	router := gin.Default()

	//模版文件加载
	Views, err := template.ParseFS(view.Views, "**/*")
	if err != nil {
		fmt.Println(err)
	}

	router.SetHTMLTemplate(Views)

	store := cookie.NewStore([]byte("secret"))

	//捕获全局painc
	router.Use(CustomRecovery())

	// 使用会话中间件
	router.Use(sessions.Sessions("porkerSession", store))

	// 使用自定义的自动生成会话 ID 的中间件
	router.Use(SessionMiddleware())

	//静态资源加载
	router.StaticFS("/public", http.FS(public.PublicFile))
	indexController := &controllers.IndexController{}
	router.GET("/", indexController.Index)

	onlineWebSocket := &websocket.OnlineWebSocket{}
	router.GET("/ws", onlineWebSocket.HandleWebSocket)

	apiController := &controllers.ApiController{}
	apiControllerGroup := router.Group("/api")
	{
		apiControllerGroup.GET("/test", apiController.Test)
		apiControllerGroup.POST("/joinRoom", apiController.JoinRoom)
		apiControllerGroup.POST("/sentPoker", apiController.SentPorker)
		apiControllerGroup.POST("/notPoker", apiController.NotPoker)
	}

	return router
}

// 自动设置session_id
func SessionMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		if session.Get("session_id") == nil {
			sessionID := uuid.New().String()
			session.Set("session_id", sessionID)
			session.Save()
		}
		c.Next()
	}
}

func CustomRecovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				// 打印 panic 信息和堆栈跟踪
				fmt.Println(r)
				debug.PrintStack()

				// 返回一个自定义的错误响应
				c.JSON(http.StatusInternalServerError, gin.H{
					"error": "Internal Server Error",
				})
			}
		}()
		c.Next()
	}
}
