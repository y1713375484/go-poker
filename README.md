# Go-poker
基于golang的一款干瞪眼扑克游戏

### 环境
需要redis，并且填写env文件

### 游戏截图
![游戏截图](/public/static/img/jt.jpg "Magic Gardens")


### 游戏玩法
默认运行端口8080
启动后访问http://127.0.0.1:8080?room=1，room=1代表房间号是1，默认一个房间内2个玩家进行游戏，当有两个玩家访问http://127.0.0.1:8080?room=1并准备后，游戏就会开始。
<br>
<br>
支持单牌、对子、顺子、炸弹等出法，开局给第一个加入房间并准备的玩家发6张牌，视为庄家，其余玩家各发5张牌，然后从庄家开始出牌
#### 单牌
如果上家出3、下家必须出4或者2或者炸弹才行
<br>
如果上家出2、那么下家只能出炸弹

#### 对子
出牌逻辑与单牌一样

#### 顺子
三张以上为顺子，上家如若出3、4、5那么下家必须出4、5、6或者炸弹才行
<br>
如若出Q、K、A那么只能出炸弹才行