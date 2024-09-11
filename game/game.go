package game

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"porker/redis"
	"slices"
	"sort"
	"time"
)

type Game struct {
}

var GameNumber int //玩家数量

var NextTime int //

var Poker = map[string]int{
	"♠A": 1, "♠2": 2, "♠3": 3, "♠4": 4, "♠5": 5, "♠6": 6, "♠7": 7, "♠8": 8, "♠9": 9, "♠10": 10, "♠J": 11, "♠Q": 12, "♠K": 13,
	"♣A": 1, "♣2": 2, "♣3": 3, "♣4": 4, "♣5": 5, "♣6": 6, "♣7": 7, "♣8": 8, "♣9": 9, "♣10": 10, "♣J": 11, "♣Q": 12, "♣K": 13,
	"♥A": 1, "♥2": 2, "♥3": 3, "♥4": 4, "♥5": 5, "♥6": 6, "♥7": 7, "♥8": 8, "♥9": 9, "♥10": 10, "♥J": 11, "♥Q": 12, "♥K": 13,
	"♦A": 1, "♦2": 2, "♦3": 3, "♦4": 4, "♦5": 5, "♦6": 6, "♦7": 7, "♦8": 8, "♦9": 9, "♦10": 10, "♦J": 11, "♦Q": 12, "♦K": 13,
}

/*
 * @Description: 初始化牌局
 * @param roomUserSessionIdArray  房间用户的会话id数组
 * @param room	房间号
 */
func InitGame(roomUserSessionIdArray []string, room string) {
	//初始化poker列表

	keys := []string{}
	for k, _ := range Poker {
		keys = append(keys, k)
	}
	shuffle(keys)
	gamePorker := map[string]map[string]int{}
	for i := 0; i < 5; i++ {
		for j := 0; j < GameNumber; j++ {
			//从第一张牌开始发，发完从切片中删除
			if gamePorker[roomUserSessionIdArray[j]] != nil {
				gamePorker[roomUserSessionIdArray[j]][keys[0]] = Poker[keys[0]]
			} else {
				gamePorker[roomUserSessionIdArray[j]] = map[string]int{
					keys[0]: Poker[keys[0]],
				}
			}

			keys = keys[1:]
		}
	}

	//给第一位玩家多发一张，因为是庄
	gamePorker[roomUserSessionIdArray[0]][keys[0]] = Poker[keys[0]]
	keys = keys[1:]
	ctx := context.Background()
	for v := 0; v < GameNumber; v++ {
		marshal, err := json.Marshal(gamePorker[roomUserSessionIdArray[v]])
		if err != nil {
			fmt.Println(err)
		}
		redis.RClient.HSet(ctx, room, roomUserSessionIdArray[v], marshal)
	}
	keysJson, err := json.Marshal(keys)
	if err != nil {
		fmt.Println(err)
	}
	redis.RClient.HSet(ctx, room, "poker", keysJson)

}

/*
 * @Description: 洗牌方法
 * @param slice	要进行洗牌的切片
 */
func shuffle(slice []string) {
	rand.Seed(time.Now().UnixNano())

	rand.Shuffle(len(slice), func(i, j int) {
		slice[i], slice[j] = slice[j], slice[i]
	})
}

/*
 * @Description: 出单牌逻辑判断
 * @param prevPoker	上位玩家出牌点数
 * @param ChkPoker	当前玩家出牌点数
 * @return string
 * @return bool
 */
func SoloPoker(prevPoker []int, chkPoker []int) (string, bool) {

	//有上次出牌记录
	if len(prevPoker) != 0 {
		prevPokerIndex := prevPoker[0]
		chkPokerIndex := chkPoker[0]
		if len(prevPoker) != len(chkPoker) {
			return "人家出了几张牌？你看看你出了几张", false
		}

		if prevPokerIndex == 2 {
			return "人家都出2了，你还出个毛啊", false
		} else if chkPokerIndex != 2 {
			if prevPokerIndex == 13 {

				if chkPokerIndex != 1 {
					return "人家出的k，你出的什么玩意", false
				}

			} else if prevPokerIndex != chkPokerIndex-1 {
				return "没2就歇着吧", false
			}

		}

	}

	return "", true
}

