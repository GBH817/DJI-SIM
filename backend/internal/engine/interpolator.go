package engine

import (
	"math"
	"time"

	"drone-sim-backend/internal/trajectory"
)

// InterpolatedPoint 插值后的飞行状态点
type InterpolatedPoint struct {
	Longitude     float64
	Latitude      float64
	Altitude      float64 // 椭球高度
	Height        float64 // EGM96 海拔高度
	Speed         float64
	Heading       float64
	WaypointIndex int
	Timestamp     time.Duration // 从起点累计的时间
}

// Segment 航段
type Segment struct {
	FromIndex     int
	ToIndex       int
	Distance      float64 // 米
	Duration      time.Duration
	Speed         float64
	VerticalSpeed float64 // 垂直速度 (m/s)，正值=爬升，负值=下降
}

// calculateDistance 使用 Haversine 公式计算两点间距离（米）
func calculateDistance(lat1, lng1, lat2, lng2 float64) float64 {
	const earthRadius = 6371000.0
	dLat := (lat2 - lat1) * math.Pi / 180.0
	dLng := (lng2 - lng1) * math.Pi / 180.0
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180.0)*math.Cos(lat2*math.Pi/180.0)*
			math.Sin(dLng/2)*math.Sin(dLng/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadius * c
}

// calculateBearing 计算从点1到点2的方位角（度）
func calculateBearing(lat1, lng1, lat2, lng2 float64) float64 {
	dLng := (lng2 - lng1) * math.Pi / 180.0
	lat1Rad := lat1 * math.Pi / 180.0
	lat2Rad := lat2 * math.Pi / 180.0
	y := math.Sin(dLng) * math.Cos(lat2Rad)
	x := math.Cos(lat1Rad)*math.Sin(lat2Rad) - math.Sin(lat1Rad)*math.Cos(lat2Rad)*math.Cos(dLng)
	bearing := math.Atan2(y, x) * 180.0 / math.Pi
	return math.Mod(bearing+360, 360)
}

// destinationPoint 从起点出发，沿方位角行进 distance 米后的目标点
func destinationPoint(lat, lng, distance, bearingDeg float64) (float64, float64) {
	const earthRadius = 6371000.0
	bearingRad := bearingDeg * math.Pi / 180.0
	latRad := lat * math.Pi / 180.0
	angularDist := distance / earthRadius

	destLatRad := math.Asin(math.Sin(latRad)*math.Cos(angularDist) +
		math.Cos(latRad)*math.Sin(angularDist)*math.Cos(bearingRad))
	destLngRad := lng*math.Pi/180.0 + math.Atan2(
		math.Sin(bearingRad)*math.Sin(angularDist)*math.Cos(latRad),
		math.Cos(angularDist)-math.Sin(latRad)*math.Sin(destLatRad))

	return destLatRad * 180.0 / math.Pi, destLngRad * 180.0 / math.Pi
}

// BuildSegments 根据航点构建航段列表，计算每个航段的距离和时间
func BuildSegments(waypoints []trajectory.Waypoint) []Segment {
	if len(waypoints) < 2 {
		return nil
	}

	segments := make([]Segment, 0, len(waypoints)-1)
	for i := 0; i < len(waypoints)-1; i++ {
		wp1 := waypoints[i]
		wp2 := waypoints[i+1]

		horizontalDist := calculateDistance(wp1.Latitude, wp1.Longitude, wp2.Latitude, wp2.Longitude)
		verticalDist := math.Abs(wp2.EllipsoidHeight - wp1.EllipsoidHeight)
		dist := math.Sqrt(horizontalDist*horizontalDist + verticalDist*verticalDist)

		speed := (wp1.Speed + wp2.Speed) / 2.0
		if speed <= 0 {
			speed = 1.0 // 防止除零，默认 1 m/s
		}

		duration := time.Duration(float64(time.Second) * dist / speed)
		durationSec := dist / speed

		// 垂直速度（正值=爬升，负值=下降）
		verticalSpeed := 0.0
		if durationSec > 0 {
			verticalSpeed = (wp2.EllipsoidHeight - wp1.EllipsoidHeight) / durationSec
		}

		segments = append(segments, Segment{
			FromIndex:     i,
			ToIndex:       i + 1,
			Distance:      dist,
			Duration:      duration,
			Speed:         speed,
			VerticalSpeed: verticalSpeed,
		})
	}

	return segments
}

