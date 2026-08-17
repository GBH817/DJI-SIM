package mqtt

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"drone-sim-backend/internal/trajectory"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/google/uuid"
)

// Publisher MQTT 发布器
type Publisher struct {
	client    mqtt.Client
	mu        sync.RWMutex
	connected bool
}

// NewPublisher 创建 MQTT 发布器
func NewPublisher(broker, clientID, username, password string) *Publisher {
	p := &Publisher{}
	opts := mqtt.NewClientOptions()
	opts.AddBroker(broker)
	opts.SetClientID(clientID)
	opts.SetAutoReconnect(true)
	opts.SetCleanSession(true)
	opts.SetConnectTimeout(5 * time.Second)
	if username != "" {
		opts.SetUsername(username)
		opts.SetPassword(password)
	}
	opts.OnConnect = func(c mqtt.Client) {
		p.mu.Lock()
		p.connected = true
		p.mu.Unlock()
		log.Println("[MQTT] 已连接到 Broker:", broker)
	}
	opts.OnConnectionLost = func(c mqtt.Client, err error) {
		p.mu.Lock()
		p.connected = false
		p.mu.Unlock()
		log.Println("[MQTT] 连接断开:", err)
	}

	p.client = mqtt.NewClient(opts)
	return p
}

// Connect 连接 MQTT Broker
func (p *Publisher) Connect() error {
	if token := p.client.Connect(); token.Wait() && token.Error() != nil {
		return token.Error()
	}
	p.mu.Lock()
	p.connected = true
	p.mu.Unlock()
	return nil
}

// Disconnect 断开连接
func (p *Publisher) Disconnect() {
	p.mu.Lock()
	p.connected = false
	p.mu.Unlock()
	p.client.Disconnect(250)
}

// ──────────────────────────────────────────────
// DJI 兼容格式的数据结构
// ──────────────────────────────────────────────

// DJITelemetry 大疆 MQTT 遥测格式（thing/product/{sn}/osd）
type DJITelemetry struct {
	TID       string     `json:"tid"`
	BID       string     `json:"bid"`
	Timestamp int64      `json:"timestamp"`
	Gateway   string     `json:"gateway"` // 设备序列号，映射 droneID
	Data      DJIOSDData `json:"data"`
}

// DJIOSDData OSD 数据
type DJIOSDData struct {
	Latitude           float64          `json:"latitude"`
	Longitude          float64          `json:"longitude"`
	Altitude           float64          `json:"altitude"`         // 椭球高度
	AttitudePitch      float64          `json:"attitude_pitch"`   // 俯仰角
	AttitudeRoll       float64          `json:"attitude_roll"`    // 横滚角
	AttitudeYaw        float64          `json:"attitude_yaw"`     // 偏航角 = heading
	HorizontalSpeed    float64          `json:"horizontal_speed"` // m/s
	VerticalSpeed      float64          `json:"vertical_speed"`   // m/s（模拟中始终为0）
	Height             float64          `json:"height"`           // 相对起飞点高度
	HomeDistance       float64          `json:"home_distance"`    // 距 home 点距离
	WindSpeed          float64          `json:"wind_speed"`       // 模拟值
	Battery            DJIBattery       `json:"battery"`
	PositionState      DJIPositionState `json:"position_state"`
	WaypointIndex      int              `json:"waypoint_index"`       // 当前航点
	TotalWaypoints     int              `json:"total_waypoints"`      // 总航点
	Progress           float64          `json:"progress"`             // 0-100 进度百分比
	TotalFlightTime    float64          `json:"total_flight_time"`    // 秒
	CurrentAction      string           `json:"current_action"`       // 当前动作类型
	HoverTimeRemaining float64          `json:"hover_time_remaining"` // 悬停剩余时间
	RunGeneration      int64            `json:"run_generation"`       // 仿真轮次
}

