package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/robfig/cron/v3"
	"porker/game"
	"porker/redis"
	"porker/websocket"
)

type ApiController struct{}

/*
 * @Description: 加入房间操作
 * @receiver this
 * @param c
 */
func (this *ApiController) JoinRoom(c *gin.Context) {
	session := sessions.Default(c)
	sessionId := session.Get("session_id").(string)
	type ReqJson struct {
		Room string `json:"room"`
		Name string `json:"name"`
	}
	reqJson := ReqJson{}
	err := c.BindJSON(&reqJson)
	if err != nil {
		fmt.Println(err)
	}
	onlineWebSocket := &websocket.OnlineWebSocket{}

	if Room, ok := websocket.RoomMap[reqJson.Room]; ok {

		//房间存在
		if Room["states"].(bool) != true {
			//未开始游戏
			websocket.RoomMapMu.Lock()
			websocket.RoomMap[reqJson.Room]["userList"].(map[string]string)[sessionId] = reqJson.Name
			websocket.RoomMap[reqJson.Room]["sessionIdList"] = append(websocket.RoomMap[reqJson.Room]["sessionIdList"].([]string), sessionId)
			roomUserNum := len(websocket.RoomMap[reqJson.Room]["userList"].(map[string]string))
			websocket.RoomMapMu.Unlock()

			//添加用户和房间映射
			websocket.UserRoomMapMu.RLock()
			websocket.UserRoomMap[sessionId] = reqJson.Room
			websocket.UserRoomMapMu.RUnlock()

			//检测房间人数是否可以开时游戏
			if roomUserNum < game.GameNumber {
				//人数不足
				//websocket.Clients[sessionId].WriteJSON(gin.H{
				//	"type": "state",
				//	"code": 2,
				//})
				onlineWebSocket.SessionIdSendMessage(sessionId, gin.H{
					"type": "state",
					"code": 2,
				})

			} else {
				//游戏可以开始了....
				roomUserSessionIdArray := websocket.RoomMap[reqJson.Room]["sessionIdList"].([]string)
				websocket.RoomMapMu.RLock()
				websocket.RoomMap[reqJson.Room]["states"] = true
				//创建每一个房间的定时器对象
				websocket.RoomMap[reqJson.Room]["cron"] = cron.New()
				websocket.RoomMapMu.RUnlock()
				//game := game.Game{}
				//初始化扑克牌
				game.InitGame(roomUserSessionIdArray, reqJson.Room)
				gameNames := []string{}
				for _, v := range websocket.RoomMap[reqJson.Room]["userList"].(map[string]string) {
					gameNames = append(gameNames, v)
				}
				//给房间内玩家发送本房间玩家姓名
				onlineWebSocket.RoomSendMessage(reqJson.Room, gin.H{
					"type":  "gamerName",
					"names": gameNames,
				})
				for i := 0; i < len(roomUserSessionIdArray); i++ {
					bytes, err := redis.RClient.HGet(context.Background(), reqJson.Room, roomUserSessionIdArray[i]).Bytes()
					if err != nil {
						fmt.Println(err)
					}
					poker := map[string]int{}
					json.Unmarshal(bytes, &poker)

					//给所有用户发送扑克
					onlineWebSocket.SessionIdSendMessage(roomUserSessionIdArray[i], gin.H{
						"type":  "send_poker",
						"poker": poker,
					})

				}
				//给第一个用户发送开始出牌指令
				onlineWebSocket.SessionIdSendMessage(roomUserSessionIdArray[0], gin.H{
					"type": "now_sent_poker",
				})

				onlineWebSocket.RoomSendMessage(reqJson.Room, gin.H{
					"type":         "now_gamer",
					"now_gamer_id": roomUserSessionIdArray[0],
				})

			}

		} else {
			//房间状态已经开始游戏了
			//websocket.Clients[sessionId].WriteJSON(gin.H{
			//	"type": "state",
			//	"code": -1,
			//})
			onlineWebSocket.SessionIdSendMessage(sessionId, gin.H{
				"type": "state",
				"code": -1,
			})
		}

	} else {
		//新房间
		websocket.RoomMapMu.Lock()
		websocket.RoomMap[reqJson.Room] = map[string]interface{}{
			"states": false, //初始化，未开始游戏
			"userList": map[string]string{
				sessionId: reqJson.Name, //sessionID和用户昵称
			},
			"sessionIdList": []string{
				sessionId,
			},
		}
		websocket.RoomMapMu.Unlock()

		//添加用户和房间映射
		websocket.UserRoomMapMu.RLock()
		websocket.UserRoomMap[sessionId] = reqJson.Room
		websocket.UserRoomMapMu.RUnlock()
		onlineWebSocket.SessionIdSendMessage(sessionId, gin.H{
			"type": "state",
			"code": 2,
		})
	}

}

