package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/robfig/cron/v3"
	"log"
	"net/http"
	"porker/game"
	"porker/redis"
	"sync"
	"time"
)

type OnlineWebSocket struct {
}

// 定义一个 upgrader，它将 HTTP 连接升级为 WebSocket 连接
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// 允许所有来源的连接
		return true
	},
}

var Clients = make(map[string]*websocket.Conn) // 连接列表 键是sessionId值是连接对象
var ClientsMu sync.RWMutex                     // 连接客户端列表的互斥锁

var UserRoomMap = make(map[string]string) //用户和房间号的映射
var UserRoomMapMu sync.RWMutex

var RoomMap = make(map[string]map[string]interface{}) //房间map，键是房间号，值是切片类型的用户sessionId
var RoomMapMu sync.RWMutex

func (this *OnlineWebSocket) HandleWebSocket(c *gin.Context) {

	// 升级 HTTP 连接为 WebSocket 连接
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Println("WebSocket连接失败:", err)
		return
	}
	session := sessions.Default(c)
	sessionId := session.Get("session_id")
	//加入连接列表
	this.onConnect(conn, sessionId.(string))

	// 启动一个 goroutine 定期向客户端发送消息
	go func() {
		for {
			pingMap := map[string]string{}
			pingMap["ping"] = "pong"
			marshal, _ := json.Marshal(pingMap)
			err := conn.WriteMessage(websocket.TextMessage, marshal)
			if err != nil {
				log.Println(conn.RemoteAddr(), "心跳发送失败:", err)
				break
			}
			time.Sleep(40 * time.Second) //40秒发送一次心跳包，如果设置60s秒以上须在nginx设置超时时间
		}
	}()
	for {
		_, mbytes, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				// 连接正常关闭或正在关闭
				this.onDisconnect(conn, sessionId.(string))
			} else {
				log.Println("websocket连接异常关闭", err)
			}
			break
		}
		this.onMessage(mbytes, sessionId.(string))

	}

}

// 收到消息后
func (this *OnlineWebSocket) onMessage(message []byte, sessionId string) {
	JsonMessage := map[string]interface{}{}
	err := json.Unmarshal(message, &JsonMessage)
	if err != nil {
		fmt.Println(err)
	}
	//fmt.Println(JsonMessage)
	msgType := JsonMessage["type"].(string)
	ctx := context.Background()
	RoomMapMu.RLock()
	corn := RoomMap[JsonMessage["room"].(string)]["cron"].(*cron.Cron)
	RoomMapMu.RUnlock()
	switch msgType {

	case "now_sent_poker":
		var entryID cron.EntryID
		entryID, err = corn.AddFunc("@every 15s", func() {

			defer func() {
				defer corn.Remove(entryID) //必须自行销毁
				redis.RClient.HSet(ctx, JsonMessage["room"].(string), "timeId", 0)
				fmt.Println("定时器自动销毁...")
			}()
			prevPokerBytes, err := redis.RClient.HGet(ctx, JsonMessage["room"].(string), "prevPoker").Bytes()
			if err != nil {
				fmt.Println(err)
			}
			roundRecording, err := redis.RClient.HGet(ctx, JsonMessage["room"].(string), "roundRecording").Int()
			if err != nil {
				fmt.Println(err)
			}
			prevPoker := map[string]int{}
			json.Unmarshal(prevPokerBytes, &prevPoker)
			if len(prevPoker) == 0 || prevPoker == nil {
				return
			}

			if roundRecording == game.GameNumber-2 {
				victoryId, err := redis.RClient.HGet(ctx, JsonMessage["room"].(string), "prevGamerId").Result() //取出上回合胜利玩家的会话ID
				if err != nil {
					fmt.Println(err)
				}
				redis.RClient.HSet(ctx, JsonMessage["room"].(string), "roundRecording", 0) //初始化回合记录
				pokerBytes, err := redis.RClient.HGet(ctx, JsonMessage["room"].(string), "poker").Bytes()
				if err != nil {
					fmt.Println(err)
				}
				poker := []string{}
				json.Unmarshal(pokerBytes, &poker)
				victoryPokerBytes, err := redis.RClient.HGet(ctx, JsonMessage["room"].(string), victoryId).Bytes() //上回合胜利玩家的手牌
				if err != nil {
					fmt.Println(err)
				}

				victoryPoker := map[string]int{}
				json.Unmarshal(victoryPokerBytes, &victoryPoker)

				if len(poker) != 0 {
					pokerKey := poker[len(poker)-1] //获取剩余扑克的最后一张的花色（键值）
					pokerValue := game.Poker[pokerKey]
					poker = poker[:len(poker)-1] //删除最后一张牌
					pokerMarshal, err := json.Marshal(poker)
					if err != nil {
						fmt.Println(err)
					}
					redis.RClient.HSet(ctx, JsonMessage["room"].(string), "poker", pokerMarshal)
					//将新牌发给上回合胜利的玩家
					victoryPoker[pokerKey] = pokerValue
					victoryPokerMarshal, err := json.Marshal(victoryPoker)
					if err != nil {
						fmt.Println(err)
					}
					redis.RClient.HSet(ctx, JsonMessage["room"].(string), victoryId, victoryPokerMarshal)

					this.SessionIdSendMessage(victoryId, gin.H{
						"type": "send_poker",
						"poker": map[string]int{
							pokerKey: pokerValue,
						},
					})
				}

				//将上回合出牌内容清空
				redis.RClient.HSet(ctx, JsonMessage["room"].(string), "prevPoker", "")

				//回合胜利者出牌
				this.SessionIdSendMessage(victoryId, gin.H{
					"type": "now_sent_poker",
				})

				this.SessionIdSendMessage(sessionId, gin.H{
					"type": "skip",
				})

				//告诉房间玩家该谁出牌
				this.RoomSendMessage(JsonMessage["room"].(string), gin.H{
					"type":         "now_gamer",
					"now_gamer_id": victoryId,
				})

			} else {
				redis.RClient.HSet(ctx, JsonMessage["room"].(string), "roundRecording", roundRecording+1) //初始化回合记录
				RoomMapMu.RLock()
				sessionIdList := RoomMap[JsonMessage["room"].(string)]["sessionIdList"].([]string)
				RoomMapMu.RUnlock()
				nextUserId := game.GetNextUser(sessionIdList, sessionId, prevPoker)

				//将上回合出牌内容清空
				redis.RClient.HSet(ctx, JsonMessage["room"].(string), "prevPoker", "")

				timeId, err := redis.RClient.HGet(ctx, JsonMessage["room"].(string), "timeId").Int()
				if err != nil {
					fmt.Println(err)
				}

				if timeId != 0 {
					corn.Remove(cron.EntryID(timeId))
					//清空redis中的定时器id
					redis.RClient.HSet(ctx, JsonMessage["room"].(string), "timeId", 0)
					fmt.Println("定时器销毁...")
				}

				this.SessionIdSendMessage(nextUserId, gin.H{
					"type": "now_sent_poker",
					"time": game.NextTime,
				})

				this.SessionIdSendMessage(sessionId, gin.H{
					"type": "skip",
				})

				this.RoomSendMessage(JsonMessage["room"].(string), gin.H{
					"type":         "now_gamer",
					"now_gamer_id": nextUserId,
				})
			}

		})
		corn.Start()
		if err != nil {
			fmt.Println(err)
		}
		//fmt.Println("定时器存储进redis")
		//fmt.Println(entryID)
		redis.RClient.HSet(ctx, JsonMessage["room"].(string), "timeId", int(entryID))
		break
		//case "action":
		//
		//	timeId, err := redis.RClient.HGet(ctx, JsonMessage["room"].(string), "timeId").Int()
		//	if err != nil {
		//		fmt.Println(err)
		//	}
		//
		//	if timeId != 0 {
		//		corn.Remove(cron.EntryID(timeId))
		//		//清空redis中的定时器id
		//		redis.RClient.HSet(ctx, JsonMessage["room"].(string), "timeId", 0)
		//		fmt.Println("定时器销毁...")
		//	}
		//	break

	}

}

