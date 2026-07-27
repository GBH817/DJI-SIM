package parser

import (
	"archive/zip"
	"bytes"
	"drone-sim-backend/internal/trajectory"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

const templateKMLPath = "wpmz/template.kml"
const waylinesWPMLPath = "wpmz/waylines.wpml"

// kmzParser KMZ 文件解析器
type kmzParser struct{}

// NewKMZParser 创建 KMZ 解析器
func NewKMZParser() *kmzParser {
	return &kmzParser{}
}

// SupportedExtensions 返回支持的文件扩展名
func (p *kmzParser) SupportedExtensions() []string {
	return []string{".kmz"}
}

// Parse 解析 KMZ 文件，返回航线轨迹
func (p *kmzParser) Parse(r io.Reader, filename string) (*trajectory.Trajectory, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %w", err)
	}

	zipReader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("解压 KMZ 失败: %w", err)
	}

	var templateData, waylinesData []byte

	for _, f := range zipReader.File {
		switch f.Name {
		case templateKMLPath, waylinesWPMLPath:
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("读取 KMZ 内部文件 %s 失败: %w", f.Name, err)
			}
			content, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return nil, fmt.Errorf("读取 KMZ 内部文件内容 %s 失败: %w", f.Name, err)
			}
			if f.Name == templateKMLPath {
				templateData = content
			} else {
				waylinesData = content
			}
		}
	}

	// 解析 template.kml 获取 mission config 和基础航点
	missionCfg, templateWaypoints, err := parseTemplateKML(templateData)
	if err != nil {
		return nil, fmt.Errorf("解析 template.kml 失败: %w", err)
	}

	// 解析 waylines.wpml 获取执行航点数据（含 executeHeight）
	folderData, wpmlWaypoints, err := parseWaylinesWPML(waylinesData)
	if err != nil {
		return nil, fmt.Errorf("解析 waylines.wpml 失败: %w", err)
	}

	// 合并航点：优先使用 waylines.wpml 的数据
	mergedWaypoints := mergeWaypoints(templateWaypoints, wpmlWaypoints)

	// 解析动作组（从 waylines.wpml 中提取）
	actionsMap, _ := parseActionGroups(waylinesData)

	// 解析起飞点: takeOffRefPoint 格式为 "lat,lng,alt"
	takeOffPoint := parseTakeOffRefPoint(missionCfg.TakeOffRefPoint)

	// 构建 Trajectory
	traj := &trajectory.Trajectory{
		Name:                    filename,
		Waypoints:               convertWaypoints(mergedWaypoints, actionsMap),
		TakeOffPoint:            takeOffPoint,
		TotalDistance:           folderData.Distance,
		TotalDuration:           time.Duration(folderData.Duration) * time.Second,
		FinishAction:            missionCfg.FinishAction,
		RTHHeight:               missionCfg.RTHHeight,
		GlobalHeight:            missionCfg.GlobalHeight,
		AutoFlightSpeed:         selectFloat(folderData.AutoFlightSpeed, missionCfg.AutoFlightSpeed),
		TakeOffSecurityHeight:   missionCfg.TakeOffSecurityHeight,
		GlobalTransitionalSpeed: missionCfg.GlobalTransitionalSpeed,
		DroneEnumValue:          missionCfg.DroneEnumValue,
		DroneSubEnumValue:       missionCfg.DroneSubEnumValue,
		DroneModelName:          trajectory.MapDroneModel(missionCfg.DroneEnumValue, missionCfg.DroneSubEnumValue),
		PayloadEnumValue:        missionCfg.PayloadEnumValue,
		PayloadSubEnumValue:     missionCfg.PayloadSubEnumValue,
		PayloadModelName:        trajectory.MapPayloadModel(missionCfg.PayloadEnumValue, missionCfg.PayloadSubEnumValue),
		FlyToWaylineMode:       missionCfg.FlyToWaylineMode,
	}

	return traj, nil
}

func init() {
	DefaultRegistry().Register(NewKMZParser())
}

// ---- 内部数据结构 ----

type kmlMissionConfig struct {
	TakeOffRefPoint          string
	FinishAction             string
	RTHHeight                float64
	AutoFlightSpeed          float64
	GlobalHeight             float64
	TakeOffSecurityHeight    float64
	GlobalTransitionalSpeed  float64
	DroneEnumValue           int
	DroneSubEnumValue        int
	PayloadEnumValue         int
	PayloadSubEnumValue      int
	FlyToWaylineMode         string
}

type kmlWaypoint struct {
	Index           int
	Longitude       float64
	Latitude        float64
	Height          float64 // EGM96高度 (template.kml 的 height)
	EllipsoidHeight float64 // 椭球高度
	Speed           float64
	Heading         float64
	TurnMode        string  // coordinateTurn / toPointAndStopWithDiscontinuityCurvature
	TurnDampingDist float64
}