/*
 * @Description: 出牌操作
 * @receiver this
 * @param c
 */
func (this *ApiController) SentPorker(c *gin.Context) {
	type ReqJson struct {
		Room          string   `json:"room"`
		ChkPokerIndex []int    `json:"chk_poker_index"`
		ChkPoker      []string `json:"chk_poker"`
	}
	session := sessions.Default(c)
	sessionId := session.Get("session_id").(string)
	onlineWebSocket := &websocket.OnlineWebSocket{}
	reqJson := ReqJson{}
	err := c.BindJSON(&reqJson)
	if err != nil {
		fmt.Println(err)
	}
	corn := websocket.RoomMap[reqJson.Room]["cron"].(*cron.Cron)
	ctx := context.Background()

	//取出当前房间上一次玩家的出牌
	bytes, err := redis.RClient.HGet(context.Background(), reqJson.Room, "prevPoker").Bytes()
	if err != nil {
		fmt.Println(err)
	}
	prevPoker := map[string]int{}
	json.Unmarshal(bytes, &prevPoker)
	//取出玩家出牌的数值
	prevPokerIndex := []int{}
	for _, v := range prevPoker {
		prevPokerIndex = append(prevPokerIndex, v)
	}

	//更具出牌数量进行判断
	if len(reqJson.ChkPokerIndex) == 1 {
		msg, ok := game.SoloPoker(prevPokerIndex, reqJson.ChkPokerIndex)
		//出牌失败就返回错误信息
		if !ok {
			c.JSON(200, gin.H{
				"code":    -1,
				"message": msg,
			})
			return
		}
	} else if len(reqJson.ChkPokerIndex) == 2 {
		msg, ok := game.DoublePoker(prevPokerIndex, reqJson.ChkPokerIndex)
		//出牌失败就返回错误信息
		if !ok {
			c.JSON(200, gin.H{
				"code":    -1,
				"message": msg,
			})
			return
		}
	} else if len(reqJson.ChkPokerIndex) >= 3 {
		msg, ok := game.StraightPoker(prevPokerIndex, reqJson.ChkPokerIndex)
		//出牌失败就返回错误信息
		if !ok {
			c.JSON(200, gin.H{
				"code":    -1,
				"message": msg,
			})
			return
		}

	}

	//满足出牌要求后清除上次的牌，把本次的牌放入
	prevPoker = map[string]int{}
	for i := 0; i < len(reqJson.ChkPokerIndex); i++ {
		prevPoker[reqJson.ChkPoker[i]] = reqJson.ChkPokerIndex[i]
	}
	marshal, err := json.Marshal(prevPoker)
	if err != nil {
		fmt.Println(err)
	}
	redis.RClient.HSet(context.Background(), reqJson.Room, "prevPoker", marshal)     //存储出的牌
	redis.RClient.HSet(context.Background(), reqJson.Room, "prevGamerId", sessionId) //存储成功出牌的玩家会话id
	redis.RClient.HSet(context.Background(), reqJson.Room, "roundRecording", 0)      //初始化回合记录
	bytes2, err := redis.RClient.HGet(context.Background(), reqJson.Room, sessionId).Bytes()
	if err != nil {
		fmt.Println(err)
	}
	thisUserPorker := map[string]int{}
	json.Unmarshal(bytes2, &thisUserPorker)
	//删除玩家的手牌
	for _, v := range reqJson.ChkPoker {
		delete(thisUserPorker, v)
	}
	//玩家手牌如果空了，代表游戏胜利
	if len(thisUserPorker) == 0 {
		onlineWebSocket.RoomSendMessage(reqJson.Room, gin.H{
			"type":       "game_over",
			"victory_id": sessionId, //胜利玩家的会话id
		})
		//删除当前房间
		websocket.RoomMapMu.Lock()
		delete(websocket.RoomMap, reqJson.Room) //map中删除房间信息
		websocket.RoomMapMu.Unlock()
		redis.RClient.Del(context.Background(), reqJson.Room) //redis中删除对局信息

		return
	}

	//如果游戏没结束，重置当前玩家手牌
	marshal2, err := json.Marshal(thisUserPorker)
	if err != nil {
		fmt.Println(err)
	}
	redis.RClient.HSet(context.Background(), reqJson.Room, sessionId, marshal2)

	//将玩家所出的牌发送给客户端
	onlineWebSocket.RoomSendMessage(reqJson.Room, gin.H{
		"type":      "sent_poker",
		"prevPoker": prevPoker,
		"id":        sessionId,
	})

	//给下一个玩家发送出牌指令
	websocket.RoomMapMu.RLock()
	sessionIdList := websocket.RoomMap[reqJson.Room]["sessionIdList"].([]string)
	websocket.RoomMapMu.RUnlock()
	nextGamerId := game.GetNextUser(sessionIdList, sessionId, prevPoker)

	//var userIndex int //当前用户在数组中的索引
	//for k, v := range sessionIdList {
	//	if v == sessionId {
	//		userIndex = k
	//		break
	//	}
	//}
	//var nextGamerId string //下一个出牌用户的id
	////如果当前用户索引值等于最后一位，那么下一个出牌的就是第一个用户
	//if userIndex == len(sessionIdList)-1 {
	//	nextGamerId = sessionIdList[0]
	//} else {
	//	if len(prevPoker) != 0 {
	//		nextGamerId = sessionIdList[userIndex+1]
	//	} else {
	//		nextGamerId = sessionIdList[userIndex]
	//	}
	//}

	timeId, err := redis.RClient.HGet(ctx, reqJson.Room, "timeId").Int()
	if err != nil {
		fmt.Println(err)
	}

	if timeId != 0 {
		corn.Remove(cron.EntryID(timeId))
		//清空redis中的定时器id
		redis.RClient.HSet(ctx, reqJson.Room, "timeId", 0)
		fmt.Println("定时器销毁...")
	}

	onlineWebSocket.RoomSendMessage(reqJson.Room, gin.H{
		"type":         "now_gamer",
		"now_gamer_id": nextGamerId,
	})

	//通知玩家出牌
	onlineWebSocket.SessionIdSendMessage(nextGamerId, gin.H{
		"type": "now_sent_poker",
		"time": game.NextTime,
	})

	c.JSON(200, gin.H{
		"code": 1,
	})

}