/*
 * @Description: 出对子
 * @param prevPoker
 * @param chkPoker
 * @return string
 * @return bool
 */
func DoublePoker(prevPoker []int, chkPoker []int) (string, bool) {

	//检测两张牌是否是对子
	if chkPoker[0] != chkPoker[1] {
		return "你瞅瞅你出的是对子么？", false
	}

	if len(prevPoker) != 0 {

		if len(prevPoker) != len(chkPoker) {
			//确保出的牌型一致
			return "你瞅瞅人家出了几张，你出了几张", false
		}
		//确保是对子后，只需要对比一张牌点数即可
		if prevPoker[0] == 2 {
			//上一个玩家出的对2
			return "人家出的对2你还出个毛啊", false
		} else if prevPoker[0] != 2 {
			if prevPoker[0] == 13 {

				if chkPoker[0] != 1 {
					return "人家出的k，你出的什么玩意", false
				}

			} else if prevPoker[0] != chkPoker[0]-1 {
				return "没2就歇着吧", false
			}

		}
	}

	return "", true

}

/*
 * @Description: 三连
 * @param prevPoker
 * @param chkPoker
 * @return string
 * @return bool
 */
func StraightPoker(prevPoker []int, chkPoker []int) (string, bool) {

	sort.Ints(chkPoker) //升序排序

	isBoom := Boom(chkPoker)
	isSz := Sz(chkPoker, len(chkPoker))

	fmt.Println(isSz, isBoom)

	//又不是顺子又不是炸弹
	if !isBoom && !isSz {
		return "你这牌又不是顺子又不是炸弹，你出了个啥？", false
	}

	if len(prevPoker) != 0 {
		//如果不是第一次出牌
		sort.Ints(prevPoker)
		prevBoom := Boom(prevPoker)
		prevSz := Sz(prevPoker, len(prevPoker))

		//假如上次出的炸弹
		if prevBoom {
			//先判断当前玩家是否出的是炸弹且炸弹牌的数量是否大于等于上一玩家
			if isBoom {
				if len(chkPoker) < len(prevPoker) {
					return "你这炸弹威力小了点", false
				} else if len(chkPoker) == len(prevPoker) {
					//数量相等那么就判断牌的值
					if prevPoker[0] == 2 {
						//如果上次是2炸
						return "人家是2炸啊，你炸不过啊", false
					}

					if chkPoker[0] != 2 {
						//如果当前玩家不是2炸
						if prevPoker[0] == 1 {
							//入果当前玩家出的不是2炸，上一个玩家出的A炸，那我就无法出牌
							return "没2炸就忍忍吧，A炸也不小了", false
						} else if prevPoker[0] == 13 {
							//入果当前玩家出的不是2炸，上一个玩家出的k炸，那我就必须出A炸
							if chkPoker[0] != 1 {
								return "你这炸弹威力小了点", false
							}
						} else {
							if prevPoker[0] > chkPoker[0] {
								return "亲，这边建议你换个威力大的炸弹重新试试", false
							}
						}
					}

				}
			} else {
				return "没炸弹就歇着吧", false
			}
		}

		//假如上次出的顺子
		if prevSz {
			//判断是不是包含A的特殊顺子类似于 ...J Q K A
			if slices.Contains(prevPoker, 1) {
				if !isBoom {
					return "顺子界天花板你怎么要？", false
				}
			}

			//如果上一个玩家和当前都出的是顺子
			if isSz {
				//当前玩家与上一个玩家的牌长度一样
				if len(prevPoker) == len(chkPoker) {
					/*
					 * 例子：上一位玩家如果出3、4、5 那么当前玩家不出炸弹就必须出 4、5、6
					 * 3 4 5 和为 12     4 5 6和为 15
					 * 4 5 6 7 和为 22    5 6 7 8 和为 26
					 * 规律：如果当前玩家出的牌符合规则
					 * 那么,当前玩家所出牌的点数之和,减上一位玩家出牌的点数之和,就等于当前玩家出牌数(当前玩家和上一位玩家出牌数量相等),
					 * 反之不等于出牌数量就说明不符合规则
					 */
					prevSum := make(chan int)
					chkSum := make(chan int)
					pokerSum := func(slice []int, sumChan chan int) {
						sum := 0
						for _, v := range slice {
							sum += v
						}
						sumChan <- sum
					}
					//协程计算
					go pokerSum(prevPoker, prevSum)
					go pokerSum(chkPoker, chkSum)

					if <-chkSum-<-prevSum != len(chkPoker) {
						return "不符合顺子出牌规则哦", false
					}

				} else {
					return "你这顺子和人家的顺子长度怎么不一样呢？", false
				}
			}
		}

	}

	return "", true
}