type wpmlFolderData struct {
	AutoFlightSpeed float64
	Distance        float64
	Duration        float64
}

// kmlActionGroup 解析用的动作组结构
type kmlActionGroup struct {
	StartIndex int
	EndIndex   int
	Actions    []kmlAction
}

// kmlAction 单个动作
type kmlAction struct {
	FuncType string // rotateYaw, gimbalRotate, zoom, takePhoto, hover, startRecord, stopRecord, focus
	Params   map[string]interface{}
}

// ---- 解析 template.kml ----

func parseTemplateKML(data []byte) (*kmlMissionConfig, []kmlWaypoint, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))

	cfg := &kmlMissionConfig{}
	var waypoints []kmlWaypoint
	var currentWP *kmlWaypoint

	var inMissionConfig bool
	var inPlacemark bool
	var inPoint bool
	var inHeadingParam bool
	var inDroneInfo bool
	var inPayloadInfo bool
	var currentText strings.Builder

	resetText := func() { currentText.Reset() }

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, err
		}

		switch t := token.(type) {
		case xml.StartElement:
			local := cleanLocalName(t.Name.Local)
			switch {
			case local == "missionConfig":
				inMissionConfig = true
			case local == "Placemark":
				if !inMissionConfig {
					inPlacemark = true
					currentWP = &kmlWaypoint{}
				}
			case local == "Point" && inPlacemark:
				inPoint = true
			case local == "waypointHeadingParam":
				inHeadingParam = true
			case local == "droneInfo":
				inDroneInfo = true
			case local == "payloadInfo":
				inPayloadInfo = true
			}
			resetText()

		case xml.CharData:
			currentText.Write(t)

		case xml.EndElement:
			local := cleanLocalName(t.Name.Local)
			text := strings.TrimSpace(currentText.String())

			switch {
			case local == "missionConfig":
				inMissionConfig = false
			case local == "Placemark":
				if inPlacemark && currentWP != nil {
					waypoints = append(waypoints, *currentWP)
					currentWP = nil
				}
				inPlacemark = false
			case local == "Point":
				inPoint = false
			case local == "waypointHeadingParam":
				inHeadingParam = false
			case local == "droneInfo":
				inDroneInfo = false
			case local == "payloadInfo":
				inPayloadInfo = false
			}

			if inMissionConfig {
				switch local {
				case "takeOffRefPoint":
					cfg.TakeOffRefPoint = text
				case "finishAction":
					cfg.FinishAction = text
				case "globalRTHHeight":
					cfg.RTHHeight, _ = strconv.ParseFloat(text, 64)
				case "autoFlightSpeed":
					cfg.AutoFlightSpeed, _ = strconv.ParseFloat(text, 64)
				case "globalHeight":
					cfg.GlobalHeight, _ = strconv.ParseFloat(text, 64)
				case "takeOffSecurityHeight":
					cfg.TakeOffSecurityHeight, _ = strconv.ParseFloat(text, 64)
				case "globalTransitionalSpeed":
					cfg.GlobalTransitionalSpeed, _ = strconv.ParseFloat(text, 64)
				case "flyToWaylineMode":
					cfg.FlyToWaylineMode = text
				}

				if inDroneInfo {
					switch local {
					case "droneEnumValue":
						cfg.DroneEnumValue, _ = strconv.Atoi(text)
					case "droneSubEnumValue":
						cfg.DroneSubEnumValue, _ = strconv.Atoi(text)
					}
				}

				if inPayloadInfo {
					switch local {
					case "payloadEnumValue":
						cfg.PayloadEnumValue, _ = strconv.Atoi(text)
					case "payloadSubEnumValue":
						cfg.PayloadSubEnumValue, _ = strconv.Atoi(text)
					}
				}
			}

			if inPlacemark && currentWP != nil {
				if inPoint && local == "coordinates" {
					lng, lat := parseCoordinates(text)
					currentWP.Longitude = lng
					currentWP.Latitude = lat
				}
				switch local {
				case "index":
					currentWP.Index, _ = strconv.Atoi(text)
				case "ellipsoidHeight":
					currentWP.EllipsoidHeight, _ = strconv.ParseFloat(text, 64)
				case "height":
					currentWP.Height, _ = strconv.ParseFloat(text, 64)
				case "waypointSpeed":
					currentWP.Speed, _ = strconv.ParseFloat(text, 64)
				case "waypointHeadingAngle":
					if inHeadingParam {
						currentWP.Heading, _ = strconv.ParseFloat(text, 64)
					}
				}
			}

			resetText()
		}
	}

	return cfg, waypoints, nil
}

