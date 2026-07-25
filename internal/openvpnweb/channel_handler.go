package openvpnweb

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"openvpn-web/internal/openvpnweb/notify"
)

// channelHandler 统一处理 /ovpn/channel 下的 CRUD 路由
func channelHandler(c *gin.Context) {
	switch c.Request.Method {
	case "GET":
		// GET /ovpn/channel - 列出全部渠道
		// GET /ovpn/channel/:id - 取单条（参数由路径提供）
		idStr := c.Param("id")
		if idStr == "" {
			channels := (&NotificationChannel{}).All()
			c.JSON(200, gin.H{"data": channels})
			return
		}
		id64, err := strconv.ParseUint(idStr, 10, 64)
		if err != nil {
			c.JSON(400, gin.H{"message": "无效的渠道 ID"})
			return
		}
		ch, err := (&NotificationChannel{}).Get(uint(id64))
		if err != nil {
			c.JSON(500, gin.H{"message": err.Error()})
			return
		}
		c.JSON(200, gin.H{"data": ch})
	case "POST":
		// POST /ovpn/channel - 新建
		var ch NotificationChannel
		if err := c.ShouldBind(&ch); err != nil {
			c.JSON(400, gin.H{"message": "请求参数错误：" + err.Error()})
			return
		}
		if err := ch.Create(); err != nil {
			c.JSON(500, gin.H{"message": err.Error()})
			return
		}
		c.JSON(200, gin.H{"message": "添加成功", "data": ch})
	case "PUT", "PATCH":
		// PUT/PATCH /ovpn/channel/:id - 更新
		idStr := c.Param("id")
		id64, err := strconv.ParseUint(idStr, 10, 64)
		if err != nil {
			c.JSON(400, gin.H{"message": "无效的渠道 ID"})
			return
		}
		var ch NotificationChannel
		if err := c.ShouldBind(&ch); err != nil {
			c.JSON(400, gin.H{"message": "请求参数错误：" + err.Error()})
			return
		}
		ch.ID = uint(id64)
		if err := ch.Update(); err != nil {
			c.JSON(500, gin.H{"message": err.Error()})
			return
		}
		c.JSON(200, gin.H{"message": "更新成功", "data": ch})
	case "DELETE":
		// DELETE /ovpn/channel/:id - 删除
		idStr := c.Param("id")
		id64, err := strconv.ParseUint(idStr, 10, 64)
		if err != nil {
			c.JSON(400, gin.H{"message": "无效的渠道 ID"})
			return
		}
		ch := NotificationChannel{}
		ch.ID = uint(id64)
		if err := ch.Delete(); err != nil {
			c.JSON(500, gin.H{"message": err.Error()})
			return
		}
		c.JSON(200, gin.H{"message": "删除成功"})
	default:
		c.JSON(405, gin.H{"message": "不支持的请求方法"})
	}
}

// channelTestHandler POST /ovpn/channel/:id/test - 给指定渠道发一条测试消息
func channelTestHandler(c *gin.Context) {
	idStr := c.Param("id")
	id64, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"message": "无效的渠道 ID"})
		return
	}
	if err := sendChannelTestMessage(uint(id64)); err != nil {
		c.JSON(500, gin.H{"message": err.Error()})
		return
	}
	c.JSON(200, gin.H{"message": "测试发送成功"})
}

// channelTypesHandler GET /ovpn/channel-types - 列出所有支持的渠道类型
func channelTypesHandler(c *gin.Context) {
	types := make([]gin.H, 0, len(notify.AllChannelTypes))
	for _, t := range notify.AllChannelTypes {
		types = append(types, gin.H{
			"type":  t,
			"label": notify.ChannelTypeLabel(t),
			"icon":  notify.ChannelTypeIcon(t),
		})
	}
	c.JSON(200, gin.H{"data": types})
}
