# 低空飞行数据生成器 — 参考文档

## 一、飞行器机型枚举

基于 DJI Cloud API 官方定义 ([产品支持](https://developer.dji.com/doc/cloud-api-tutorial/cn/overview/product-support.html))。

| 型号 | droneEnumValue | droneSubEnumValue | 规格 |
|------|:---:|:---:|------|
| Matrice 400 | 103 | 0 | 55min / 23m/s / 抗风 12m/s |
| Matrice 350 RTK | 89 | 0 | 55min / 23m/s / 抗风 12m/s |
| Matrice 300 RTK | 60 | 0 | 55min / 23m/s / 抗风 12m/s |
| Matrice 30 | 67 | 0 | 41min飞行/36min悬停 / 23m/s / 抗风 12m/s |
| Matrice 30T | 67 | 1 | 41min飞行/36min悬停 / 23m/s / 抗风 12m/s |
| Mavic 3E | 77 | 0 | 45min飞行/38min悬停 / 21m/s / 抗风 12m/s |
| Mavic 3T | 77 | 1 | 45min飞行/38min悬停 / 21m/s / 抗风 12m/s |
| Mavic 3M | 77 | 2 | 43min飞行/37min悬停 / 21m/s / 抗风 12m/s |
| Mavic 3TA | 77 | 3 | 45min飞行/38min悬停 / 21m/s / 抗风 12m/s |
| Matrice 3D | 91 | 0 | 45min飞行/38min悬停 / 21m/s / 抗风 12m/s |
| Matrice 3TD | 91 | 1 | 45min飞行/38min悬停 / 21m/s / 抗风 12m/s |
| Matrice 4E | 99 | 0 | 49min飞行/42min悬停 / 21m/s / 抗风 12m/s |
| Matrice 4T | 99 | 1 | 49min飞行/42min悬停 / 21m/s / 抗风 12m/s |
| Matrice 4D | 100 | 0 | 49min飞行/42min悬停 / 21m/s / 抗风 12m/s |
| Matrice 4TD | 100 | 1 | 49min飞行/42min悬停 / 21m/s / 抗风 12m/s |

## 二、负载相机枚举

基于 DJI Cloud API 相机枚举值。

| 负载名称 | payloadEnumValue | payloadSubEnumValue |
|----------|:---:|:---:|
| 禅思 H20 | 42 | 0 |
| 禅思 H20T | 43 | 0 |
| 禅思 H20N | 61 | 0 |
| 禅思 H30 | 82 | 0 |
| 禅思 H30T | 83 | 0 |
| Matrice 30 Camera | 52 | 0 |
| Matrice 30T Camera | 53 | 0 |
| Mavic 3E Camera | 66 | 0 |
| Mavic 3T Camera | 67 | 0 |
| Mavic 3TA Camera | 129 | 0 |
| Matrice 3D Camera | 80 | 0 |
| Matrice 3TD Camera | 81 | 0 |
| Matrice 4E Camera | 88 | 0 |
| Matrice 4T Camera | 89 | 0 |
| Matrice 4D Camera | 98 | 0 |
| Matrice 4TD Camera | 99 | 0 |

## 三、MQTT 飞行控制指令

发布 Topic：`thing/product/{droneSN}/services`

请求格式：
```json
{
  "tid": "uuid",
  "bid": "uuid",
  "timestamp": 1722096000000,
  "data": {
    "method": "指令名称",
    "speed": 1.0,
    "direction": 180
  }
}
```

回复 Topic：`thing/product/{droneSN}/services_reply`
```json
{ "tid": "uuid", "bid": "uuid", "timestamp": 1722096000000, "data": { "result": 0 } }
```
`result`: 0=成功, 1=失败。

### 航线控制指令（大疆标准）

| method | 对应动作 | 说明 |
|--------|---------|------|
| `flight_task_execute` | 开始执行航线 | 从 idle / paused 状态启动 |
| `flight_task_pause` | 暂停航线 | 仅 running / hovering 状态有效 |
| `flight_task_resume` | 恢复航线 | 仅 paused 状态有效 |
| `flight_task_terminate` | 停止航线 | 回到 idle，复位到起飞点 |
| `flight_task_return_home` | 返航 | 从当前位置飞回起飞点 |

### 仿真参数指令（扩展）

| method | 参数 | 说明 |
|--------|------|------|
| `sim_set_speed` | `data.speed` (倍率) | 设置仿真速度倍率 |
| `sim_set_wind` | `data.speed` (m/s) + `data.direction` (度) | 设置风速和风向 |

## 四、MQTT DRC 紧急控制指令

发布 Topic：`thing/product/{droneSN}/drc/down`

请求格式：
```json
{ "method": "指令名称", "seq": 1, "data": {} }
```

| method | 对应动作 | 物理效果 |
|--------|---------|----------|
| `drone_emergency_stop` | 紧急停机 | 电机断电，自由落体 `h = h₀ − ½gt²` |
| `drc_emergency_landing` | 紧急迫降 | 可控下降 3m/s |

> DRC 指令优先级高于航线，可打断正在执行的航线任务。

## 五、无人机状态枚举

### 内部状态

| 状态值 | 含义 | 说明 |
|--------|------|------|
| `idle` | 待命 | 初始状态 / 航线停止后 |
| `running` | 飞行中 | 正在按航线执行 |
| `paused` | 已暂停 | 航线执行暂停 |
| `completed` | 已完成 | 航线执行完毕 |
| `hovering` | 悬停中 | 在航点执行悬停动作 |
| `emergency_stop` | 紧急停机中 | 自由落体坠落中 |
| `emergency_landing` | 紧急迫降中 | 可控下降中 |
| `emergency_stopped` | 已坠毁 | 紧急停机后已着陆 |
| `emergency_landed` | 已迫降 | 紧急迫降后已着陆 |

### DJI mode_code 映射

| 内部状态 | mode_code | DJI 含义 |
|----------|:---:|------|
| `idle` / `paused` / `completed` / `emergency_stopped` / `emergency_landed` | 0 | Standby |
| `running` / `hovering` | 5 | Wayline |
| `emergency_landing` | 2 | Auto Landing |
| `emergency_stop` | 7 | Failsafe |

## 六、MQTT Topics 一览

| Topic | 方向 | 用途 |
|-------|:---:|------|
| `thing/product/{sn}/services` | 发送 → 后端 | DJI 飞行控制指令 |
| `thing/product/{sn}/services_reply` | 后端 → 回复 | 指令执行结果 |
| `thing/product/{sn}/drc/down` | 发送 → 后端 | DRC 紧急控制指令 |
| `thing/product/{sn}/osd` | 后端 → 订阅 | DJI 兼容遥测 (10Hz) |
| `thing/product/{sn}/state` | 后端 → 订阅 | 设备状态（上线/模式变更） |
| `drone/{sn}/telemetry` | 后端 → 订阅 | 简化格式遥测 (10Hz) |
| `drone/{sn}/status` | 后端 → 订阅 | 状态变更通知 |

## 七、外部平台集成流程

```
1. POST /api/upload  (multipart: files=@航线.kmz, droneSn=xxx)
   → 解析 KMZ → 注册无人机 → 发布 MQTT state (上线)
   → 返回航线数据 + droneSN

2. MQTT 发送 thing/product/{sn}/services
   {"method": "flight_task_execute"}
   → 后端开始仿真 → 发布 OSD 遥测 (10Hz)

3. 实时接收 OSD 遥测
   订阅 thing/product/{sn}/osd 或 drone/{sn}/telemetry

4. 实时接收状态变更
   订阅 thing/product/{sn}/state
```
