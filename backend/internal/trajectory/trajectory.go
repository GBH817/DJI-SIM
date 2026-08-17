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

// MapDroneModel 根据 DJI 官方 Cloud API 枚举值映射飞行器型号名称
// 参考: https://developer.dji.com/doc/cloud-api-tutorial/cn/overview/product-support.html
func MapDroneModel(enumValue, subValue int) string {
	switch {
	case enumValue == 103 && subValue == 0:
		return "Matrice 400"
	case enumValue == 89 && subValue == 0:
		return "Matrice 350 RTK"
	case enumValue == 60 && subValue == 0:
		return "Matrice 300 RTK"
	case enumValue == 67 && subValue == 0:
		return "Matrice 30"
	case enumValue == 67 && subValue == 1:
		return "Matrice 30T"
	case enumValue == 77 && subValue == 0:
		return "Mavic 3E"
	case enumValue == 77 && subValue == 1:
		return "Mavic 3T"
	case enumValue == 77 && subValue == 2:
		return "Mavic 3M"
	case enumValue == 77 && subValue == 3:
		return "Mavic 3TA"
	case enumValue == 91 && subValue == 0:
		return "Matrice 3D"
	case enumValue == 91 && subValue == 1:
		return "Matrice 3TD"
	case enumValue == 99 && subValue == 0:
		return "Matrice 4E"
	case enumValue == 99 && subValue == 1:
		return "Matrice 4T"
	case enumValue == 100 && subValue == 0:
		return "Matrice 4D"
	case enumValue == 100 && subValue == 1:
		return "Matrice 4TD"
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

// GetModelSpec 根据 DJI 官方枚举值获取机型规格
func GetModelSpec(enumValue, subValue int) DroneModelSpec {
	const enterpriseWindResist = 12.0

	switch {
	case enumValue == 103: // Matrice 400: 55min, 23m/s
		return DroneModelSpec{"Matrice 400", 55 * 60, 55 * 60, 23, enterpriseWindResist}
	case enumValue == 89: // Matrice 350 RTK: 55min, 23m/s
		return DroneModelSpec{"Matrice 350 RTK", 55 * 60, 55 * 60, 23, enterpriseWindResist}
	case enumValue == 60: // Matrice 300 RTK: 55min, 23m/s
		return DroneModelSpec{"Matrice 300 RTK", 55 * 60, 55 * 60, 23, enterpriseWindResist}
	case enumValue == 67 && subValue == 0: // Matrice 30: 41min飞行/36min悬停, 23m/s
		return DroneModelSpec{"Matrice 30", 41 * 60, 36 * 60, 23, enterpriseWindResist}
	case enumValue == 67 && subValue == 1: // Matrice 30T
		return DroneModelSpec{"Matrice 30T", 41 * 60, 36 * 60, 23, enterpriseWindResist}
	case enumValue == 77 && subValue == 0: // Mavic 3E: 45min/38min, 21m/s
		return DroneModelSpec{"Mavic 3E", 45 * 60, 38 * 60, 21, enterpriseWindResist}
	case enumValue == 77 && subValue == 1: // Mavic 3T
		return DroneModelSpec{"Mavic 3T", 45 * 60, 38 * 60, 21, enterpriseWindResist}
	case enumValue == 77 && subValue == 2: // Mavic 3M: 43min/37min
		return DroneModelSpec{"Mavic 3M", 43 * 60, 37 * 60, 21, enterpriseWindResist}
	case enumValue == 77 && subValue == 3: // Mavic 3TA
		return DroneModelSpec{"Mavic 3TA", 45 * 60, 38 * 60, 21, enterpriseWindResist}
	case enumValue == 91: // Matrice 3D/3TD: 45min, 21m/s
		return DroneModelSpec{"Matrice 3D/Matrice 3TD", 45 * 60, 38 * 60, 21, enterpriseWindResist}
	case enumValue == 99: // Matrice 4E/4T: 49min/42min, 21m/s
		return DroneModelSpec{"Matrice 4E/Matrice 4T", 49 * 60, 42 * 60, 21, enterpriseWindResist}
	case enumValue == 100: // Matrice 4D/4TD: 49min, 21m/s
		return DroneModelSpec{"Matrice 4D/Matrice 4TD", 49 * 60, 42 * 60, 21, enterpriseWindResist}
	default:
		return DroneModelSpec{"Unknown", 30 * 60, 25 * 60, 15, 10}
	}
}

// MapPayloadModel 根据 DJI 官方 Cloud API 枚举值映射负载名称
// 参考: https://developer.dji.com/doc/cloud-api-tutorial/cn/overview/product-support.html
func MapPayloadModel(enumValue, subValue int) string {
	switch {
	// 禅思 H20/H20T/H20N/H30/H30T 系列
	case enumValue == 42:
		return "H20"
	case enumValue == 43:
		return "H20T"
	case enumValue == 61:
		return "H20N"
	case enumValue == 82:
		return "H30"
	case enumValue == 83:
		return "H30T"
	// Matrice 30/30T 一体机相机
	case enumValue == 52:
		return "M30 Camera"
	case enumValue == 53:
		return "M30T Camera"
	// Mavic 3 行业系列相机
	case enumValue == 66:
		return "M3E Camera"
	case enumValue == 67:
		return "M3T Camera"
	case enumValue == 129:
		return "M3TA Camera"
	// Matrice 3D/3TD 相机
	case enumValue == 80:
		return "M3D Camera"
	case enumValue == 81:
		return "M3TD Camera"
	// Matrice 4E/4T 相机
	case enumValue == 88:
		return "M4E Camera"
	case enumValue == 89:
		return "M4T Camera"
	// Matrice 4D/4TD 相机
	case enumValue == 98:
		return "M4D Camera"
	case enumValue == 99:
		return "M4TD Camera"
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
