package engine

import (
	"fmt"
	"math"
	"sync"
	"time"

	"drone-sim-backend/internal/trajectory"
)

// DroneState 无人机实时状态
type DroneState struct {
	DroneID         string
	Name            string
	Trajectory      *trajectory.Trajectory
	Segments        []Segment
	StartTime       time.Time
	PausedAt        time.Duration
	Elapsed         time.Duration
	Status          string  // "idle", "running", "paused", "completed", "hovering"
	SpeedMultiplier float64 // 仿真速度倍率，默认 1

	// 仿真用航点（可能包含起飞点作为首航点）
	SimWaypoints         []trajectory.Waypoint
	OriginalSimWaypoints []trajectory.Waypoint // 原始航线备份，ReturnHome 替换后恢复用
	OriginalSegments     []Segment             // 原始航段备份

	// 悬停相关
	InHover           bool
	HoverStartTime    time.Time
	HoverDuration     time.Duration
	HoverWaypointIdx  int
	LastWaypointIndex int

	// 电池模拟
	ModelSpec    trajectory.DroneModelSpec // 机型规格
	CumFlightSec float64                   // 累计仿真飞行秒数（不含悬停）
	CumHoverSec  float64                   // 累计仿真悬停秒数
	lastTickTime time.Time                 // 上次 tick 时间（用于计算增量）

	// 风力模拟
	WindSpeed     float64 // 当前风速 (m/s)
	WindDirection float64 // 风向 (度, 0=北, 气象方向: 风来的方向)
	CumDriftLat   float64 // 累计纬度漂移 (度)
	CumDriftLng   float64 // 累计经度漂移 (度)
	WindWarning   bool    // 当前是否超过抗风限制

	// 仿真轮次
	RunGeneration int64 // 每次 Start / ReturnHome 递增，前端用于区分新旧遥测

	Ticker   *time.Ticker
	StopCh   chan struct{}
	PauseCh  chan struct{}
	ResumeCh chan struct{}
	mu       sync.Mutex
}

// TelemetryCallback 遥测回调函数类型
type TelemetryCallback func(telemetry trajectory.RemoteIDTelemetry)

// ModeChangeCallback 飞行模式变更回调（用于发布 state topic）
type ModeChangeCallback func(droneID, status string)

// Engine 仿真引擎管理器
type Engine struct {
	drones       map[string]*DroneState
	mu           sync.RWMutex
	onTelemetry  TelemetryCallback
	onModeChange ModeChangeCallback
}

// NewEngine 创建仿真引擎
func NewEngine(teleCb TelemetryCallback, modeCb ModeChangeCallback) *Engine {
	return &Engine{
		drones:       make(map[string]*DroneState),
		onTelemetry:  teleCb,
		onModeChange: modeCb,
	}
}