// ---- 解析 waylines.wpml ----

func parseWaylinesWPML(data []byte) (*wpmlFolderData, []kmlWaypoint, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))

	folder := &wpmlFolderData{}
	var waypoints []kmlWaypoint
	var currentWP *kmlWaypoint

	var inFolder bool
	var inPlacemark bool
	var inPoint bool
	var inHeadingParam bool
	var inTurnParam bool
	var currentText strings.Builder

	resetText := func() { currentText.Reset() }

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, err
		}

		switch t := token.(type) {
		case xml.StartElement:
			local := cleanLocalName(t.Name.Local)
			switch {
			case local == "Folder":
				inFolder = true
			case local == "Placemark" && inFolder:
				inPlacemark = true
				currentWP = &kmlWaypoint{}
			case local == "Point" && inPlacemark:
				inPoint = true
			case local == "waypointHeadingParam":
				inHeadingParam = true
			case local == "waypointTurnParam":
				inTurnParam = true
			}
			resetText()

		case xml.CharData:
			currentText.Write(t)

		case xml.EndElement:
			local := cleanLocalName(t.Name.Local)
			text := strings.TrimSpace(currentText.String())

			switch {
			case local == "Folder":
				inFolder = false
			case local == "Placemark":
				if inPlacemark && currentWP != nil {
					waypoints = append(waypoints, *currentWP)
					currentWP = nil
				}
				inPlacemark = false
			case local == "Point":
				inPoint = false
			case local == "waypointHeadingParam":
				inHeadingParam = false
			case local == "waypointTurnParam":
				inTurnParam = false
			}

			if inFolder && !inPlacemark {
				switch local {
				case "autoFlightSpeed":
					folder.AutoFlightSpeed, _ = strconv.ParseFloat(text, 64)
				case "distance":
					folder.Distance, _ = strconv.ParseFloat(text, 64)
				case "duration":
					folder.Duration, _ = strconv.ParseFloat(text, 64)
				}
			}

			if inPlacemark && currentWP != nil {
				if inPoint && local == "coordinates" {
					lng, lat := parseCoordinates(text)
					currentWP.Longitude = lng
					currentWP.Latitude = lat
				}
				switch local {
				case "index":
					currentWP.Index, _ = strconv.Atoi(text)
				case "executeHeight":
					currentWP.EllipsoidHeight, _ = strconv.ParseFloat(text, 64)
				case "waypointSpeed":
					currentWP.Speed, _ = strconv.ParseFloat(text, 64)
				case "waypointHeadingAngle":
					if inHeadingParam {
						currentWP.Heading, _ = strconv.ParseFloat(text, 64)
					}
				case "waypointTurnMode":
					if inTurnParam {
						currentWP.TurnMode = text
					}
				case "waypointTurnDampingDist":
					if inTurnParam {
						currentWP.TurnDampingDist, _ = strconv.ParseFloat(text, 64)
					}
				}
			}

			resetText()
		}
	}

	return folder, waypoints, nil
}

// ---- 辅助函数 ----

// cleanLocalName 去除 XML namespace 前缀，返回纯 local name
// encoding/xml 会保留 namespace 信息在 xml.Name.Space，但 Local 已经包含 namespace 前缀时要去掉
func cleanLocalName(name string) string {
	// xml.Decoder 在遇到有前缀的元素时，Local 字段可能是 "wpml:index" 形式
	if idx := strings.LastIndex(name, ":"); idx >= 0 {
		return name[idx+1:]
	}
	return name
}

// parseCoordinates 解析 "lng,lat" 或 "lng,lat,alt" 格式的坐标字符串
func parseCoordinates(s string) (lng, lat float64) {
	parts := strings.Split(strings.TrimSpace(s), ",")
	if len(parts) >= 2 {
		lng, _ = strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		lat, _ = strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	}
	return
}

// parseTakeOffRefPoint 解析 "lat,lng,alt" 格式的起飞参考点
func parseTakeOffRefPoint(s string) [3]float64 {
	var result [3]float64
	parts := strings.Split(strings.TrimSpace(s), ",")
	if len(parts) >= 3 {
		result[0], _ = strconv.ParseFloat(strings.TrimSpace(parts[0]), 64) // lat
		result[1], _ = strconv.ParseFloat(strings.TrimSpace(parts[1]), 64) // lng
		result[2], _ = strconv.ParseFloat(strings.TrimSpace(parts[2]), 64) // alt
	} else if len(parts) >= 2 {
		result[0], _ = strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		result[1], _ = strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	}
	return result
}