// DJIBattery 电池信息
type DJIBattery struct {
	CapacityPercent int     `json:"capacity_percent"`
	Voltage         float64 `json:"voltage"`
	Temperature     float64 `json:"temperature"`
}

// DJIPositionState 定位状态
type DJIPositionState struct {
	IsFixed int `json:"is_fixed"`
	Quality int `json:"quality"`
}

// toDJI 将内部遥测数据转换为大疆兼容格式
func toDJI(tel trajectory.RemoteIDTelemetry) DJITelemetry {
	batPercent := int(tel.BatteryPercent)
	if batPercent < 0 {
		batPercent = 0
	}
	if batPercent > 100 {
		batPercent = 100
	}
	// 根据电量百分比估算电压（4.2V满电 ~ 3.5V没电，6S电池为基准）
	voltage := 25.2*tel.BatteryPercent/100 + 21.0*(100-tel.BatteryPercent)/100

	return DJITelemetry{
		TID:       uuid.New().String(),
		BID:       uuid.New().String(),
		Timestamp: tel.Timestamp,
		Gateway:   tel.DroneID,
		Data: DJIOSDData{
			Latitude:        tel.Latitude,
			Longitude:       tel.Longitude,
			Altitude:        tel.Altitude,
			AttitudePitch:   0, // 模拟中假设平飞
			AttitudeRoll:    0,
			AttitudeYaw:     tel.Heading,
			HorizontalSpeed: tel.Speed,
			VerticalSpeed:   tel.VerticalSpeed,
			Height:          tel.HeightAboveTakeoff,
			HomeDistance:    0,
			WindSpeed:       tel.WindSpeed,
			Battery: DJIBattery{
				CapacityPercent: batPercent,
				Voltage:         voltage,
				Temperature:     35,
			},
			PositionState: DJIPositionState{
				IsFixed: 1,
				Quality: 5,
			},
			WaypointIndex:      tel.WaypointIndex,
			TotalWaypoints:     tel.TotalWaypoints,
			Progress:           tel.Progress,
			TotalFlightTime:    0,
			CurrentAction:      tel.CurrentAction,
			HoverTimeRemaining: tel.HoverTimeRemaining,
			RunGeneration:      tel.RunGeneration,
		},
	}
}

// ──────────────────────────────────────────────
// 发布方法
// ──────────────────────────────────────────────

// publish 内部发布方法
func (p *Publisher) publish(topic string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		log.Println("[MQTT] JSON 序列化失败:", err)
		return
	}
	token := p.client.Publish(topic, 0, false, string(data))
	token.Wait()
}

// publishRetained 发布保留消息，新订阅者立即可收到最后一条
func (p *Publisher) publishRetained(topic string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		log.Println("[MQTT] JSON 序列化失败:", err)
		return
	}
	token := p.client.Publish(topic, 0, true, string(data))
	token.Wait()
}

// PublishTelemetry 同时发布简化和大疆两种格式的遥测数据
//   - drone/{id}/telemetry  → 简化格式（当前前端使用）
//   - thing/product/{id}/osd → 大疆兼容格式（其他项目使用）
func (p *Publisher) PublishTelemetry(telemetry trajectory.RemoteIDTelemetry) {
	p.mu.RLock()
	if !p.connected {
		p.mu.RUnlock()
		return
	}
	p.mu.RUnlock()

	// 简化格式 topic
	p.publish(fmt.Sprintf("drone/%s/telemetry", telemetry.DroneID), telemetry)
	// 大疆兼容格式 topic
	p.publish(fmt.Sprintf("thing/product/%s/osd", telemetry.DroneID), toDJI(telemetry))
}

// PublishStatus 发布状态变更到 drone/{id}/status
func (p *Publisher) PublishStatus(droneID, status string, telemetry trajectory.RemoteIDTelemetry) {
	p.mu.RLock()
	if !p.connected {
		p.mu.RUnlock()
		return
	}
	p.mu.RUnlock()

	telemetry.Status = status
	p.publish(fmt.Sprintf("drone/%s/status", droneID), telemetry)
	// DJI 格式也同步推
	p.publish(fmt.Sprintf("thing/product/%s/osd", droneID), toDJI(telemetry))
}