// AddDrone 添加无人机仿真实例
func (e *Engine) AddDrone(traj *trajectory.Trajectory) string {
	e.mu.Lock()
	defer e.mu.Unlock()

	droneID := traj.ID
	if droneID == "" {
		droneID = fmt.Sprintf("drone-%d", time.Now().UnixNano())
	}

	// 构建仿真航点：如果有起飞点且与首个航点不同，根据 flyToWaylineMode 插入起飞段
	waypoints := traj.Waypoints
	takeoffLat, takeoffLng, takeoffAlt := traj.TakeOffPoint[0], traj.TakeOffPoint[1], traj.TakeOffPoint[2]
	if (takeoffLat != 0 || takeoffLng != 0) && len(waypoints) > 0 {
		firstWp := waypoints[0]
		if absDiff(takeoffLat, firstWp.Latitude) > 0.00001 || absDiff(takeoffLng, firstWp.Longitude) > 0.00001 {
			climbSpeed := traj.GlobalTransitionalSpeed
			if climbSpeed <= 0 {
				climbSpeed = traj.AutoFlightSpeed
			}

			var takeoffWps []trajectory.Waypoint

			switch traj.FlyToWaylineMode {
			case "safely":
				// 安全模式：垂直爬升 → 平飞 → 下降
				safeHeight := traj.TakeOffSecurityHeight
				targetHeight := firstWp.EllipsoidHeight
				if safeHeight <= 0 {
					safeHeight = targetHeight
				}
				climbToHeight := math.Max(safeHeight, targetHeight)

				// 0: 起飞点
				takeoffWps = append(takeoffWps, trajectory.Waypoint{
					Index:           -1,
					Latitude:        takeoffLat,
					Longitude:       takeoffLng,
					EllipsoidHeight: takeoffAlt,
					Height:          takeoffAlt,
					Speed:           climbSpeed,
					Heading:         0,
				})

				// 1: 垂直爬升到安全高度
				if climbToHeight > takeoffAlt+0.5 {
					takeoffWps = append(takeoffWps, trajectory.Waypoint{
						Index:           -1,
						Latitude:        takeoffLat,
						Longitude:       takeoffLng,
						EllipsoidHeight: climbToHeight,
						Height:          climbToHeight,
						Speed:           climbSpeed,
						Heading:         0,
					})
				}

				// 2: 平飞到首航点正上方
				takeoffWps = append(takeoffWps, trajectory.Waypoint{
					Index:           -1,
					Latitude:        firstWp.Latitude,
					Longitude:       firstWp.Longitude,
					EllipsoidHeight: climbToHeight,
					Height:          climbToHeight,
					Speed:           climbSpeed,
					Heading:         0,
				})

				// 3: 下降到首航点高度（如果需要）
				if math.Abs(climbToHeight-targetHeight) > 0.5 {
					takeoffWps = append(takeoffWps, trajectory.Waypoint{
						Index:           -1,
						Latitude:        firstWp.Latitude,
						Longitude:       firstWp.Longitude,
						EllipsoidHeight: targetHeight,
						Height:          firstWp.Height,
						Speed:           climbSpeed,
						Heading:         0,
					})
				}

			default: // pointToPoint 或未设置
				// 直线飞向首航点
				takeoffWps = append(takeoffWps, trajectory.Waypoint{
					Index:           -1,
					Latitude:        takeoffLat,
					Longitude:       takeoffLng,
					EllipsoidHeight: takeoffAlt,
					Height:          takeoffAlt,
					Speed:           climbSpeed,
					Heading:         0,
				})
			}

			waypoints = append(takeoffWps, waypoints...)
		}
	}

	// 如果结束动作为 goHome，追加返航航点
	if traj.FinishAction == "goHome" && len(waypoints) > 0 {
		takeoffLat, takeoffLng, takeoffAlt := traj.TakeOffPoint[0], traj.TakeOffPoint[1], traj.TakeOffPoint[2]
		if takeoffLat != 0 || takeoffLng != 0 {
			lastWp := waypoints[len(waypoints)-1]
			rthSpeed := traj.GlobalTransitionalSpeed
			if rthSpeed <= 0 {
				rthSpeed = traj.AutoFlightSpeed
			}
			if rthSpeed <= 0 {
				rthSpeed = 7
			}

			rthHeightVal := traj.RTHHeight
			if rthHeightVal <= 0 {
				rthHeightVal = lastWp.EllipsoidHeight
			}
			rthHeight := math.Max(rthHeightVal, lastWp.EllipsoidHeight)

			// 1. 从末航点垂直爬升到返航高度
			if rthHeight > lastWp.EllipsoidHeight+0.5 {
				waypoints = append(waypoints, trajectory.Waypoint{
					Index:           -2,
					Latitude:        lastWp.Latitude,
					Longitude:       lastWp.Longitude,
					EllipsoidHeight: rthHeight,
					Height:          rthHeight,
					Speed:           rthSpeed,
					Heading:         0,
				})
			}

			// 2. 平飞到起飞点正上方
			waypoints = append(waypoints, trajectory.Waypoint{
				Index:           -2,
				Latitude:        takeoffLat,
				Longitude:       takeoffLng,
				EllipsoidHeight: rthHeight,
				Height:          rthHeight,
				Speed:           rthSpeed,
				Heading:         0,
			})

			// 3. 下降到起飞点高度
			if math.Abs(rthHeight-takeoffAlt) > 0.5 {
				waypoints = append(waypoints, trajectory.Waypoint{
					Index:           -2,
					Latitude:        takeoffLat,
					Longitude:       takeoffLng,
					EllipsoidHeight: takeoffAlt,
					Height:          takeoffAlt,
					Speed:           rthSpeed,
					Heading:         0,
				})
			}
		}
	}

	// 展开 coordinateTurn 转弯点为圆弧航点
	waypoints = expandCoordinateTurns(waypoints)

	segments := BuildSegments(waypoints)

	spec := trajectory.GetModelSpec(traj.DroneEnumValue, traj.DroneSubEnumValue)

	ds := &DroneState{
		DroneID:              droneID,
		Name:                 traj.Name,
		Trajectory:           traj,
		Segments:             segments,
		SimWaypoints:         waypoints,
		OriginalSimWaypoints: waypoints,
		OriginalSegments:     segments,
		Status:               "idle",
		SpeedMultiplier:      1,
		LastWaypointIndex:    -1,
		ModelSpec:            spec,
		CumFlightSec:         0,
		CumHoverSec:          0,
		StopCh:               make(chan struct{}),
		PauseCh:              make(chan struct{}),
		ResumeCh:             make(chan struct{}),
	}

	e.drones[droneID] = ds
	return droneID
}

func absDiff(a, b float64) float64 {
	d := a - b
	if d < 0 {
		return -d
	}
	return d
}

