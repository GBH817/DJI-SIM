package trajectory

import "time"

// WaypointAction 航点动作（对应 DJI WPML actionGroup）
type WaypointAction struct {
	Type   string                 `json:"type"`   // hover, takePhoto, startRecord, stopRecord, rotateYaw, gimbalRotate, zoom, focus
	Params map[string]interface{} `json:"params"` // 动作参数，如 hoverTime, aircraftHeading 等
}

// Waypoint 单个航点
type Waypoint struct {
	Index           int              `json:"index"`
	Longitude       float64          `json:"lng"`
	Latitude        float64          `json:"lat"`
	Height          float64          `json:"height"`          // EGM96海拔高度 (template.kml 的 height)
	EllipsoidHeight float64          `json:"ellipsoidHeight"` // 椭球高度 (waylines.wpml 的 executeHeight / template.kml 的 ellipsoidHeight)
	Speed           float64          `json:"speed"`           // m/s
	Heading         float64          `json:"heading"`         // 航向角
	Timestamp       time.Duration    `json:"timestamp"`       // 从起点起的累计时间
	Actions         []WaypointAction `json:"actions"`         // DJI WPML 动作列表
	TurnMode        string           `json:"turnMode"`        // 转弯模式: coordinateTurn / toPointAndStopWithDiscontinuityCurvature
	TurnDampingDist float64          `json:"turnDampingDist"` // 转弯阻尼距离 (m)
}

// HoverTime 获取航点的悬停时间（秒），无悬停动作返回 0
func (w *Waypoint) HoverTime() float64 {
	for _, a := range w.Actions {
		if a.Type == "hover" {
			if ht, ok := a.Params["hoverTime"]; ok {
				switch v := ht.(type) {
				case float64:
					return v
				case int:
					return float64(v)
				}
			}
		}
	}
	return 0
}

// Trajectory 航线轨迹
type Trajectory struct {
	ID                     string        `json:"id"`
	Name                   string        `json:"name"`
	Waypoints              []Waypoint    `json:"waypoints"`
	TakeOffPoint           [3]float64    `json:"takeOffPoint"`           // lat,lng,alt
	TotalDistance          float64       `json:"totalDistance"`          // 米
	TotalDuration          time.Duration `json:"totalDuration"`
	FinishAction           string        `json:"finishAction"`
	RTHHeight              float64       `json:"rthHeight"`
	GlobalHeight           float64       `json:"globalHeight"`
	AutoFlightSpeed        float64       `json:"autoFlightSpeed"`
	TakeOffSecurityHeight  float64       `json:"takeOffSecurityHeight"`  // 安全起飞高度 (m)
	GlobalTransitionalSpeed float64      `json:"globalTransitionalSpeed"` // 过渡爬升速度 (m/s)
	DroneEnumValue         int           `json:"droneEnumValue"`          // 飞行器机型枚举
	DroneSubEnumValue      int           `json:"droneSubEnumValue"`       // 飞行器子类型枚举
	DroneModelName         string        `json:"droneModelName"`          // 飞行器型号名称 (映射后)
	PayloadEnumValue       int           `json:"payloadEnumValue"`        // 负载枚举
	PayloadSubEnumValue    int           `json:"payloadSubEnumValue"`     // 负载子类型枚举
	PayloadModelName       string        `json:"payloadModelName"`        // 负载名称 (映射后)
	FlyToWaylineMode       string        `json:"flyToWaylineMode"`        // 飞向首航点模式: safely / pointToPoint
}

// DroneModelName 根据 DJI 枚举值映射飞行器型号名称
func MapDroneModel(enumValue, subValue int) string {
	switch {
	case enumValue == 67 && subValue == 0:
		return "Matrice 30"
	case enumValue == 67 && subValue == 1:
		return "Matrice 30T"
	case enumValue == 77 && subValue == 0:
		return "Matrice 300 RTK"
	case enumValue == 91 && subValue == 0:
		return "Matrice 350 RTK"
	case enumValue == 60 && subValue == 0:
		return "Mavic 3E"
	case enumValue == 60 && subValue == 1:
		return "Mavic 3T"
	case enumValue == 60 && subValue == 2:
		return "Mavic 3M"
	case enumValue == 81 && subValue == 0:
		return "Mavic 3D"
	case enumValue == 81 && subValue == 1:
		return "Mavic 3TD"
	case enumValue == 100 && subValue == 0:
		return "Matrice 4E"
	case enumValue == 100 && subValue == 1:
		return "Matrice 4T"
	default:
		return "Unknown"
	}
}

// DroneModelSpec 机型规格参数（基于大疆官方技术参数）
type DroneModelSpec struct {
	ModelName          string  // 型号名称
	MaxFlightTimeSec   float64 // 最大飞行时间（秒）
	MaxHoverTimeSec    float64 // 最大悬停时间（秒）
	MaxHorizontalSpeed float64 // 最大水平速度 (m/s)
	MaxWindResistance  float64 // 最大抗风速度 (m/s)，基于大疆官方规格
}