// 连接事件处理函数
func (this *OnlineWebSocket) onConnect(conn *websocket.Conn, sessionId string) {

	ClientsMu.Lock()
	if _, ok := Clients[sessionId]; !ok {
		Clients[sessionId] = conn
	} else {
		conn.WriteJSON(gin.H{
			"text": "您正在别处游戏",
		})
		conn.Close()
	}
	ClientsMu.Unlock()

}

// 断开连接事件处理函数
func (this *OnlineWebSocket) onDisconnect(conn *websocket.Conn, sessionId string) {
	ClientsMu.Lock()
	delete(Clients, sessionId) //删除连接
	ClientsMu.Unlock()
	conn.Close() //断开连接

	//用户断开连接后在用户会话与房间映射表中查询当前用户会话是否存在，存在就从房间中移除
	UserRoomMapMu.RLock()
	room, ok := UserRoomMap[sessionId]
	if ok {
		RoomMapMu.Lock()
		m := RoomMap[room]
		if m["userList"] != nil {
			if _, userOk := m["userList"].(map[string]string)[sessionId]; userOk {
				delete(m["userList"].(map[string]string), sessionId) //从房间中删除已经离开的玩家
				//检测房间还有没有人
				if len(m["userList"].(map[string]string)) == 0 {
					delete(RoomMap, room) //直接删除房间
					RoomMapMu.Unlock()    //前面上了锁，无论如何也得解锁
					return
				}
				//如果游戏已经开始了
				if m["states"] == true {
					RoomMapMu.Unlock() //先解锁，防止在房间内发送消息失败成死锁
					this.RoomSendMessage(room, gin.H{
						"type": "disband",
					})
					delete(RoomMap, room)                         //直接删除房间
					redis.RClient.Del(context.Background(), room) //删除redis对局信息
				} else {
					RoomMapMu.Unlock() //前面上了锁，无论如何也得解锁
				}
			}
		} else {
			RoomMapMu.Unlock() //前面上了锁，无论如何也得解锁
		}

	}

}

/*
 * @Description: 广播消息
 * @receiver this
 * @param message
 */
func (this *OnlineWebSocket) broadcastMessage(message []byte) {
	ClientsMu.RLock()
	defer ClientsMu.RUnlock()
	for sessionId, client := range Clients {
		err := client.WriteMessage(websocket.TextMessage, message)
		if err != nil {
			log.Println("当前连接广播发送消息失败:", err)
			client.Close()
			delete(Clients, sessionId)
		}
	}
}

/*
 * @Description: 在房间内发送消息
 * @receiver this
 * @param room
 * @param message
 */
func (this *OnlineWebSocket) RoomSendMessage(room string, message gin.H) {
	RoomMapMu.RLock()
	defer RoomMapMu.RUnlock()
	ClientsMu.RLock()
	for k, _ := range RoomMap[room]["userList"].(map[string]string) {
		Clients[k].WriteJSON(message)
	}
	ClientsMu.RUnlock()
}

/*
 * @Description: 通过会话id给某一个用户发消息
 * @receiver this
 * @param sessionId
 * @param message
 */
func (this *OnlineWebSocket) SessionIdSendMessage(sessionId string, message gin.H) {
	ClientsMu.RLock()
	defer ClientsMu.RUnlock()
	client, ok := Clients[sessionId]
	if ok {
		client.WriteJSON(message)
	}
}