// expandCoordinateTurns 将 coordinateTurn 航点展开为贝塞尔圆弧过渡点
// 对于 toPointAndStopWithDiscontinuityCurvature 的航点保持不变
func expandCoordinateTurns(waypoints []trajectory.Waypoint) []trajectory.Waypoint {
	if len(waypoints) < 3 {
		return waypoints
	}

	const arcPoints = 5 // 每个弯生成的圆弧点数

	var result []trajectory.Waypoint
	for i := 0; i < len(waypoints); i++ {
		wp := waypoints[i]

		// 只有 coordinateTurn 且不是首尾航点才展开圆弧
		if wp.TurnMode != "coordinateTurn" || i == 0 || i == len(waypoints)-1 {
			result = append(result, wp)
			continue
		}

		prevWp := waypoints[i-1]
		nextWp := waypoints[i+1]

		// 阻尼距离不超过相邻段距离的一半
		maxD := math.Min(
			calculateDistance(prevWp.Latitude, prevWp.Longitude, wp.Latitude, wp.Longitude),
			calculateDistance(wp.Latitude, wp.Longitude, nextWp.Latitude, nextWp.Longitude),
		) / 2.0
		d := wp.TurnDampingDist
		if d <= 0 || d > maxD {
			d = maxD
		}
		if d < 1 {
			// 阻尼距离太小，不展开
			result = append(result, wp)
			continue
		}

		// 计算切入点和切出点
		bearingIn := calculateBearing(wp.Latitude, wp.Longitude, prevWp.Latitude, prevWp.Longitude)
		bearingOut := calculateBearing(wp.Latitude, wp.Longitude, nextWp.Latitude, nextWp.Longitude)

		aLat, aLng := destinationPoint(wp.Latitude, wp.Longitude, d, bearingIn)  // 切入点
		cLat, cLng := destinationPoint(wp.Latitude, wp.Longitude, d, bearingOut) // 切出点

		// 高度线性插值
		aAlt := wp.EllipsoidHeight + (prevWp.EllipsoidHeight-wp.EllipsoidHeight)*(d/calculateDistance(prevWp.Latitude, prevWp.Longitude, wp.Latitude, wp.Longitude))
		cAlt := wp.EllipsoidHeight + (nextWp.EllipsoidHeight-wp.EllipsoidHeight)*(d/calculateDistance(wp.Latitude, wp.Longitude, nextWp.Latitude, nextWp.Longitude))

		// 用二次贝塞尔曲线生成圆弧过渡点：控制点为 wp 本身
		for j := 1; j <= arcPoints; j++ {
			t := float64(j) / float64(arcPoints+1)
			lat := (1-t)*(1-t)*aLat + 2*(1-t)*t*wp.Latitude + t*t*cLat
			lng := (1-t)*(1-t)*aLng + 2*(1-t)*t*wp.Longitude + t*t*cLng
			alt := (1-t)*(1-t)*aAlt + 2*(1-t)*t*wp.EllipsoidHeight + t*t*cAlt

			arcWp := trajectory.Waypoint{
				Index:           -3, // 标记为圆弧过渡点
				Latitude:        lat,
				Longitude:       lng,
				EllipsoidHeight: alt,
				Height:          alt,
				Speed:           wp.Speed,
				Heading:         0,
			}

			// 中间弧点（j=3, t=0.5）最接近原航点，转移动作到此
			if j == 3 {
				arcWp.Index = wp.Index
				arcWp.Actions = wp.Actions
				arcWp.Heading = wp.Heading
			}

			result = append(result, arcWp)
		}
		// 不再追加原始航点（圆弧已替代转角，追加会导致倒退飞行）
	}

	return result
}

// Start 启动仿真
func (e *Engine) Start(droneID string) error {
	e.mu.Lock()
	ds, ok := e.drones[droneID]
	if !ok {
		e.mu.Unlock()
		return fmt.Errorf("drone %s not found", droneID)
	}

	if ds.Status == "running" || ds.Status == "hovering" {
		e.mu.Unlock()
		return fmt.Errorf("drone %s is already running", droneID)
	}

	// 如果是从 paused 恢复，调整起始时间
	if ds.Status == "paused" {
		ds.StartTime = time.Now().Add(-ds.PausedAt)
	} else {
		ds.StartTime = time.Now()
		ds.Elapsed = 0
		ds.LastWaypointIndex = -1
		ds.CumFlightSec = 0 // 重新开始飞行，重置累计时间
		ds.CumHoverSec = 0
		ds.CumDriftLat = 0 // 清除上次飞行的风力漂移
		ds.CumDriftLng = 0

		// 恢复原始航线（ReturnHome 可能替换了 SimWaypoints）
		if ds.OriginalSimWaypoints != nil {
			ds.SimWaypoints = ds.OriginalSimWaypoints
			ds.Segments = ds.OriginalSegments
		}
	}

	ds.RunGeneration++ // 递增仿真轮次
	ds.Status = "running"
	ds.InHover = false
	ds.Ticker = time.NewTicker(100 * time.Millisecond)

	// 重新创建 channels（旧的可能在之前被关闭了）
	ds.StopCh = make(chan struct{})
	ds.PauseCh = make(chan struct{})
	ds.ResumeCh = make(chan struct{})

	e.mu.Unlock()

	// 启动仿真循环
	go e.runSimulationLoop(ds)

	return nil
}

