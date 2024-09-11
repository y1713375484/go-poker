package controllers

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"os"
)

type IndexController struct{}

func (this *IndexController) Index(c *gin.Context) {
	room := c.Query("room")
	websocketUrl := os.Getenv("WEBSOCKET")
	c.HTML(http.StatusOK, "index.html", gin.H{
		"room": room,
		"url":  websocketUrl,
	})
}