/*
 * @Description: 不出牌
 * @receiver this
 * @param c
 */
func (this *ApiController) NotPoker(c *gin.Context) {
	session := sessions.Default(c)
	sessionId := session.Get("session_id").(string)
	type ReqJson struct {
		Room string `json:"room"`
		Name string `json:"name"`
	}
	reqJson := ReqJson{}
	err := c.BindJSON(&reqJson)
	if err != nil {
		fmt.Println(err)
	}
	corn := websocket.RoomMap[reqJson.Room]["cron"].(*cron.Cron)

	onlineWebSocket := &websocket.OnlineWebSocket{}
	ctx := context.Background()

	roundRecording, err := redis.RClient.HGet(ctx, reqJson.Room, "roundRecording").Int()
	if err != nil {
		fmt.Println(err)
	}
	prevPokerBytes, err := redis.RClient.HGet(ctx, reqJson.Room, "prevPoker").Bytes()
	if err != nil {
		fmt.Println(err)
	}
	prevPoker := map[string]int{}
	json.Unmarshal(prevPokerBytes, &prevPoker)

	if len(prevPoker) == 0 {
		//如果上次出牌为空，本次就必须出牌
		c.JSON(200, gin.H{
			"code":    -1,
			"message": "不能不出牌啊，哥",
		})
		return
	}

	if roundRecording == game.GameNumber-2 {
		victoryId, err := redis.RClient.HGet(ctx, reqJson.Room, "prevGamerId").Result() //取出上回合胜利玩家的会话ID
		if err != nil {
			fmt.Println(err)
		}
		redis.RClient.HSet(ctx, reqJson.Room, "roundRecording", 0) //初始化回合记录
		pokerBytes, err := redis.RClient.HGet(ctx, reqJson.Room, "poker").Bytes()
		if err != nil {
			fmt.Println(err)
		}
		poker := []string{}
		json.Unmarshal(pokerBytes, &poker)
		victoryPokerBytes, err := redis.RClient.HGet(ctx, reqJson.Room, victoryId).Bytes() //上回合胜利玩家的手牌
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
			redis.RClient.HSet(ctx, reqJson.Room, "poker", pokerMarshal)
			//将新牌发给上回合胜利的玩家
			victoryPoker[pokerKey] = pokerValue
			victoryPokerMarshal, err := json.Marshal(victoryPoker)
			if err != nil {
				fmt.Println(err)
			}
			redis.RClient.HSet(ctx, reqJson.Room, victoryId, victoryPokerMarshal)

			onlineWebSocket.SessionIdSendMessage(victoryId, gin.H{
				"type": "send_poker",
				"poker": map[string]int{
					pokerKey: pokerValue,
				},
			})
		}

		//将上回合出牌内容清空
		redis.RClient.HSet(ctx, reqJson.Room, "prevPoker", "")

		timeId, err := redis.RClient.HGet(ctx, reqJson.Room, "timeId").Int()
		if err != nil {
			fmt.Println(err)
		}

		if timeId != 0 {
			corn.Remove(cron.EntryID(timeId))
			//清空redis中的定时器id
			redis.RClient.HSet(ctx, reqJson.Room, "timeId", 0)
			fmt.Println("定时器销毁...")
		}

		//回合胜利者出牌
		onlineWebSocket.SessionIdSendMessage(victoryId, gin.H{
			"type": "now_sent_poker",
		})

		//告诉房间玩家该谁出牌
		onlineWebSocket.RoomSendMessage(reqJson.Room, gin.H{
			"type":         "now_gamer",
			"now_gamer_id": victoryId,
		})

	} else {
		redis.RClient.HSet(ctx, reqJson.Room, "roundRecording", roundRecording+1) //初始化回合记录
		websocket.RoomMapMu.RLock()
		sessionIdList := websocket.RoomMap[reqJson.Room]["sessionIdList"].([]string)
		websocket.RoomMapMu.RUnlock()
		nextUserId := game.GetNextUser(sessionIdList, sessionId, prevPoker)

		//将上回合出牌内容清空
		redis.RClient.HSet(ctx, reqJson.Room, "prevPoker", "")

		timeId, err := redis.RClient.HGet(ctx, reqJson.Room, "timeId").Int()
		if err != nil {
			fmt.Println(err)
		}

		if timeId != 0 {
			corn.Remove(cron.EntryID(timeId))
			//清空redis中的定时器id
			redis.RClient.HSet(ctx, reqJson.Room, "timeId", 0)
			fmt.Println("定时器销毁...")
		}

		onlineWebSocket.RoomSendMessage(reqJson.Room, gin.H{
			"type":         "now_gamer",
			"now_gamer_id": nextUserId,
		})

		onlineWebSocket.SessionIdSendMessage(nextUserId, gin.H{
			"type": "now_sent_poker",
			"time": game.NextTime,
		})
	}

	c.JSON(200, gin.H{
		"code": 1,
	})

}

func (this *ApiController) Test(c *gin.Context) {
	//fmt.Println(websocket.RoomMap)
	msg, ok := game.StraightPoker([]int{12, 13, 1}, []int{3, 4, 5})
	fmt.Println(msg)
	fmt.Println(ok)
}