// runSimulationLoop 仿真主循环
func (e *Engine) runSimulationLoop(ds *DroneState) {
	ticker := ds.Ticker
	stopCh := ds.StopCh
	pauseCh := ds.PauseCh
	resumeCh := ds.ResumeCh
	waypoints := ds.SimWaypoints

	// 初始模式变更：进入 running
	if e.onModeChange != nil {
		e.onModeChange(ds.DroneID, "running")
	}

	// 计算总飞行时长（不含悬停）
	var totalFlightDuration time.Duration
	for _, seg := range ds.Segments {
		totalFlightDuration += seg.Duration
	}

	// 计算总悬停时间（以数组索引为 key）
	var totalHoverDuration time.Duration
	hoverTimes := make(map[int]time.Duration)
	for i, wp := range waypoints {
		if ht := wp.HoverTime(); ht > 0 {
			d := time.Duration(ht * float64(time.Second))
			hoverTimes[i] = d
			totalHoverDuration += d
		}
	}

	// 总时长（飞行 + 悬停），用于进度计算
	totalDuration := totalFlightDuration + totalHoverDuration

	for {
		select {
		case <-ticker.C:
			ds.mu.Lock()

			// --- 悬停阶段 ---
			if ds.InHover {
				if ds.Status == "running" {
					ds.Status = "hovering"
				}

				hoverElapsed := time.Since(ds.HoverStartTime)

				// 累计悬停时间用于电池模拟
				ds.CumHoverSec += 0.1 * ds.SpeedMultiplier

				if hoverElapsed >= ds.HoverDuration {
					// 悬停结束，恢复飞行
					ds.InHover = false
					ds.Status = "running"
					// 调整起始时间，补偿悬停耗时
					ds.StartTime = ds.StartTime.Add(ds.HoverDuration)

					if e.onModeChange != nil {
						e.onModeChange(ds.DroneID, "running")
					}
					ds.mu.Unlock()
					continue
				}

				// 仍在悬停，发送速度为0的遥测
				wp := waypoints[ds.HoverWaypointIdx]
				stablePt := InterpolatedPoint{
					Longitude:     wp.Longitude,
					Latitude:      wp.Latitude,
					Altitude:      wp.EllipsoidHeight,
					Height:        wp.Height,
					Speed:         0,
					Heading:       wp.Heading,
					WaypointIndex: wp.Index,
					Timestamp:     ds.Elapsed,
				}
				telemetry := e.buildTelemetry(ds, stablePt)
				telemetry.Status = "hovering"
				telemetry.CurrentAction = "hover"

				// 填充当前动作列表
				if len(wp.Actions) > 0 {
					telemetry.WaypointActions = wp.Actions
				}
				telemetry.HoverTimeRemaining = ds.HoverDuration.Seconds() - hoverElapsed.Seconds()

				// 进度计算（飞行+已悬停比例）
				completedHover := totalFlightDuration + hoverElapsed
				// 累加之前航点的悬停时间
				for i := 0; i < wp.Index && i < len(waypoints); i++ {
					if ht, ok := hoverTimes[i]; ok {
						completedHover += ht
					}
				}
				if totalDuration > 0 {
					telemetry.Progress = float64(completedHover) / float64(totalDuration) * 100
				}

				ds.mu.Unlock()

				if e.onTelemetry != nil {
					e.onTelemetry(telemetry)
				}
				continue
			}

			if ds.Status != "running" {
				ds.mu.Unlock()
				continue
			}

			// --- 正常飞行阶段 ---
			elapsed := time.Duration(float64(time.Since(ds.StartTime)) * ds.SpeedMultiplier)
			if elapsed < 0 {
				elapsed = 0
			}
			ds.Elapsed = elapsed

			// 累计飞行时间用于电池模拟（每个 tick = 100ms * 速度倍率 * 爬升权重）
			vs := CurrentVerticalSpeed(ds.Segments, ds.Elapsed)
			ds.CumFlightSec += 0.1 * ds.SpeedMultiplier * powerWeight(vs)

			if elapsed >= totalFlightDuration {
				// 飞行结束，检查最后一个航点是否有悬停
				lastIdx := len(waypoints) - 1
				lastWp := waypoints[lastIdx]
				if ht := hoverTimes[lastIdx]; ht > 0 && !ds.InHover {
					// 在终点悬停
					ds.InHover = true
					ds.HoverStartTime = time.Now()
					ds.HoverDuration = ht
					ds.HoverWaypointIdx = lastWp.Index
					ds.Status = "hovering"
					ds.LastWaypointIndex = lastWp.Index
					ds.Elapsed = totalFlightDuration

					if e.onModeChange != nil {
						e.onModeChange(ds.DroneID, "hovering")
					}
					ds.mu.Unlock()
					continue
				}

				// 仿真完成
				ds.Status = "completed"
				ds.Elapsed = totalFlightDuration
				ticker.Stop()

				pt := Interpolate(ds.SimWaypoints, ds.Segments, totalFlightDuration)
				telemetry := e.buildTelemetry(ds, pt)
				telemetry.Status = "completed"
				telemetry.Progress = 100

				droneID := ds.DroneID
				ds.mu.Unlock()

				if e.onTelemetry != nil {
					e.onTelemetry(telemetry)
				}
				if e.onModeChange != nil {
					e.onModeChange(droneID, "completed")
				}
				return
			}

			// 正常插值
			pt := Interpolate(ds.SimWaypoints, ds.Segments, elapsed)

			// 风力偏移：超限时累积漂移（离地 > 5m 才生效，避免地面偏移）
			takeoffAlt := ds.Trajectory.TakeOffPoint[2]
			if pt.Altitude < takeoffAlt+5 {
				ds.WindWarning = false
			} else {
				windDrift := ComputeWindDrift(
					ds.WindSpeed, ds.WindDirection,
					ds.ModelSpec.MaxWindResistance,
					pt.Heading, pt.Speed,
					0.1*ds.SpeedMultiplier, // dt = tick * speed multiplier
				)
				if windDrift.Warning {
					ds.WindWarning = true
					ds.CumDriftLat += windDrift.DriftLat
					ds.CumDriftLng += windDrift.DriftLng
					pt.Speed = windDrift.GroundSpeed
					// 风力超限时耗电增加
					ds.CumFlightSec += 0.1 * ds.SpeedMultiplier * powerWeight(vs) * (windDrift.PowerFactor - 1.0)
				} else {
					ds.WindWarning = false
					// 无风或风力在抗风范围内：逐步回收累计漂移，线性回归原航线
					if ds.CumDriftLat != 0 || ds.CumDriftLng != 0 {
						const metersPerDegLat = 111320.0
						latRad := pt.Latitude * math.Pi / 180.0
						metersPerDegLng := metersPerDegLat * math.Cos(latRad)

						driftNorthM := ds.CumDriftLat * metersPerDegLat
						driftEastM := ds.CumDriftLng * metersPerDegLng
						driftDistM := math.Sqrt(driftNorthM*driftNorthM + driftEastM*driftEastM)

						if driftDistM > 0 && pt.Speed > 0 {
							dt := 0.1 * ds.SpeedMultiplier
							recoveryM := pt.Speed * dt // 以航线速度线性飞回
							if recoveryM > driftDistM {
								recoveryM = driftDistM
							}
							ratio := recoveryM / driftDistM
							ds.CumDriftLat -= ds.CumDriftLat * ratio
							ds.CumDriftLng -= ds.CumDriftLng * ratio
						}
					}
				}
			}

			// 检查是否到达新航点，触发悬停
			if pt.WaypointIndex != ds.LastWaypointIndex && pt.WaypointIndex >= 0 {
				ds.LastWaypointIndex = pt.WaypointIndex

				// 到达航点 pt.WaypointIndex，检查该航点是否有悬停
				hoverWpIdx := pt.WaypointIndex
				if ht, ok := hoverTimes[hoverWpIdx]; ok && ht > 0 {
					ds.InHover = true
					ds.HoverStartTime = time.Now()
					ds.HoverDuration = ht
					ds.HoverWaypointIdx = hoverWpIdx
					ds.Status = "hovering"

					if e.onModeChange != nil {
						e.onModeChange(ds.DroneID, "hovering")
					}

					// 发送到达航点时的遥测（速度降为0）
					arrivalPt := InterpolatedPoint{
						Longitude:     waypoints[hoverWpIdx].Longitude,
						Latitude:      waypoints[hoverWpIdx].Latitude,
						Altitude:      waypoints[hoverWpIdx].EllipsoidHeight,
						Height:        waypoints[hoverWpIdx].Height,
						Speed:         0,
						Heading:       pt.Heading,
						WaypointIndex: hoverWpIdx,
						Timestamp:     elapsed,
					}
					telemetry := e.buildTelemetry(ds, arrivalPt)
					telemetry.Status = "hovering"
					telemetry.CurrentAction = "hover"
					if len(waypoints[hoverWpIdx].Actions) > 0 {
						telemetry.WaypointActions = waypoints[hoverWpIdx].Actions
					}
					telemetry.HoverTimeRemaining = ht.Seconds()
					if totalDuration > 0 {
						// 累加该航点之前所有悬停时间 + 飞行时间
						accumHover := time.Duration(0)
						for i := 0; i <= hoverWpIdx && i < len(waypoints); i++ {
							if h, ok := hoverTimes[i]; ok {
								accumHover += h
							}
						}
						telemetry.Progress = float64(elapsed+accumHover) / float64(totalDuration) * 100
					}

					ds.mu.Unlock()

					if e.onTelemetry != nil {
						e.onTelemetry(telemetry)
					}
					continue
				}
			}

			telemetry := e.buildTelemetry(ds, pt)
			telemetry.Status = "running"

			// 进度：飞行时间 + 已完成悬停时间
			var completedHover time.Duration
			for i := 0; i <= pt.WaypointIndex && i < len(waypoints); i++ {
				if h, ok := hoverTimes[i]; ok {
					completedHover += h
				}
			}
			if totalDuration > 0 {
				telemetry.Progress = float64(elapsed+completedHover) / float64(totalDuration) * 100
			}

			ds.mu.Unlock()

			if e.onTelemetry != nil {
				e.onTelemetry(telemetry)
			}

		case <-pauseCh:
			ds.mu.Lock()
			// 暂停时也处理 hover 状态
			if ds.InHover {
				ds.Ticker.Stop()
				ds.PausedAt = ds.Elapsed
				ds.Status = "paused"
				ds.InHover = false // 恢复后重新进入 hover
				droneID := ds.DroneID
				ds.mu.Unlock()
				if e.onModeChange != nil {
					e.onModeChange(droneID, "paused")
				}
			} else if ds.Status == "running" {
				ds.Ticker.Stop()
				ds.PausedAt = ds.Elapsed
				ds.Status = "paused"
				droneID := ds.DroneID
				ds.mu.Unlock()
				if e.onModeChange != nil {
					e.onModeChange(droneID, "paused")
				}
			} else {
				ds.mu.Unlock()
			}

		case <-resumeCh:
			ds.mu.Lock()
			if ds.Status == "paused" {
				ds.StartTime = time.Now().Add(-ds.PausedAt)
				ds.Ticker = time.NewTicker(100 * time.Millisecond)
				ds.Status = "running"
				ds.InHover = false
				ds.LastWaypointIndex = -1

				ticker = ds.Ticker
				droneID := ds.DroneID
				ds.mu.Unlock()
				if e.onModeChange != nil {
					e.onModeChange(droneID, "running")
				}
			} else {
				ds.mu.Unlock()
			}

		case <-stopCh:
			// 停止清理已由 Stop() 同步完成，协程直接退出
			return
		}
	}
}