// mergeWaypoints 合并 template 和 waylines 的航点，优先使用 waylines 数据
func mergeWaypoints(template, wpml []kmlWaypoint) []kmlWaypoint {
	// 用 wpml 的航点作为基础
	if len(wpml) > 0 {
		// 构建 template 索引（按 index）
		tmplByIndex := make(map[int]kmlWaypoint)
		for _, w := range template {
			tmplByIndex[w.Index] = w
		}

		result := make([]kmlWaypoint, len(wpml))
		for i, w := range wpml {
			result[i] = w
			// 用 template 数据补充
			if tw, ok := tmplByIndex[w.Index]; ok {
				result[i].Height = tw.Height
				// 如果 waylines 没有设置 speed/heading，用 template 的值
				if result[i].Speed == 0 {
					result[i].Speed = tw.Speed
				}
				if result[i].Heading == 0 {
					result[i].Heading = tw.Heading
				}
				// 如果 waylines 没有坐标，用 template 的
				if result[i].Longitude == 0 && result[i].Latitude == 0 {
					result[i].Longitude = tw.Longitude
					result[i].Latitude = tw.Latitude
				}
			}
		}
		return result
	}

	// 如果没有 waylines 数据，直接用 template 的
	return template
}

// convertWaypoints 将内部航点转换为 trajectory.Waypoint
func convertWaypoints(kws []kmlWaypoint, actionsMap map[int][]kmlAction) []trajectory.Waypoint {
	result := make([]trajectory.Waypoint, len(kws))
	for i, w := range kws {
		result[i] = trajectory.Waypoint{
			Index:           w.Index,
			Longitude:       w.Longitude,
			Latitude:        w.Latitude,
			Height:          w.Height,
			EllipsoidHeight: w.EllipsoidHeight,
			Speed:           w.Speed,
			Heading:         w.Heading,
			TurnMode:        w.TurnMode,
			TurnDampingDist: w.TurnDampingDist,
		}
		// 附加动作
		if actions, ok := actionsMap[w.Index]; ok {
			for _, a := range actions {
				result[i].Actions = append(result[i].Actions, trajectory.WaypointAction{
					Type:   a.FuncType,
					Params: a.Params,
				})
			}
		}
	}
	return result
}

// selectFloat 优先返回 a，如果 a 为 0 则返回 b
func selectFloat(a, b float64) float64 {
	if a != 0 {
		return a
	}
	return b
}

// parseActionGroups 解析 waylines.wpml 中的 actionGroup 元素
func parseActionGroups(data []byte) (map[int][]kmlAction, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))

	result := make(map[int][]kmlAction)
	var inActionGroup bool
	var inAction bool
	var inActionParam bool
	var currentGroup *kmlActionGroup
	var currentAction *kmlAction
	var currentText strings.Builder

	resetText := func() { currentText.Reset() }

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		switch t := token.(type) {
		case xml.StartElement:
			local := cleanLocalName(t.Name.Local)
			switch {
			case local == "actionGroup":
				inActionGroup = true
				currentGroup = &kmlActionGroup{}
			case local == "action" && inActionGroup:
				inAction = true
				currentAction = &kmlAction{Params: make(map[string]interface{})}
			case local == "actionActuatorFuncParam" && inAction:
				inActionParam = true
			}
			resetText()

		case xml.CharData:
			currentText.Write(t)

		case xml.EndElement:
			local := cleanLocalName(t.Name.Local)
			text := strings.TrimSpace(currentText.String())

			switch {
			case local == "actionGroup":
				inActionGroup = false
				if currentGroup != nil && len(currentGroup.Actions) > 0 {
					for i := currentGroup.StartIndex; i <= currentGroup.EndIndex; i++ {
						// 每个航点添加该动作组的所有动作
						result[i] = append(result[i], currentGroup.Actions...)
					}
				}
				currentGroup = nil
			case local == "action":
				if inAction && currentAction != nil && currentAction.FuncType != "" {
					currentGroup.Actions = append(currentGroup.Actions, *currentAction)
				}
				inAction = false
				currentAction = nil
			case local == "actionActuatorFuncParam":
				inActionParam = false
			}

			if inActionGroup && currentGroup != nil {
				switch local {
				case "actionGroupStartIndex":
					currentGroup.StartIndex, _ = strconv.Atoi(text)
				case "actionGroupEndIndex":
					currentGroup.EndIndex, _ = strconv.Atoi(text)
				}
			}

			if inAction && currentAction != nil {
				switch local {
				case "actionActuatorFunc":
					currentAction.FuncType = text
				}

				if inActionParam {
					// 解析参数，存储所有子元素
					if text != "" {
						currentAction.Params[local] = text
						// 尝试转换为数值
						if f, err := strconv.ParseFloat(text, 64); err == nil {
							currentAction.Params[local] = f
						}
					}
				}
			}

			resetText()
		}
	}

	return result, nil
}
