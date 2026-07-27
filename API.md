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

### 2.2 实时遥测（服务端发布）

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
    "vertical_speed": 0,
    "height": 100.0,
    "wind_speed": 5.0,
    "battery": { "capacity_percent": 85, "voltage": 24.8, "temperature": 35 },
    "position_state": { "is_fixed": 1, "quality": 5 },
    "flight_status": "running",
    "waypoint_index": 3,
    "total_waypoints": 10,
    "progress": 45.2,
    "current_action": "hover",
    "hover_time_remaining": 5.0,
    "run_generation": 2
  }
}
```

**status / flight_status 枚举**：

| 值 | 含义 |
|----|------|
| `idle` | 待命 |
| `running` | 飞行中 |
| `paused` | 已暂停 |
| `completed` | 已完成 |
| `hovering` | 悬停中 |

---

### 2.3 设备状态（服务端发布）

**Topic**：`thing/product/{droneId}/state`

触发时机：设备上线、飞行模式变更

```json
{
  "tid": "uuid",
  "bid": "uuid",
  "timestamp": 1722096000000,
  "gateway": "drone-xxx",
  "data": {
    "firmware_version": "v10.01.0000",
    "mode_code": 0,
    "battery": { "capacity_percent": 100, "voltage": 25.2, "temperature": 35 },
    "payloads": [{ "payload_index": "52-0-0", "firmware_version": "v04.00.0010" }],
    "cameras": [{ "payload_index": "52-0-0", "camera_mode": 0, "photo_state": 0, "recording_state": 0 }],
    "obstacle_avoidance": { "horizon": 1, "upside": 1, "downside": 1 },
    "night_lights_state": 1
  }
}
```

**mode_code 映射**：`idle/completed` → 0, `running/hovering` → 5。

---

## 三、典型调用流程

```
1. HTTP POST /api/upload          → 上传 KMZ，获取 droneId
2. MQTT 订阅 thing/product/{droneId}/osd     → 接收实时遥测
3. MQTT 订阅 thing/product/{droneId}/state   → 接收状态变更
4. MQTT 发送 flight_task_execute              → 开始执行航线
5. MQTT 发送 flight_task_pause               → 暂停
6. MQTT 发送 flight_task_resume              → 继续
7. MQTT 发送 flight_task_terminate           → 停止
8. MQTT 发送 flight_task_return_home          → 返航

参数调节：
  HTTP POST /api/sim/speed       → 设置仿真倍率
  HTTP POST /api/sim/wind        → 设置风力
  (也支持 MQTT sim_set_speed / sim_set_wind)
```

## 四、关键机制

- **遥测频率**：10Hz（100ms/帧），叠加风力漂移后的位置
- **电池模拟**：基于机型续航参数，飞行+悬停综合计算，爬升额外耗电 +8%/m/s
- **风力模拟**：风速 ≤ 抗风限制时无影响；超限时位置漂移、对地速度变化、耗电增加；关闭风力后线性回归原航线
- **runGeneration**：每次 `Start` / `ReturnHome` 递增，用于前端区分新旧轮次的遥测数据