// buildTelemetry 根据插值点构建遥测数据
func (e *Engine) buildTelemetry(ds *DroneState, pt InterpolatedPoint) trajectory.RemoteIDTelemetry {
	takeoffLat := ds.Trajectory.TakeOffPoint[0]
	takeoffLng := ds.Trajectory.TakeOffPoint[1]
	takeoffAlt := ds.Trajectory.TakeOffPoint[2]

	// 叠加累计风力漂移
	driftedLat := pt.Latitude + ds.CumDriftLat
	driftedLng := pt.Longitude + ds.CumDriftLng

	heightAboveTakeoff := 0.0
	if takeoffLat != 0 || takeoffLng != 0 || takeoffAlt != 0 {
		heightAboveTakeoff = pt.Altitude - takeoffAlt
	}

	return trajectory.RemoteIDTelemetry{
		DroneID:            ds.DroneID,
		Latitude:           driftedLat,
		Longitude:          driftedLng,
		Altitude:           pt.Altitude,
		HeightAboveTakeoff: heightAboveTakeoff,
		Speed:              pt.Speed,
		Heading:            pt.Heading,
		Timestamp:          time.Now().UnixMilli(),
		WaypointIndex:      pt.WaypointIndex,
		TotalWaypoints:     len(ds.SimWaypoints),
		BatteryPercent:     ds.calcBatteryPercent(),
		WindSpeed:          ds.WindSpeed,
		WindDirection:      ds.WindDirection,
		WindWarning:        ds.WindWarning,
		RunGeneration:      ds.RunGeneration,
	}
}