// IsConnected 检查连接状态
func (p *Publisher) IsConnected() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.connected
}

// Client 返回底层 MQTT 客户端（用于订阅回复等）
func (p *Publisher) Client() mqtt.Client {
	return p.client
}

// ──────────────────────────────────────────────
// State 数据结构（thing/product/{sn}/state）
// 状态变化时按需上报，对应 DJI pushMode=1
// ──────────────────────────────────────────────

// DJIStateMessage state topic 消息信封
type DJIStateMessage struct {
	TID       string       `json:"tid"`
	BID       string       `json:"bid"`
	Timestamp int64        `json:"timestamp"`
	Gateway   string       `json:"gateway"`
	Data      DJIStateData `json:"data"`
}

// DJIStateData 设备状态属性（thing/product/{sn}/state）
type DJIStateData struct {
	FirmwareVersion   string       `json:"firmware_version"`   // 固件版本
	CompatibleStatus  int          `json:"compatible_status"`  // 固件兼容性 0=正常
	ActivationTime    int64        `json:"activation_time"`    // 激活时间
	WpmzVersion       string       `json:"wpmz_version"`       // 航线库版本
	Storage           DJIStorage   `json:"storage"`            // 存储空间
	ModeCode          int          `json:"mode_code"`          // 飞行模式码（大疆标准）
	FlightStatus      string       `json:"flight_status"`      // 飞行状态详情（仿真扩展字段）
	Battery           DJIBattery   `json:"battery"`            // 电池信息
	Payloads          []DJIPayload `json:"payloads"`           // 负载列表
	Cameras           []DJICamera  `json:"cameras"`            // 相机列表
	ObstacleAvoidance DJIObstacle  `json:"obstacle_avoidance"` // 避障状态
	RTHAltitude       int          `json:"rth_altitude"`       // 返航高度
	RTHMode           int          `json:"rth_mode"`           // 0=最优 1=预设
	RCLostAction      int          `json:"rc_lost_action"`     // 失控动作
	NightLightsState  int          `json:"night_lights_state"` // 夜航灯
	DongleInfos       []DJIDongle  `json:"dongle_infos"`       // 4G模块
	MaintainStatus    DJIMaintain  `json:"maintain_status"`    // 维护信息
}

// DJIStorage 存储信息
type DJIStorage struct {
	Total int `json:"total"` // KB
	Used  int `json:"used"`
}

// DJIPayload 负载信息
type DJIPayload struct {
	PayloadIndex    string `json:"payload_index"`
	Sn              string `json:"sn"`
	FirmwareVersion string `json:"firmware_version"`
	ControlSource   string `json:"control_source"`
}

// DJICamera 相机信息
type DJICamera struct {
	PayloadIndex         string  `json:"payload_index"`
	CameraMode           int     `json:"camera_mode"`     // 0=拍照 1=录像
	PhotoState           int     `json:"photo_state"`     // 0=空闲 1=拍照中
	RecordingState       int     `json:"recording_state"` // 0=空闲 1=录像中
	RemainPhotoNum       int     `json:"remain_photo_num"`
	RemainRecordDuration int     `json:"remain_record_duration"`
	ZoomFactor           float64 `json:"zoom_factor"`
}

// DJIObstacle 避障状态
type DJIObstacle struct {
	Horizon  int `json:"horizon"`  // 水平 0=关 1=开
	Upside   int `json:"upside"`   // 上方
	Downside int `json:"downside"` // 下方
}

// DJIDongle 4G 模块
type DJIDongle struct {
	Imei       string `json:"imei"`
	DongleType int    `json:"dongle_type"` // 6=旧 10=新(eSIM)
}

