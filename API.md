# 低空飞行数据生成器 — 后端 API 文档

## 服务配置

| 配置项 | 默认值 | 说明 |
|--------|--------|------|
| HTTP 端口 | `:8080` | 环境变量 `SERVER_PORT` |
| 内嵌 MQTT TCP | `:1883` | 所有网卡，无需外部 Broker |
| 内嵌 MQTT WebSocket | `:1884` | 所有网卡 |
| CORS | `*` | 允许任意来源跨域 |

---

## 一、HTTP API

### 1.1 上传航线文件

```
POST /api/upload
Content-Type: multipart/form-data
```

**参数**：`files` — 一个或多个 `.kmz` 文件

**响应**：
```json
{
  "success": true,
  "data": [
    {
      "id": "drone-xxx",
      "name": "航线名称",
      "waypoints": [{ "lat": 22.1, "lng": 113.1, "height": 100, "ellipsoidHeight": 100, "speed": 8, "heading": 90, "index": 0 }],
      "takeOffPoint": [22.0, 113.0, 10],
      "droneModelName": "Matrice 350 RTK",
      "payloadModelName": "H20T",
      "takeOffSecurityHeight": 30,
      "globalTransitionalSpeed": 5,
      "autoFlightSpeed": 8,
      "totalDistance": 1200.5,
      "finishAction": "goHome",
      "flyToWaylineMode": "safely"
    }
  ]
}
```

**调用示例**（curl）：
```bash
curl -X POST http://localhost:8080/api/upload -F "files=@航线.kmz"
```

---

### 1.2 获取所有无人机状态

```
GET /api/sim/status
```

**响应**：
```json
{
  "success": true,
  "data": [
    {
      "id": "drone-xxx",
      "name": "航线名",
      "status": "running",
      "progress": 45.2,
      "lat": 22.123456,
      "lng": 113.654321,
      "alt": 120.5,
      "heightAboveTakeoff": 100.0,
      "speed": 8.0,
      "verticalSpeed": 2.5,
      "heading": 90.0,
      "waypointIndex": 3,
      "totalWaypoints": 10,
      "currentAction": "hover",
      "hoverTimeRemaining": 5.0,
      "speedMultiplier": 1.0,
      "windSpeed": 5.0,
      "windDirection": 180,
      "windWarning": false,
      "maxWindResistance": 12.0,
      "runGeneration": 2
    }
  ]
}
```

---

### 1.3 获取已注册航线数据

```
GET /api/sim/trajectories
```

**响应**：同上 `data` 结构，每个元素为完整的 `Trajectory` 对象（含航点、起飞点、机型等）。

---

### 1.4 设置仿真速度倍率

```
POST /api/sim/speed
```

**请求**：
```json
{ "droneId": "drone-xxx", "speed": 2.0 }
```

**响应**：`{ "success": true }`

---

### 1.5 设置风力参数

```
POST /api/sim/wind
```

**请求**：
```json
{ "droneId": "drone-xxx", "speed": 12.0, "direction": 270 }
```

- `speed`：风速 m/s，不可为负，设为 0 关闭风力
- `direction`：风向（度），0=北 90=东 180=南 270=西（气象方向：风来的方向）

**响应**：`{ "success": true }`

---

### 1.6 删除无人机

```
DELETE /api/sim/drone/:id
```

**响应**：`{ "success": true }`

---

## 二、MQTT 接口

### 2.1 飞行控制指令（向服务端发送）

**Topic**：`thing/product/{droneId}/services`

**请求格式**：
```json
{
  "tid": "uuid",
  "bid": "uuid",
  "timestamp": 1722096000000,
  "data": {
    "method": "flight_task_execute",
    "speed": 1.0,
    "direction": 180
  }
}
```

**支持的 method**：

