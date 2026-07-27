# 系统设计文档（SDD）— 工程模块说明 v1.1

> **文档状态:** 已定稿
> **版本:** v1.1
> **作者:** drone-sim 项目组
> **所属模块:** 全栈（前端 + 后端服务 + MQTT 通信）
> **更新日期:** 2026-07-27
> **关联文档:** `API.md`

---

## 1. 模块概述

本系统是一个**低空飞行数据生成器**，支持上传大疆 KMZ 航线文件后进行时间驱动的仿真飞行，通过 MQTT 以 DJI Remote ID 兼容格式实时发布遥测数据，前端基于 CesiumJS 三维地球可视化展示。系统支持多机仿真、风力模拟、风速对电量影响、仿真生命周期控制。

### 1.1 模块定位与边界

- **模块名称**：低空飞行数据生成器
- **核心职责**：解析 DJI KMZ 航线文件 → 时间驱动仿真 → MQTT 发布遥测 → 3D 可视化
- **上下游依赖**：
  - 上游：用户上传的 DJI KMZ 航线文件（WPML 格式）
  - 下游：外部系统可通过 MQTT Broker 订阅遥测数据
- **核心设计原则**：
  - **数据单向流动**：KMZ → 轨迹域模型 → 仿真引擎 → MQTT 推送 → 前端渲染
  - **可扩展解析架构**：Strategy 模式，新增格式只需实现接口并注册
  - **零外部依赖运行**：内嵌 MQTT Broker，无需 Mosquitto

---

## 2. 技术架构与选型

| 层级/组件 | 技术选型及版本 | **选型理由** |
|:---|:---|:---|
| **前端框架** | Vue 3.5 + TypeScript 6.0 | Composition API 封装 Cesium/MQTT 逻辑，组合式函数 |
| **3D 渲染** | CesiumJS 1.143 | WGS84 坐标系原生支持，天地图底图替代不可用的 Cesium Ion |
| **后端框架** | Go 1.26 + Gin 1.12 | goroutine 驱动多机仿真，高性能 HTTP 路由 |
| **实时通信** | MQTT 3.1.1（TCP + WebSocket） | 10Hz 遥测推送，DJI 协议兼容 |
| **内嵌 Broker** | mochi-mqtt/server v2.7.9 | 编译进二进制，TCP :1883 + WS :1884 |
| **MQTT 客户端** | Paho MQTT v1.5（Go）/ mqtt.js v5.15（前端） | Go 端发布+订阅指令，浏览器 WebSocket 订阅 |
| **构建工具** | Vite 8 + vue-tsc | HMR 开发，`vite-plugin-cesium` 处理 Cesium 静态资源 |
| **数据存储** | 纯内存（无持久化） | 航线数据存内存，刷新后通过 API 恢复 |

---

## 3. 模块内部架构

### 3.1 目录结构

```
drone-sim/
├── backend/
│   ├── cmd/server/main.go            # 入口层：组装依赖、注册路由、启动 Broker
│   └── internal/
│       ├── config/config.go          # 配置层：.env 环境变量
│       ├── parser/                   # 解析层：KMZ 文件解析
│       │   ├── parser.go             #   RouteParser 接口 + 注册表
│       │   └── kmz.go                #   DJI KMZ 解析实现
│       ├── trajectory/trajectory.go  # 领域模型：Waypoint、Trajectory、RemoteIDTelemetry、DroneModelSpec
│       ├── engine/                   # 仿真引擎层
│       │   ├── engine.go             #   仿真核心（ticker、多机、悬停、电量、风力、返航、停止同步化）
│       │   └── interpolator.go       #   轨迹插值（Haversine、贝塞尔弧线、风力漂移计算）
│       └── mqtt/                     # 通信层
│           ├── broker.go             #   内嵌 MQTT Broker
│           └── publisher.go          #   遥测发布（DJI OSD 兼容 + 简化格式）
├── frontend/
│   ├── src/
│   │   ├── App.vue                   # 主界面（左右悬浮面板 + Cesium 容器）
│   │   ├── style.css                 # 深色主题样式
│   │   └── composables/
│   │       ├── useCesium.ts          #   Cesium Viewer 初始化（天地图底图）
│   │       ├── useTrajectory.ts      #   Entity 管理（无人机、轨迹、航点标记、路径重置）
│   │       ├── useMQTT.ts            #   MQTT WebSocket 客户端
│   │       └── useDebug.ts           #   Debug 离线模式
│   └── vite.config.ts
└── API.md                            # 对外 API 文档
```

### 3.2 核心模块职责

| 组件 | 文件 | 核心职责 |
|:---|:---|:---|
| **KMZParser** | `backend/internal/parser/kmz.go` | 解压 KMZ → 解析 KML/WPML → 生成 Trajectory |
| **Engine** | `backend/internal/engine/engine.go` | 仿真核心：100ms ticker、多机管理、风力模拟、电量计算、同步停止、原始航线备份恢复 |
| **Interpolator** | `backend/internal/engine/interpolator.go` | Haversine 距离、航段插值、贝塞尔弧线、风力漂移 `ComputeWindDrift` |
| **Publisher** | `backend/internal/mqtt/publisher.go` | 双写遥测（内嵌+外部 Broker），DJI 兼容格式 |
| **App.vue** | `frontend/src/App.vue` | 主界面：KMZ 上传、航线列表、无人机详情、仿真控制面板、风力罗盘、状态恢复 |
| **useTrajectory** | `frontend/src/composables/useTrajectory.ts` | Cesium Entity 全生命周期、路径重置 `resetPath`、起飞点获取 |