// lerp 线性插值
func lerp(a, b, t float64) float64 {
	return a + (b-a)*t
}

// lerpAngle 角度线性插值，处理 0-360 环绕
func lerpAngle(a, b, t float64) float64 {
	diff := math.Mod(b-a+540, 360) - 180
	return math.Mod(a+diff*t+360, 360)
}

// WindDriftResult 风力偏移计算结果
type WindDriftResult struct {
	DriftLat    float64 // 本 tick 纬度漂移增量 (度)
	DriftLng    float64 // 本 tick 经度漂移增量 (度)
	GroundSpeed float64 // 修正后的对地速度 (m/s)
	Warning     bool    // 是否超过抗风限制
	PowerFactor float64 // 风力对耗电的影响系数 (1.0=无影响, >1=更耗电)
}

// ComputeWindDrift 计算本 tick 的风力偏移增量
// windSpeed: 风速 (m/s)
// windDirection: 风向 (度, 0=北, 90=东, 气象方向: 风来的方向)
// maxWindResistance: 机型最大抗风速度 (m/s)
// heading: 无人机当前航向 (度)
// airspeed: 无人机空速 (m/s)
// dt: 时间步长 (秒)
// 当 windSpeed <= maxWindResistance 时返回零漂移
func ComputeWindDrift(windSpeed, windDirection, maxWindResistance, heading, airspeed, dt float64) WindDriftResult {
	result := WindDriftResult{
		PowerFactor: 1.0,
	}

	// 无风或风速在抗风范围内，无人机完全补偿
	if windSpeed <= 0 || windSpeed <= maxWindResistance {
		result.GroundSpeed = airspeed
		return result
	}

	// 超出抗风限制
	result.Warning = true
	excess := windSpeed - maxWindResistance

	// 风去向 (风推动无人机的方向)
	windToDeg := math.Mod(windDirection+180, 360)
	windToRad := windToDeg * math.Pi / 180.0
	headingRad := heading * math.Pi / 180.0

	// 风去向的速度分量 (m/s): [东, 北]
	windEast := excess * math.Sin(windToRad)
	windNorth := excess * math.Cos(windToRad)

	// 无人机空速分量 (沿航向)
	droneEast := airspeed * math.Sin(headingRad)
	droneNorth := airspeed * math.Cos(headingRad)

	// 对地速度 = 空速 + 超额风速
	groundEast := droneEast + windEast
	groundNorth := droneNorth + windNorth
	groundSpeed := math.Sqrt(groundEast*groundEast + groundNorth*groundNorth)
	if groundSpeed < 0 {
		groundSpeed = 0
	}
	result.GroundSpeed = groundSpeed

	// 将漂移距离转换为经纬度偏移（小距离近似）
	const metersPerDegLat = 111320.0
	latRad := 0.0 // 使用赤道近似，精度足够
	metersPerDegLng := metersPerDegLat * math.Cos(latRad)

	// 风去向的经纬位移: 东分量 → 经度, 北分量 → 纬度
	driftEast := excess * math.Sin(windToRad) * dt
	driftNorth := excess * math.Cos(windToRad) * dt

	result.DriftLat = driftNorth / metersPerDegLat
	result.DriftLng = driftEast / metersPerDegLng

	// 超限风力下耗电增加：逆风分量越大，耗电越多
	// 逆风分量 (正值为逆风): 风去向与航向相反时为逆风
	headwindFactor := -(math.Cos(windToRad)*math.Cos(headingRad) + math.Sin(windToRad)*math.Sin(headingRad))
	if headwindFactor > 0 {
		result.PowerFactor = 1.0 + headwindFactor*(excess/airspeed)*2.0 // 超限逆风时显著增加耗电
	}

	return result
}