// calcBatteryPercent 根据累计飞行时间计算电量百分比
// 公式：电量 = 100 * (1 - effectiveTime / maxFlightTime)
// 其中 effectiveTime = 飞行时间 + 悬停时间 * (maxFlightTime / maxHoverTime)
// 悬停耗电更快（如 M30 飞行41min vs 悬停36min），权重因子补偿
func (ds *DroneState) calcBatteryPercent() float64 {
	if ds.ModelSpec.MaxFlightTimeSec <= 0 {
		return 100
	}

	// 悬停耗电权重：悬停续航更短 → 权重 > 1 → 悬停更耗电
	hoverWeight := 1.0
	if ds.ModelSpec.MaxHoverTimeSec > 0 {
		hoverWeight = ds.ModelSpec.MaxFlightTimeSec / ds.ModelSpec.MaxHoverTimeSec
	}

	effectiveSec := ds.CumFlightSec + ds.CumHoverSec*hoverWeight
	battery := 100.0 * (1.0 - effectiveSec/ds.ModelSpec.MaxFlightTimeSec)
	if battery < 0 {
		return 0
	}
	if battery > 100 {
		return 100
	}
	return battery
}

// powerWeight 根据垂直速度返回耗电权重
// 爬升耗电显著增加，下降耗电减少
func powerWeight(verticalSpeed float64) float64 {
	if verticalSpeed > 0.5 {
		// 爬升：每 m/s 增加约 8% 额外耗电
		return 1.0 + verticalSpeed*0.08
	}
	if verticalSpeed < -0.5 {
		// 下降：约 75% 耗电（重力辅助）
		return 0.75
	}
	// 平飞：标准耗电
	return 1.0
}

// Pause 暂停仿真
func (e *Engine) Pause(droneID string) error {
	e.mu.RLock()
	ds, ok := e.drones[droneID]
	e.mu.RUnlock()

	if !ok {
		return fmt.Errorf("drone %s not found", droneID)
	}

	ds.mu.Lock()
	if ds.Status != "running" && ds.Status != "hovering" {
		ds.mu.Unlock()
		return fmt.Errorf("drone %s is not running (current status: %s)", droneID, ds.Status)
	}
	ds.mu.Unlock()

	// 发送暂停信号
	select {
	case ds.PauseCh <- struct{}{}:
	default:
	}

	return nil
}

// Resume 继续仿真
func (e *Engine) Resume(droneID string) error {
	e.mu.RLock()
	ds, ok := e.drones[droneID]
	e.mu.RUnlock()

	if !ok {
		return fmt.Errorf("drone %s not found", droneID)
	}

	ds.mu.Lock()
	if ds.Status != "paused" {
		ds.mu.Unlock()
		return fmt.Errorf("drone %s is not paused (current status: %s)", droneID, ds.Status)
	}
	ds.mu.Unlock()

	// 发送恢复信号
	select {
	case ds.ResumeCh <- struct{}{}:
	default:
	}
	return nil
}

// SetSpeed 设置仿真速度倍率（支持飞行中动态调节）
func (e *Engine) SetSpeed(droneID string, multiplier float64) error {
	e.mu.RLock()
	ds, ok := e.drones[droneID]
	e.mu.RUnlock()

	if !ok {
		return fmt.Errorf("drone %s not found", droneID)
	}
	if multiplier <= 0 {
		return fmt.Errorf("速度倍率必须大于 0")
	}

	ds.mu.Lock()
	oldMultiplier := ds.SpeedMultiplier
	ds.SpeedMultiplier = multiplier

	// 飞行中切换速度时，调整 StartTime 保持当前位置不跳变
	if ds.Status == "running" && oldMultiplier > 0 && oldMultiplier != multiplier {
		now := time.Now()
		oldElapsed := time.Duration(float64(now.Sub(ds.StartTime)) * oldMultiplier)
		ds.StartTime = now.Add(-time.Duration(float64(oldElapsed) / multiplier))
	}
	ds.mu.Unlock()
	return nil
}

// SetWind 设置风力参数
// direction: 风向 (度, 0=北, 90=东, 气象方向: 风来的方向)
// speed: 风速 (m/s), 设为 0 可关闭风力模拟
func (e *Engine) SetWind(droneID string, speed, direction float64) error {
	e.mu.RLock()
	ds, ok := e.drones[droneID]
	e.mu.RUnlock()

	if !ok {
		return fmt.Errorf("drone %s not found", droneID)
	}
	if speed < 0 {
		return fmt.Errorf("风速不能为负")
	}
	if direction < 0 || direction >= 360 {
		return fmt.Errorf("风向必须在 0-360 之间")
	}

	ds.mu.Lock()
	defer ds.mu.Unlock()

	ds.WindSpeed = speed
	ds.WindDirection = direction

	// 重置风力为 0 时停止漂移累积，但保留当前位置不清零
	if speed <= 0 {
		ds.WindWarning = false
	}

	return nil
}

// RemoveDrone 从引擎中彻底移除无人机（先停止再删除）
func (e *Engine) RemoveDrone(droneID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	ds, ok := e.drones[droneID]
	if !ok {
		return fmt.Errorf("drone %s not found", droneID)
	}

	// 先停止仿真
	if ds.Ticker != nil {
		ds.Ticker.Stop()
	}
	ds.Status = "idle"
	delete(e.drones, droneID)
	return nil
}