---

## 4. 核心数据流

### 4.1 上传 KMZ 并启动仿真

```
用户选择 KMZ → POST /api/upload (multipart/form-data)
  → KMZParser 解压解析 → Engine.AddDrone() 注册
  → 返回 Trajectory JSON → 前端 addTrajectory() 绘制预览
  → 前端 MQTT 订阅 thing/product/{id}/osd
用户点击开始 → MQTT flight_task_execute
  → Engine.Start() 启动 goroutine
  → 每 100ms tick: 插值 → 风力计算 → 遥测发布
  → 前端 handleOSDMessage 更新位置/状态
```

### 4.2 仿真控制（当前实际实现）

飞行控制（开始/暂停/继续/停止/返航）统一走 **MQTT** `thing/product/{droneId}/services`：

| method | 功能 | 协议 |
|--------|------|------|
| `flight_task_execute` | 开始 | 大疆官方 |
| `flight_task_pause` | 暂停 | 大疆官方 |
| `flight_task_resume` | 继续 | 大疆官方 |
| `flight_task_terminate` | 停止 | 大疆官方 |
| `flight_task_return_home` | 返航 | 大疆官方 |
| `sim_set_speed` | 设置仿真倍速 | 自定义（HTTP+MQTT 双通道） |
| `sim_set_wind` | 设置风力 | 自定义（HTTP+MQTT 双通道） |

### 4.3 风力模拟流程

```
每 tick:
  空速向量 = 地速 − 风速（向量减法）
  耗电倍数 windPwr = |空速| / |地速|（上下限 0.5~2.0）
  电量消耗 = 基础消耗 × windPwr × 爬升权重

  if 风速 > 最大抗风:
    累计漂移 += 超额风速 × dt 的经纬偏移
    对地速度 = 空速 + 超额风速
    离地 < 5m 时不偏移（避免地面漂移）

  if 风速 ≤ 抗风 && 有累计漂移:
    以当前航线速度逐步回收漂移，线性回归原航线
```

### 4.4 遥测数据过滤

后端每轮仿真递增 `RunGeneration`，遥测携带此值。前端 `handleOSDMessage` 比较 `run_generation`，丢弃旧轮次数据，防止停止后旧遥测导致位置跳变。

---

## 5. 接口定义

详见 [API.md](file:///c:/Users/13210/Desktop/drone-sim/API.md)。

| 方法 | 路径 | 功能 |
|:---|:---|:---|
| POST | `/api/upload` | 上传 KMZ 航线文件 |
| GET | `/api/sim/status` | 所有无人机状态 |
| GET | `/api/sim/trajectories` | 已注册航线数据 |
| POST | `/api/sim/speed` | 设置仿真速度倍率 |
| POST | `/api/sim/wind` | 设置风力参数 |
| DELETE | `/api/sim/drone/:id` | 移除无人机 |
| MQTT | `thing/product/{id}/osd` | 实时遥测（10Hz，DJI 兼容） |
| MQTT | `thing/product/{id}/state` | 设备状态 |
| MQTT | `thing/product/{id}/services` | 飞行控制指令 |

> 注：`/api/sim/start|pause|resume|stop` 已移除，统一改用 MQTT。

---

## 6. 部署

**环境变量**（`backend/.env`）：
```
SERVER_PORT=:8080
MQTT_BROKER=tcp://localhost:1883
MQTT_CLIENT_ID=drone-sim-server
```

**启动**：
```bash
# 后端
cd backend && go run ./cmd/server
# 前端
cd frontend && npm run dev
```

内嵌 MQTT Broker 自动启动，TCP :1883 + WebSocket :1884，无需外部依赖。

---

## 7. 关键机制

- **遥测频率**：10Hz（100ms/帧）
- **同步停止**：`Stop()` 先停 ticker 再设 idle，确保调用返回后状态确定
- **原始航线保护**：`AddDrone` 保存 `OriginalSimWaypoints`，`Start` 恢复，防止返航后航线被替换
- **风力耗电**：向量物理模型，逆风/侧风/顺风全覆盖
- **漂移回收**：关闭风力后按航线速度线性回归，非瞬间跳回
- **离地保护**：高度 < 起飞点+5m 时风力漂移不生效
- **runGeneration**：区分新旧轮次遥测，防止停止后旧数据干扰

---

## 8. 附录

### 机型规格

| 型号 | 最大飞行 | 最大悬停 | 水平速度 | 抗风 |
|------|----------|----------|----------|------|
| Matrice 350 RTK | 55min | 55min | 23m/s | 12m/s |
| Matrice 300 RTK | 55min | 55min | 23m/s | 12m/s |
| Matrice 30/30T | 41min | 36min | 23m/s | 12m/s |
| Mavic 3E/3T | 45min | 38min | 21m/s | 12m/s |
| Mavic 3M | 43min | 37min | 21m/s | 12m/s |
| Matrice 4E/4T | 49min | 42min | 21m/s | 12m/s |

### 电量消耗公式

```
电量% = 100 × (1 − effectiveTime / maxFlightTime)
effectiveTime = 飞行秒数 × windPwr × 爬升权重 + 悬停秒数 × (maxFlight/maxHover)
windPwr = |空速| / |地速|（0.5~2.0 区间）
爬升权重 = 1.0 + verticalSpeed × 0.08（爬升时），0.75（下降时），1.0（平飞时）
```