| method | 功能 | 备注 |
|--------|------|------|
| `flight_task_execute` | 开始执行航线 | — |
| `flight_task_pause` | 暂停 | — |
| `flight_task_resume` | 继续 | — |
| `flight_task_terminate` | 停止 | — |
| `flight_task_return_home` | 返航 | — |
| `sim_set_speed` | 设置仿真速度 | `data.speed` 传入倍率 |
| `sim_set_wind` | 设置风力 | `data.speed` + `data.direction` |

> `flight_task_*` 为大疆官方 API 同名指令，其他大疆前端可直接对接。

**回复 Topic**：`thing/product/{droneId}/services_reply`
```json
{
  "tid": "uuid",
  "bid": "uuid",
  "timestamp": 1722096000000,
  "data": { "result": 0 }
}
```
`result`：0=成功，1=失败。

---

### 2.2 DRC 远程控制指令（向服务端发送）

**Topic**：`thing/product/{droneId}/drc/down`

按大疆 DRC 协议格式发送：
```json
{
  "method": "drone_emergency_stop",
  "seq": 1,
  "data": {}
}
```

**支持的 method**：

| method | 功能 | 物理效果 |
|--------|------|----------|
| `drone_emergency_stop` | 紧急停机 | 电机立刻断电，自由落体 `h = h₀ − ½gt²`，风力继续水平漂移 |
| `drc_emergency_landing` | 紧急降落 | 可控下降 3m/s，风力继续水平漂移 |

> DRC 指令优先级高于航线，可打断正在执行的航线任务。着陆后状态变为 `emergency_stopped`（坠毁）或 `emergency_landed`（迫降）。

---

### 2.3 实时遥测（服务端发布）

**Topic**：`drone/{droneId}/telemetry`（简化格式） / `thing/product/{droneId}/osd`（大疆兼容格式）

发布频率：每 100ms

**简化格式** (`drone/{droneId}/telemetry`)：
```json
{
  "droneId": "drone-xxx",
  "lat": 22.123456,
  "lng": 113.654321,
  "alt": 120.5,
  "heightAboveTakeoff": 100.0,
  "speed": 8.0,
  "heading": 90.0,
  "status": "running",
  "timestamp": 1722096000000,
  "waypointIndex": 3,
  "totalWaypoints": 10,
  "progress": 45.2,
  "currentAction": "hover",
  "hoverTimeRemaining": 5.0,
  "batteryPercent": 85.5,
  "windSpeed": 5.0,
  "windDirection": 180,
  "windWarning": false,
  "runGeneration": 2
}
```

**大疆兼容格式** (`thing/product/{droneId}/osd`)：
```json
{
  "tid": "uuid",
  "bid": "uuid",
  "timestamp": 1722096000000,
  "gateway": "drone-xxx",
  "data": {
    "latitude": 22.123456,
    "longitude": 113.654321,
    "altitude": 120.5,
    "attitude_pitch": 0,
    "attitude_roll": 0,
    "attitude_yaw": 90.0,
    "horizontal_speed": 8.0,
    "vertical_speed": 2.5,
    "height": 100.0,
    "wind_speed": 5.0,
    "battery": { "capacity_percent": 85, "voltage": 24.8, "temperature": 35 },
    "position_state": { "is_fixed": 1, "quality": 5 },
    "waypoint_index": 3,
    "total_waypoints": 10,
    "progress": 45.2,
    "current_action": "hover",
    "hover_time_remaining": 5.0,
    "run_generation": 2
  }
}
```

---

### 2.4 设备状态（服务端发布）

**Topic**：`thing/product/{droneId}/state`

触发时机：设备上线、飞行模式变更（含紧急操作）

```json
{
  "tid": "uuid",
  "bid": "uuid",
  "timestamp": 1722096000000,
  "gateway": "drone-xxx",
  "data": {
    "firmware_version": "v10.01.0000",
    "mode_code": 5,
    "flight_status": "running",
    "battery": { "capacity_percent": 100, "voltage": 25.2, "temperature": 35 },
    "payloads": [{ "payload_index": "52-0-0", "firmware_version": "v04.00.0010" }],
    "cameras": [{ "payload_index": "52-0-0", "camera_mode": 0, "photo_state": 0, "recording_state": 0 }],
    "obstacle_avoidance": { "horizon": 1, "upside": 1, "downside": 1 },
    "night_lights_state": 1
  }
}
```