// ReturnHome 终止当前航线并生成返航路径：爬升→平飞→降落
func (e *Engine) ReturnHome(droneID string) error {
	e.mu.RLock()
	ds, ok := e.drones[droneID]
	e.mu.RUnlock()
	if !ok {
		return fmt.Errorf("drone %s not found", droneID)
	}

	ds.mu.Lock()
	defer ds.mu.Unlock()

	if ds.Status != "running" && ds.Status != "paused" && ds.Status != "hovering" {
		return fmt.Errorf("drone %s is not active (status: %s)", droneID, ds.Status)
	}

	// 停止当前仿真
	if ds.Ticker != nil {
		ds.Ticker.Stop()
	}

	// 获取当前位置
	var pt InterpolatedPoint
	if ds.Status == "hovering" {
		wp := ds.SimWaypoints[ds.HoverWaypointIdx]
		pt = InterpolatedPoint{
			Latitude:  wp.Latitude,
			Longitude: wp.Longitude,
			Altitude:  wp.EllipsoidHeight,
			Speed:     wp.Speed,
		}
	} else {
		pt = Interpolate(ds.SimWaypoints, ds.Segments, ds.Elapsed)
	}

	takeoffLat := ds.Trajectory.TakeOffPoint[0]
	takeoffLng := ds.Trajectory.TakeOffPoint[1]
	takeoffAlt := ds.Trajectory.TakeOffPoint[2]
	if takeoffLat == 0 && takeoffLng == 0 {
		return fmt.Errorf("no takeoff point defined")
	}

	// 确定返航高度和速度
	rthHeight := ds.Trajectory.RTHHeight
	if rthHeight <= 0 {
		rthHeight = pt.Altitude
	}
	rthHeight = math.Max(rthHeight, pt.Altitude)

	rthSpeed := ds.Trajectory.GlobalTransitionalSpeed
	if rthSpeed <= 0 {
		rthSpeed = ds.Trajectory.AutoFlightSpeed
	}
	if rthSpeed <= 0 {
		rthSpeed = 7
	}

	// 构建返航航点：当前位置 → 爬升 → 平飞 → 下降
	var rthWps []trajectory.Waypoint
	idx := -4 // 返航标记

	// 0: 当前位置
	rthWps = append(rthWps, trajectory.Waypoint{
		Index:           idx,
		Latitude:        pt.Latitude,
		Longitude:       pt.Longitude,
		EllipsoidHeight: pt.Altitude,
		Height:          pt.Altitude,
		Speed:           rthSpeed,
	})

	// 1: 垂直爬升到返航高度
	if rthHeight > pt.Altitude+0.5 {
		rthWps = append(rthWps, trajectory.Waypoint{
			Index:           idx,
			Latitude:        pt.Latitude,
			Longitude:       pt.Longitude,
			EllipsoidHeight: rthHeight,
			Height:          rthHeight,
			Speed:           rthSpeed,
		})
	}

	// 2: 平飞到起飞点正上方
	rthWps = append(rthWps, trajectory.Waypoint{
		Index:           idx,
		Latitude:        takeoffLat,
		Longitude:       takeoffLng,
		EllipsoidHeight: rthHeight,
		Height:          rthHeight,
		Speed:           rthSpeed,
	})

	// 3: 下降到起飞点高度
	if math.Abs(rthHeight-takeoffAlt) > 0.5 {
		rthWps = append(rthWps, trajectory.Waypoint{
			Index:           idx,
			Latitude:        takeoffLat,
			Longitude:       takeoffLng,
			EllipsoidHeight: takeoffAlt,
			Height:          takeoffAlt,
			Speed:           rthSpeed,
		})
	}

	// 替换仿真数据
	ds.SimWaypoints = rthWps
	ds.Segments = BuildSegments(rthWps)
	ds.Elapsed = 0
	ds.StartTime = time.Now()
	ds.LastWaypointIndex = -1
	ds.InHover = false
	ds.Status = "running"
	ds.RunGeneration++ // 递增仿真轮次
	ds.Ticker = time.NewTicker(100 * time.Millisecond)
	ds.StopCh = make(chan struct{})
	ds.PauseCh = make(chan struct{})
	ds.ResumeCh = make(chan struct{})

	// 启动返航仿真
	go e.runSimulationLoop(ds)
	return nil
}

// Stop 停止仿真（同步完成状态清理后才返回）
func (e *Engine) Stop(droneID string) error {
	e.mu.RLock()
	ds, ok := e.drones[droneID]
	e.mu.RUnlock()

	if !ok {
		return fmt.Errorf("drone %s not found", droneID)
	}

	ds.mu.Lock()
	if ds.Status == "idle" {
		ds.mu.Unlock()
		return nil
	}

	// 同步停止：先停 ticker、改状态，再通知协程退出
	if ds.Ticker != nil {
		ds.Ticker.Stop()
	}
	ds.Status = "idle"
	ds.Elapsed = 0
	ds.InHover = false
	ds.LastWaypointIndex = -1
	ds.CumDriftLat = 0
	ds.CumDriftLng = 0
	droneID = ds.DroneID
	ds.mu.Unlock()

	// 通知协程退出（协程会在下个 select 周期收到并 return）
	select {
	case ds.StopCh <- struct{}{}:
	default:
	}

	if e.onModeChange != nil {
		e.onModeChange(droneID, "idle")
	}

	return nil
}

// DroneStatusInfo 状态摘要
type DroneStatusInfo struct {
	ID                 string  `json:"id"`
	Name               string  `json:"name"`
	Status             string  `json:"status"`
	Progress           float64 `json:"progress"`
	Latitude           float64 `json:"lat"`
	Longitude          float64 `json:"lng"`
	Altitude           float64 `json:"alt"`
	HeightAboveTakeoff float64 `json:"heightAboveTakeoff"`
	Speed              float64 `json:"speed"`
	Heading            float64 `json:"heading"`
	WaypointIndex      int     `json:"waypointIndex"`
	TotalWaypoints     int     `json:"totalWaypoints"`
	CurrentAction      string  `json:"currentAction"`
	HoverTimeRemaining float64 `json:"hoverTimeRemaining"`
	SpeedMultiplier    float64 `json:"speedMultiplier"`
	WindSpeed          float64 `json:"windSpeed"`
	WindDirection      float64 `json:"windDirection"`
	WindWarning        bool    `json:"windWarning"`
	MaxWindResistance  float64 `json:"maxWindResistance"`
	RunGeneration      int64   `json:"runGeneration"`
}

