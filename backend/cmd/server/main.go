package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"drone-sim-backend/internal/config"
	"drone-sim-backend/internal/engine"
	"drone-sim-backend/internal/mqtt"
	"drone-sim-backend/internal/parser"
	"drone-sim-backend/internal/trajectory"

	pahomqtt "github.com/eclipse/paho.mqtt.golang"
)

var (
	simEngine            *engine.Engine
	mqttPublisher        *mqtt.Publisher // 外部 Broker（供其他系统）
	localPublisher       *mqtt.Publisher // 内嵌 Broker（供前端）
	mqttBroker           *mqtt.EmbeddedBroker
	uploadedTrajectories = make(map[string]*trajectory.Trajectory) // droneId -> traj
)

func main() {
	cfg := config.LoadConfig()

	// 启动内嵌 MQTT Broker（无需外部 Mosquitto）
	mqttBroker = mqtt.NewEmbeddedBroker(":1883", ":1884")
	if err := mqttBroker.Start(); err != nil {
		log.Fatalf("[MQTT Broker] 启动失败: %v", err)
	}

	// 初始化 MQTT Publisher，可通过环境变量 MQTT_BROKER 配置目标地址
	mqttPublisher = mqtt.NewPublisher(cfg.MQTTBroker, cfg.MQTTClientID, cfg.MQTTUsername, cfg.MQTTPassword)
	if err := mqttPublisher.Connect(); err != nil {
		log.Fatalf("[MQTT] Publisher 连接失败: %v", err)
	}
	log.Println("[MQTT] Publisher 已连接到外部 Broker:", cfg.MQTTBroker)

	// 同时连接到内嵌 Broker，供前端 WebSocket 订阅
	localPublisher = mqtt.NewPublisher("tcp://127.0.0.1:1883", cfg.MQTTClientID+"-local", "", "")
	if err := localPublisher.Connect(); err != nil {
		log.Fatalf("[MQTT] 内嵌 Publisher 连接失败: %v", err)
	}
	log.Println("[MQTT] 内嵌 Publisher 已连接到本地 Broker")

	// 初始化仿真引擎，传入遥测回调和模式变更回调（双写）
	simEngine = engine.NewEngine(
		func(tel trajectory.RemoteIDTelemetry) {
			localPublisher.PublishTelemetry(tel)
			mqttPublisher.PublishTelemetry(tel)
		},
		func(droneID, status string) {
			localPublisher.PublishModeChange(droneID, status)
			mqttPublisher.PublishModeChange(droneID, status)
		},
	)

	// 订阅 thing/product/+/services，接收飞行控制指令（必须在 simEngine 初始化之后）
	setupServiceSubscriber()

	r := gin.Default()
	r.Use(cors.New(cors.Config{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders: []string{"Origin", "Content-Type", "Authorization"},
	}))

	// 健康检查
	r.GET("/api/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// 上传 KMZ 文件
	r.POST("/api/upload", func(c *gin.Context) {
		form, err := c.MultipartForm()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请上传文件"})
			return
		}

		files := form.File["files"]
		if len(files) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "没有文件"})
			return
		}

		var results []*trajectory.Trajectory
		for _, fileHeader := range files {
			file, err := fileHeader.Open()
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "无法打开文件 " + fileHeader.Filename})
				return
			}
			defer file.Close()

			// 根据扩展名找解析器
			ext := ".kmz" // 默认
			for i := len(fileHeader.Filename) - 1; i >= 0; i-- {
				if fileHeader.Filename[i] == '.' {
					ext = fileHeader.Filename[i:]
					break
				}
			}

			routeParser, ok := parser.DefaultRegistry().FindParser(ext)
			if !ok {
				c.JSON(http.StatusBadRequest, gin.H{"error": "不支持的文件格式: " + ext})
				return
			}

			traj, err := routeParser.Parse(file, fileHeader.Filename)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "解析失败: " + err.Error()})
				return
			}

			// 注册到引擎
			droneID := simEngine.AddDrone(traj)
			traj.ID = droneID
			uploadedTrajectories[droneID] = traj
			results = append(results, traj)

			// 发布设备上线 state 消息（双写）
			localPublisher.PublishDeviceOnline(droneID)
			mqttPublisher.PublishDeviceOnline(droneID)
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    results,
		})
	})

	// 仿真控制 API
	r.POST("/api/sim/start", func(c *gin.Context) {
		var req struct {
			DroneID string `json:"droneId"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 droneId"})
			return
		}
		if err := simEngine.Start(req.DroneID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true})
	})

	r.POST("/api/sim/pause", func(c *gin.Context) {
		var req struct {
			DroneID string `json:"droneId"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 droneId"})
			return
		}
		if err := simEngine.Pause(req.DroneID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true})
	})

	r.POST("/api/sim/resume", func(c *gin.Context) {
		var req struct {
			DroneID string `json:"droneId"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 droneId"})
			return
		}
		if err := simEngine.Resume(req.DroneID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true})
	})

	r.POST("/api/sim/stop", func(c *gin.Context) {
		var req struct {
			DroneID string `json:"droneId"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 droneId"})
			return
		}
		if err := simEngine.Stop(req.DroneID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true})
	})

	r.POST("/api/sim/speed", func(c *gin.Context) {
		var req struct {
			DroneID string  `json:"droneId"`
			Speed   float64 `json:"speed"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "缺少参数"})
			return
		}
		if err := simEngine.SetSpeed(req.DroneID, req.Speed); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true})
	})

	// 风力模拟控制 API
	r.POST("/api/sim/wind", func(c *gin.Context) {
		var req struct {
			DroneID   string  `json:"droneId"`
			Speed     float64 `json:"speed"`     // 风速 (m/s)
			Direction float64 `json:"direction"` // 风向 (度, 0=北, 气象方向)
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "缺少参数"})
			return
		}
		if err := simEngine.SetWind(req.DroneID, req.Speed, req.Direction); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true})
	})

	r.DELETE("/api/sim/drone/:id", func(c *gin.Context) {
		droneID := c.Param("id")
		if err := simEngine.RemoveDrone(droneID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		delete(uploadedTrajectories, droneID)
		c.JSON(http.StatusOK, gin.H{"success": true})
	})

	r.GET("/api/sim/status", func(c *gin.Context) {
		statusMap := simEngine.GetStatus()
		// 转换为数组
		var statusList []engine.DroneStatusInfo
		for _, info := range statusMap {
			statusList = append(statusList, info)
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": statusList})
	})

	r.GET("/api/sim/trajectories", func(c *gin.Context) {
		trajs := simEngine.GetTrajectories()
		c.JSON(http.StatusOK, gin.H{"success": true, "data": trajs})
	})

	// 启动服务
	log.Println("[Server] 启动在端口", cfg.ServerPort)
	if err := r.Run(cfg.ServerPort); err != nil {
		log.Fatal("服务启动失败:", err)
	}
}

// 确保 parser 包的 init 注册被触发
var _ = parser.DefaultRegistry()

// ──────────────────────────────────────────────
// DJI MQTT 服务指令处理
// ──────────────────────────────────────────────

// djiServiceCommand DJI thing/product/{sn}/services 指令格式
type djiServiceCommand struct {
	TID       string          `json:"tid"`
	BID       string          `json:"bid"`
	Timestamp int64           `json:"timestamp"`
	Data      djiServiceData  `json:"data"`
}

type djiServiceData struct {
	Method string `json:"method"`
}

// djiServiceReply 指令回复
type djiServiceReply struct {
	TID       string         `json:"tid"`
	BID       string         `json:"bid"`
	Timestamp int64          `json:"timestamp"`
	Data      djiReplyData   `json:"data"`
}

type djiReplyData struct {
	Result int `json:"result"` // 0=成功
}

func setupServiceSubscriber() {
	opts := pahomqtt.NewClientOptions()
	opts.AddBroker("tcp://127.0.0.1:1883")
	opts.SetClientID("drone-sim-services-sub")
	opts.SetCleanSession(true)
	opts.SetAutoReconnect(true)

	opts.SetDefaultPublishHandler(func(client pahomqtt.Client, msg pahomqtt.Message) {
		topic := msg.Topic()
		// 从 topic 提取 droneId: thing/product/{droneId}/services
		parts := strings.Split(topic, "/")
		if len(parts) < 3 {
			return
		}
		droneID := parts[2]

		var cmd djiServiceCommand
		if err := json.Unmarshal(msg.Payload(), &cmd); err != nil {
			log.Println("[Services] 解析指令失败:", err)
			return
		}

		log.Printf("[Services] 收到指令 drone=%s method=%s", droneID, cmd.Data.Method)

		var execErr error
		switch cmd.Data.Method {
		case "flight_task_execute":
			execErr = simEngine.Start(droneID)
		case "flight_task_pause":
			execErr = simEngine.Pause(droneID)
		case "flight_task_resume":
			execErr = simEngine.Resume(droneID)
		case "flight_task_terminate":
			execErr = simEngine.Stop(droneID)
		case "flight_task_return_home":
			execErr = simEngine.ReturnHome(droneID)
		default:
			log.Println("[Services] 未知方法:", cmd.Data.Method)
			return
		}

		result := 0
		if execErr != nil {
			result = 1
			log.Println("[Services] 执行失败:", execErr)
		}

		// 回复到 services_reply
		reply := djiServiceReply{
			TID:       cmd.TID,
			BID:       cmd.BID,
			Timestamp: cmd.Timestamp,
			Data:      djiReplyData{Result: result},
		}
		replyData, _ := json.Marshal(reply)
		client.Publish(
			"thing/product/"+droneID+"/services_reply",
			0,
			false,
			string(replyData),
		)
	})

	client := pahomqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Println("[Services] 订阅连接失败:", token.Error())
		return
	}

	if token := client.Subscribe("thing/product/+/services", 0, nil); token.Wait() && token.Error() != nil {
		log.Println("[Services] 订阅失败:", token.Error())
		return
	}

	log.Println("[Services] 已订阅 thing/product/+/services")
}