**mode_code 映射**（大疆标准）：

| mode_code | status | 含义 |
|-----------|--------|------|
| 0 | `idle` / `paused` / `completed` / `emergency_stopped` / `emergency_landed` | 待命 / 暂停 / 完成 / 已坠毁 / 已迫降 |
| 2 | `emergency_landing` | 紧急降落中（可控下降 3m/s） |
| 5 | `running` / `hovering` | 航线飞行中 / 悬停中 |
| 7 | `emergency_stop` | 紧急停机中（电机断电，自由落体） |

> `flight_status` 为仿真扩展字段，携带准确的详细状态字符串（如 `"emergency_stopped"`），用于前端精确显示。大疆标准设备可仅依赖 `mode_code`。

**状态枚举全表**：

| status | 含义 | 前端显示 |
|--------|------|----------|
| `idle` | 待命 | 灰色 |
| `running` | 飞行中 | 绿色 |
| `paused` | 已暂停 | 橙色 |
| `completed` | 已完成 | 蓝色 |
| `hovering` | 悬停中 | 黄色 |
| `emergency_stop` | 紧急停机中（坠落） | 红色 |
| `emergency_landing` | 紧急降落中 | 黄色 |
| `emergency_stopped` | 已坠毁 | 红色 |
| `emergency_landed` | 已迫降 | 黄色 |

> **注意**：`flight_status` 仅出现在 state topic（`thing/product/{id}/state`），OSD topic 不携带飞行状态，符合大疆 OSD 协议规范。

---

## 三、典型调用流程

```
1. HTTP POST /api/upload          → 上传 KMZ，获取 droneId
2. MQTT 订阅 thing/product/{droneId}/osd     → 接收实时遥测
3. MQTT 订阅 thing/product/{droneId}/state   → 接收状态变更（含 mode_code）
4. MQTT 发送 flight_task_execute              → 开始执行航线
5. MQTT 发送 flight_task_pause               → 暂停
6. MQTT 发送 flight_task_resume              → 继续
7. MQTT 发送 flight_task_terminate           → 停止
8. MQTT 发送 flight_task_return_home          → 返航

紧急操作：
  MQTT 发送 drone_emergency_stop  (DRC)     → 紧急停机（自由落体）
  MQTT 发送 drc_emergency_landing (DRC)     → 紧急降落（3m/s 可控下降）

参数调节：
  HTTP POST /api/sim/speed       → 设置仿真倍率
  HTTP POST /api/sim/wind        → 设置风力
  (也支持 MQTT sim_set_speed / sim_set_wind)
```

---

## 四、关键机制

- **遥测频率**：10Hz（100ms/帧），叠加风力漂移后的位置
- **垂直速度**：根据当前航段高度差实时计算，正值=爬升，负值=下降
- **电池模拟**：基于机型续航参数，飞行+悬停综合计算，爬升额外耗电 +8%/m/s，下降 75% 耗电
- **风力模拟**：风速 ≤ 抗风限制时无影响，仅影响耗电；超限时位置漂移、对地速度变化、耗电增加；关闭风力后线性回归原航线
- **紧急降落**：紧急停机为自由落体 `h = h₀ − ½gt²`，垂直速度 = −gt；紧急降落为可控下降 3m/s。风力和电量持续模拟，着陆后状态变更为 `emergency_stopped` / `emergency_landed`
- **runGeneration**：每次 `Start` / `ReturnHome` 递增，用于前端区分新旧轮次的遥测数据
- **状态来源**：飞行状态通过 state topic 的 `mode_code`（大疆标准）和 `flight_status`（仿真扩展）上报；OSD topic 仅含遥测数据不含状态