// GetStatus 获取所有无人机状态
func (e *Engine) GetStatus() map[string]DroneStatusInfo {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make(map[string]DroneStatusInfo, len(e.drones))
	for id, ds := range e.drones {
		ds.mu.Lock()

		info := DroneStatusInfo{
			ID:     ds.DroneID,
			Name:   ds.Name,
			Status: ds.Status,
		}

		if ds.Status == "hovering" {
			info.Status = "hovering"
			info.CurrentAction = "hover"
			info.HoverTimeRemaining = math.Max(0, ds.HoverDuration.Seconds()-time.Since(ds.HoverStartTime).Seconds())
		}

		// 计算 hover 时间总和
		var totalHoverDuration time.Duration
		for _, wp := range ds.SimWaypoints {
			if ht := wp.HoverTime(); ht > 0 {
				totalHoverDuration += time.Duration(ht * float64(time.Second))
			}
		}

		// 计算总飞行时长
		var totalFlightDuration time.Duration
		for _, seg := range ds.Segments {
			totalFlightDuration += seg.Duration
		}
		totalDuration := totalFlightDuration + totalHoverDuration

		if totalDuration > 0 {
			switch ds.Status {
			case "hovering":
				var completedHover time.Duration
				for i := 0; i < ds.HoverWaypointIdx && i < len(ds.SimWaypoints); i++ {
					if ht := ds.SimWaypoints[i].HoverTime(); ht > 0 {
						completedHover += time.Duration(ht * float64(time.Second))
					}
				}
				hoverElapsed := time.Since(ds.HoverStartTime)
				info.Progress = float64(ds.Elapsed+completedHover+hoverElapsed) / float64(totalDuration) * 100
			case "running":
				var completedHover time.Duration
				for i := 0; i <= ds.LastWaypointIndex && i < len(ds.SimWaypoints); i++ {
					if ht := ds.SimWaypoints[i].HoverTime(); ht > 0 {
						completedHover += time.Duration(ht * float64(time.Second))
					}
				}
				info.Progress = float64(ds.Elapsed+completedHover) / float64(totalDuration) * 100
			case "paused":
				var completedHover time.Duration
				for i := 0; i <= ds.LastWaypointIndex && i < len(ds.SimWaypoints); i++ {
					if ht := ds.SimWaypoints[i].HoverTime(); ht > 0 {
						completedHover += time.Duration(ht * float64(time.Second))
					}
				}
				info.Progress = float64(ds.PausedAt+completedHover) / float64(totalDuration) * 100
			case "completed":
				info.Progress = 100
			default:
				info.Progress = 0
			}
		}

		// 获取当前位置
		var pt InterpolatedPoint
		switch ds.Status {
		case "hovering":
			if ds.HoverWaypointIdx >= 0 && ds.HoverWaypointIdx < len(ds.SimWaypoints) {
				wp := ds.SimWaypoints[ds.HoverWaypointIdx]
				pt = InterpolatedPoint{
					Latitude:      wp.Latitude,
					Longitude:     wp.Longitude,
					Altitude:      wp.EllipsoidHeight,
					Height:        wp.Height,
					Speed:         0,
					Heading:       wp.Heading,
					WaypointIndex: wp.Index,
				}
			}
		case "running":
			pt = Interpolate(ds.SimWaypoints, ds.Segments, ds.Elapsed)
		case "paused":
			pt = Interpolate(ds.SimWaypoints, ds.Segments, ds.PausedAt)
		case "completed":
			pt = Interpolate(ds.SimWaypoints, ds.Segments, totalFlightDuration)
		default:
			if len(ds.SimWaypoints) > 0 {
				wp := ds.SimWaypoints[0]
				pt = InterpolatedPoint{
					Latitude:  wp.Latitude,
					Longitude: wp.Longitude,
					Altitude:  wp.EllipsoidHeight,
					Speed:     wp.Speed,
					Heading:   wp.Heading,
				}
			}
		}

		info.Latitude = pt.Latitude + ds.CumDriftLat
		info.Longitude = pt.Longitude + ds.CumDriftLng
		info.Altitude = pt.Altitude
		info.Speed = pt.Speed
		info.Heading = pt.Heading
		info.WaypointIndex = pt.WaypointIndex
		info.TotalWaypoints = len(ds.SimWaypoints)
		info.SpeedMultiplier = ds.SpeedMultiplier
		info.WindSpeed = ds.WindSpeed
		info.WindDirection = ds.WindDirection
		info.WindWarning = ds.WindWarning
		info.MaxWindResistance = ds.ModelSpec.MaxWindResistance
		info.RunGeneration = ds.RunGeneration

		takeoffAlt := ds.Trajectory.TakeOffPoint[2]
		if takeoffAlt != 0 {
			info.HeightAboveTakeoff = pt.Altitude - takeoffAlt
		}

		ds.mu.Unlock()

		result[id] = info
	}

	return result
}

// GetTrajectories 获取所有已注册无人机的轨迹数据（供前端恢复状态）
func (e *Engine) GetTrajectories() []*trajectory.Trajectory {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make([]*trajectory.Trajectory, 0, len(e.drones))
	for _, ds := range e.drones {
		result = append(result, ds.Trajectory)
	}
	return result
}