// GetModelSpec 根据 DJI 枚举值获取机型规格
func GetModelSpec(enumValue, subValue int) DroneModelSpec {
	// 基于 DJI 官方技术参数：
	// M350 RTK: 55min 飞行 / 测于 8m/s 无风空载
	// M300 RTK: 55min 飞行
	// M30/M30T: 41min 飞行 / 36min 悬停
	// Mavic 3E/3T: 45min 飞行 / 38min 悬停
	// Mavic 3M: 43min 飞行 / 37min 悬停
	// Mavic 3D/3TD: 45min 飞行
	// Matrice 4E/4T: 49min 飞行 / 42min 悬停
	// 所有企业级无人机的抗风能力均为 12 m/s（大疆官方规格）
	const enterpriseWindResist = 12.0

	switch {
	case enumValue == 91: // Matrice 350 RTK
		return DroneModelSpec{"Matrice 350 RTK", 55 * 60, 55 * 60, 23, enterpriseWindResist}
	case enumValue == 77: // Matrice 300 RTK
		return DroneModelSpec{"Matrice 300 RTK", 55 * 60, 55 * 60, 23, enterpriseWindResist}
	case enumValue == 67 && subValue == 0: // Matrice 30
		return DroneModelSpec{"Matrice 30", 41 * 60, 36 * 60, 23, enterpriseWindResist}
	case enumValue == 67 && subValue == 1: // Matrice 30T
		return DroneModelSpec{"Matrice 30T", 41 * 60, 36 * 60, 23, enterpriseWindResist}
	case enumValue == 60 && subValue == 0: // Mavic 3E
		return DroneModelSpec{"Mavic 3E", 45 * 60, 38 * 60, 21, enterpriseWindResist}
	case enumValue == 60 && subValue == 1: // Mavic 3T
		return DroneModelSpec{"Mavic 3T", 45 * 60, 38 * 60, 21, enterpriseWindResist}
	case enumValue == 60 && subValue == 2: // Mavic 3M
		return DroneModelSpec{"Mavic 3M", 43 * 60, 37 * 60, 21, enterpriseWindResist}
	case enumValue == 81: // Mavic 3D/3TD
		return DroneModelSpec{"Mavic 3D/Mavic 3TD", 45 * 60, 38 * 60, 21, enterpriseWindResist}
	case enumValue == 100: // Matrice 4E/4T
		return DroneModelSpec{"Matrice 4E/Matrice 4T", 49 * 60, 42 * 60, 21, enterpriseWindResist}
	default:
		return DroneModelSpec{"Unknown", 30 * 60, 25 * 60, 15, 10}
	}
}

// MapPayloadModel 根据 DJI 枚举值映射负载名称
func MapPayloadModel(enumValue, subValue int) string {
	switch {
	case enumValue == 66 && subValue == 0:
		return "H20"
	case enumValue == 66 && subValue == 1:
		return "H20T"
	case enumValue == 52 && subValue == 0:
		return "M30 Camera"
	case enumValue == 53 && subValue == 0:
		return "M30T Camera"
	case enumValue == 76 && subValue == 0:
		return "M3E Camera"
	case enumValue == 77 && subValue == 0:
		return "M3T Camera"
	case enumValue == 78 && subValue == 0:
		return "M3M Camera"
	case enumValue == 85 && subValue == 0:
		return "M3D Camera"
	case enumValue == 86 && subValue == 1:
		return "M3TD Wide Camera"
	case enumValue == 86 && subValue == 2:
		return "M3TD Tele Camera"
	case enumValue == 97 && subValue == 0:
		return "M4E Camera"
	case enumValue == 98 && subValue == 1:
		return "M4T Wide Camera"
	case enumValue == 98 && subValue == 2:
		return "M4T Tele Camera"
	case enumValue == 99 && subValue == 2:
		return "M4T Hybrid"
	default:
		return "Unknown"
	}
}

// RemoteIDTelemetry Remote ID 遥测数据
type RemoteIDTelemetry struct {
	DroneID            string            `json:"droneId"`
	Latitude           float64           `json:"lat"`
	Longitude          float64           `json:"lng"`
	Altitude           float64           `json:"alt"`               // 椭球高度
	HeightAboveTakeoff float64           `json:"heightAboveTakeoff"` // 相对起飞点高度
	Speed              float64           `json:"speed"`             // m/s 水平对地速度
	VerticalSpeed      float64           `json:"verticalSpeed"`     // m/s 垂直速度（正=爬升，负=下降）
	Heading            float64           `json:"heading"`           // 度，0-360
	Status             string            `json:"status"`            // "idle"|"running"|"paused"|"completed"|"hovering"
	Timestamp          int64             `json:"timestamp"`         // unix timestamp ms
	WaypointIndex      int               `json:"waypointIndex"`     // 当前航点索引
	TotalWaypoints     int               `json:"totalWaypoints"`    // 总航点数
	Progress           float64           `json:"progress"`          // 0-100
	CurrentAction      string            `json:"currentAction"`     // 当前执行的动作类型（hover/takePhoto等），无动作时为空
	WaypointActions    []WaypointAction  `json:"waypointActions"`   // 当前航点的动作列表
	HoverTimeRemaining float64           `json:"hoverTimeRemaining"` // 悬停剩余时间（秒）
	BatteryPercent     float64           `json:"batteryPercent"`    // 电池电量百分比 (0-100)，基于机型续航模拟
	WindSpeed          float64           `json:"windSpeed"`         // 当前风速 (m/s)
	WindDirection      float64           `json:"windDirection"`     // 风向 (度, 0=北, 90=东, 气象方向: 风来的方向)
	WindWarning        bool              `json:"windWarning"`       // 是否超过抗风限制
	RunGeneration      int64             `json:"runGeneration"`     // 仿真轮次，每次Start递增，用于前端区分新旧遥测数据
}