// Interpolate 线性插值：给定航点和已过时间，计算当前位置
// 航向角根据实际飞行方向（两点间的方位角）计算，而非 KMZ 文件中存储的值
func Interpolate(waypoints []trajectory.Waypoint, segments []Segment, elapsed time.Duration) InterpolatedPoint {
	n := len(waypoints)
	if n == 0 {
		return InterpolatedPoint{}
	}

	// 计算分段航向角
	segmentBearing := make([]float64, len(segments))
	for i, seg := range segments {
		wp1 := waypoints[seg.FromIndex]
		wp2 := waypoints[seg.ToIndex]
		segmentBearing[i] = calculateBearing(wp1.Latitude, wp1.Longitude, wp2.Latitude, wp2.Longitude)
	}

	if n == 1 || elapsed <= 0 {
		wp := waypoints[0]
		heading := wp.Heading
		if heading == 0 && len(segments) > 0 {
			heading = segmentBearing[0]
		}
		return InterpolatedPoint{
			Longitude:     wp.Longitude,
			Latitude:      wp.Latitude,
			Altitude:      wp.EllipsoidHeight,
			Height:        wp.Height,
			Speed:         wp.Speed,
			Heading:       heading,
			WaypointIndex: 0,
			Timestamp:     elapsed,
		}
	}

	// 计算总时长
	var totalDuration time.Duration
	for _, seg := range segments {
		totalDuration += seg.Duration
	}

	// 如果已过总时长，返回终点
	if elapsed >= totalDuration {
		wp := waypoints[n-1]
		heading := wp.Heading
		if heading == 0 && len(segments) > 0 {
			heading = segmentBearing[len(segmentBearing)-1]
		}
		return InterpolatedPoint{
			Longitude:     wp.Longitude,
			Latitude:      wp.Latitude,
			Altitude:      wp.EllipsoidHeight,
			Height:        wp.Height,
			Speed:         wp.Speed,
			Heading:       heading,
			WaypointIndex: n - 1,
			Timestamp:     elapsed,
		}
	}

	// 找到 elapsed 落在哪个航段
	remaining := elapsed
	for i, seg := range segments {
		if remaining < seg.Duration {
			// 落在当前航段
			t := float64(remaining) / float64(seg.Duration)
			if t > 1.0 {
				t = 1.0
			}

			wp1 := waypoints[seg.FromIndex]
			wp2 := waypoints[seg.ToIndex]

			return InterpolatedPoint{
				Longitude:     lerp(wp1.Longitude, wp2.Longitude, t),
				Latitude:      lerp(wp1.Latitude, wp2.Latitude, t),
				Altitude:      lerp(wp1.EllipsoidHeight, wp2.EllipsoidHeight, t),
				Height:        lerp(wp1.Height, wp2.Height, t),
				Speed:         lerp(wp1.Speed, wp2.Speed, t),
				Heading:       segmentBearing[i], // 使用实际飞行方向
				WaypointIndex: seg.FromIndex,
				Timestamp:     elapsed,
			}
		}
		remaining -= seg.Duration
	}

	// 理论上不会走到这里，兜底返回终点
	wp := waypoints[n-1]
	return InterpolatedPoint{
		Longitude:     wp.Longitude,
		Latitude:      wp.Latitude,
		Altitude:      wp.EllipsoidHeight,
		Height:        wp.Height,
		Speed:         wp.Speed,
		Heading:       wp.Heading,
		WaypointIndex: n - 1,
		Timestamp:     elapsed,
	}
}

// CurrentVerticalSpeed 根据已过时间查找当前所在航段的垂直速度
// 返回垂直速度 m/s（正值=爬升，负值=下降）
func CurrentVerticalSpeed(segments []Segment, elapsed time.Duration) float64 {
	remaining := elapsed
	for _, seg := range segments {
		if remaining < seg.Duration {
			return seg.VerticalSpeed
		}
		remaining -= seg.Duration
	}
	// 超出所有航段（飞行结束），返回 0
	return 0
}