/*
 * @Description: 判断当前排序是否为炸弹
 * @param pokerList
 * @return bool
 */
func Boom(pokerList []int) bool {
	sum := 0
	for i := 0; i < len(pokerList); i++ {
		if pokerList[0] != pokerList[i] {
			sum++
		}
	}

	if sum == 0 {
		return true
	} else {
		return false
	}

}

func Sz(pokerList []int, pokerListLen int) bool {

	//先将切片去重，检测是否有重复值
	seen := make(map[int]bool)
	result := []int{}
	for _, val := range pokerList {
		if _, ok := seen[val]; !ok {
			seen[val] = true
			result = append(result, val)
		}
	}

	if len(result) != pokerListLen {
		return false
	}

	//检测顺子中是否有2，2不能当作顺子使用
	if slices.Contains(pokerList, 2) {
		return false
	}

	/*
	 * 特殊的顺子
	 * 类似与 ...J Q K A 但是不能 A 2 3 4....
	 */
	if slices.Contains(pokerList, 1) {

		if slices.Contains(pokerList, 13) {
			//因为已经排序过那么A肯定为数组第一位就是0，所以只需判断其他牌是否是连续数字，从1开始判断
			for i := 1; i < pokerListLen-1; i++ {
				if pokerList[i]+1 != pokerList[i+1] {
					return false
				}
			}
		} else {
			return false
		}

	} else {
		//不是特殊顺子那么就直接判断是不是连续数段

		for i := 0; i < pokerListLen-1; i++ {
			if pokerList[i]+1 != pokerList[i+1] {
				return false
			}
		}
	}

	return true
}

/*
 * @Description: 获取下一个出牌玩家的会话id
 * @param sessionIdList	房间内玩家会话id
 * @param sessionId	当前玩家会话id
 * @param prevPoker	上一次出的牌
 * @return string
 */
func GetNextUser(sessionIdList []string, sessionId string, prevPoker map[string]int) string {
	//给下一个玩家发送出牌指令
	//websocket.RoomMapMu.RLock()
	//sessionIdList := websocket.RoomMap[room]["sessionIdList"].([]string)
	//websocket.RoomMapMu.RUnlock()
	var userIndex int //当前用户在数组中的索引
	for k, v := range sessionIdList {
		if v == sessionId {
			userIndex = k
			break
		}
	}
	var nextGamerId string //下一个出牌用户的id
	//如果当前用户索引值等于最后一位，那么下一个出牌的就是第一个用户
	if userIndex == len(sessionIdList)-1 {
		nextGamerId = sessionIdList[0]
	} else {
		if len(prevPoker) != 0 {
			nextGamerId = sessionIdList[userIndex+1]
		} else {
			nextGamerId = sessionIdList[userIndex]
		}
	}

	return nextGamerId
}