// DJIMaintain 维护信息
type DJIMaintain struct {
	MaintainStatusArray []interface{} `json:"maintain_status_array"`
}

// buildState 根据航线信息构建默认 state 数据
func buildState(droneID string) DJIStateData {
	return DJIStateData{
		FirmwareVersion:  "v10.01.0000",
		CompatibleStatus: 0,
		ActivationTime:   1721600000,
		WpmzVersion:      "1.0.2",
		Storage:          DJIStorage{Total: 1048576, Used: 262144},
		ModeCode:         0, // Standby
		Battery:          DJIBattery{CapacityPercent: 100, Voltage: 25.2, Temperature: 35},
		Payloads: []DJIPayload{
			{PayloadIndex: "52-0-0", Sn: droneID + "-cam", FirmwareVersion: "v04.00.0010", ControlSource: "A"},
		},
		Cameras: []DJICamera{
			{PayloadIndex: "52-0-0", CameraMode: 0, PhotoState: 0, RecordingState: 0, RemainPhotoNum: 500, RemainRecordDuration: 3600, ZoomFactor: 2},
		},
		ObstacleAvoidance: DJIObstacle{Horizon: 1, Upside: 1, Downside: 1},
		RTHAltitude:       100,
		RTHMode:           1,
		RCLostAction:      2, // RTH
		NightLightsState:  1,
		DongleInfos:       []DJIDongle{{Imei: "860123456789012", DongleType: 10}},
		MaintainStatus:    DJIMaintain{MaintainStatusArray: []interface{}{}},
	}
}

// modeCodeFromStatus 将内部状态映射为 DJI mode_code
// DJI Cloud API mode_code 定义:
//
//	0=Standby, 5=Wayline, 2=AutoLanding, 7=Failsafe/Emergency
func modeCodeFromStatus(status string) int {
	switch status {
	case "idle":
		return 0 // Standby
	case "running":
		return 5 // Wayline flight
	case "paused":
		return 0 // Standby (paused 视为待命)
	case "completed":
		return 0 // Standby
	case "hovering":
		return 5 // Wayline flight (悬停属于执行航线的一部分)
	case "emergency_stop":
		return 7 // Failsafe / Emergency motor stop
	case "emergency_landing":
		return 2 // Auto Landing (可控迫降)
	case "emergency_stopped":
		return 0 // Standby (已坠毁于地面)
	case "emergency_landed":
		return 0 // Standby (已迫降于地面)
	default:
		return 0
	}
}

// PublishDeviceOnline 无人机上线时发布 state
func (p *Publisher) PublishDeviceOnline(droneID string) {
	p.mu.RLock()
	if !p.connected {
		p.mu.RUnlock()
		return
	}
	p.mu.RUnlock()

	msg := DJIStateMessage{
		TID:       uuid.New().String(),
		BID:       uuid.New().String(),
		Timestamp: currentTimeMillis(),
		Gateway:   droneID,
		Data:      buildState(droneID),
	}
	p.publishRetained(fmt.Sprintf("thing/product/%s/state", droneID), msg)
}

// PublishModeChange 飞行模式变更时发布 state topic（大疆规范：通过 mode_code 上报）
func (p *Publisher) PublishModeChange(droneID, status string) {
	p.mu.RLock()
	if !p.connected {
		p.mu.RUnlock()
		return
	}
	p.mu.RUnlock()

	state := buildState(droneID)
	state.ModeCode = modeCodeFromStatus(status)
	state.FlightStatus = status // 仿真扩展：详细状态字符串

	msg := DJIStateMessage{
		TID:       uuid.New().String(),
		BID:       uuid.New().String(),
		Timestamp: currentTimeMillis(),
		Gateway:   droneID,
		Data:      state,
	}
	p.publishRetained(fmt.Sprintf("thing/product/%s/state", droneID), msg)
}

func currentTimeMillis() int64 {
	return time.Now().UnixMilli()
}
