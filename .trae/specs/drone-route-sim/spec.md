# 无人机航线模拟与 Remote ID 可视化系统 Spec

## Why
大疆 KMZ 航线文件需要解析为可仿真的飞行轨迹，通过 MQTT 模拟 Remote ID 协议向外发送无人机实时状态，前端以 3D 方式可视化展示。系统需保留拓展其他航线格式的能力。

## What Changes
- 新增 `backend/` Go 后端项目，负责 KMZ 解析、轨迹生成、MQTT 遥测发布
- 新增 `frontend/` Vue 3 前端项目，负责 KMZ 上传、CesiumJS 3D 可视化、MQTT 遥测订阅
- 航线解析器采用 Strategy 模式，支持注册新格式解析器

## Impact
- Affected specs: 无（新项目）
- Affected code: 项目根目录新增 `backend/` 和 `frontend/` 两个子目录

## ADDED Requirements

### Requirement: KMZ 航线文件解析
系统 SHALL 解析大疆 WPML 格式的 KMZ 文件，提取航线航点（坐标、高度、速度、动作）和任务配置。

#### Scenario: 上传单个 KMZ 文件
- **WHEN** 用户通过前端上传一个 KMZ 文件
- **THEN** 后端解压 KMZ，解析 wpmz/template.kml 和 wpmz/waylines.wpml，返回解析后的航线信息

#### Scenario: 上传多个 KMZ 文件
- **WHEN** 用户同时上传多个 KMZ 文件
- **THEN** 每个文件独立解析，生成独立的飞行轨迹，可同时进行多机仿真

#### Scenario: KMZ 缺少必要文件
- **WHEN** KMZ 中不存在 wpmz/template.kml 或 wpmz/waylines.wpml
- **THEN** 返回明确错误信息，提示文件不完整

### Requirement: 通用航线解析器架构
系统 SHALL 提供可扩展的航线解析器注册机制，支持未来添加 GPX、KML、SRT 等格式。

#### Scenario: 注册新格式解析器
- **WHEN** 需要支持新的航线文件格式
- **THEN** 开发者实现 RouteParser 接口，注册到 ParserRegistry 即可

### Requirement: 轨迹域模型
系统 SHALL 定义统一的轨迹领域模型，包含航点序列、任务元信息和时间轴计算。

#### Scenario: 从航点生成插值轨迹
- **WHEN** 解析出航点序列后
- **THEN** 根据航点速度和距离计算出每个航段的时间，生成带时间戳的插值轨迹

### Requirement: 仿真引擎
系统 SHALL 提供时间驱动的仿真引擎，按固定频率推进飞行状态，通过 MQTT 发布实时遥测数据。

#### Scenario: 启动仿真
- **WHEN** 用户在前端点击"开始仿真"
- **THEN** 后端启动仿真 ticker（100ms 间隔），从起点开始推进轨迹，每个 tick 发布 MQTT 消息

#### Scenario: 暂停/继续仿真
- **WHEN** 用户点击暂停，再点击继续
- **THEN** 仿真引擎暂停 tick，保持当前位置；继续后从暂停点恢复推进

#### Scenario: 仿真完成
- **WHEN** 无人机到达最后一个航点
- **THEN** 自动停止仿真，发布最终状态并通知前端

### Requirement: Remote ID MQTT 遥测
系统 SHALL 通过 MQTT 以 Remote ID 格式发布无人机实时遥测数据。

#### Scenario: 实时位置发布
- **WHEN** 仿真引擎每个 tick 推进
- **THEN** 发布 `drone/{droneId}/telemetry` 主题，包含经纬度、椭球高度、海拔高度、速度、航向角、飞行状态

#### Scenario: 多机同时仿真
- **WHEN** 多个无人机同时进行仿真
- **THEN** 每台无人机使用独立的 MQTT topic（通过 droneId 区分），前端分别展示

### Requirement: Nginx 反向代理服务
系统 SHALL 通过 Nginx 将前端静态文件和后端 API 统一在同一端口下提供服务。

#### Scenario: 生产环境部署
- **WHEN** 在服务器上部署整个系统
- **THEN** 用户通过单一端口（如 80）访问前端页面和后端 API

### Requirement: 3D 可视化前端
系统 SHALL 基于 Vue 3 + CesiumJS 在前端以 3D 地球方式展示无人机位置、姿态和飞行轨迹。

#### Scenario: 页面加载
- **WHEN** 用户打开前端页面
- **THEN** 显示 Cesium 3D 地球，默认视角定位到中国区域

#### Scenario: 实时位置更新
- **WHEN** 收到 MQTT 遥测数据
- **THEN** Cesium Entity 的无人机模型实时更新位置和朝向，同时绘制飞行轨迹线

#### Scenario: 显示无人机状态面板
- **WHEN** 仿真运行中
- **THEN** 侧边栏显示当前无人机经纬度、高度、速度、航向角、当前航点索引等全部状态信息

#### Scenario: 轨迹预览
- **WHEN** KMZ 上传并解析成功后，仿真尚未开始
- **THEN** 在地球上绘制完整的规划航线（虚线），让用户预览

### Requirement: 多机管理
系统 SHALL 支持管理多个无人机仿真实例。

#### Scenario: 多机列表
- **WHEN** 上传了多个 KMZ 文件
- **THEN** 侧边栏显示所有无人机列表，可选择查看某台无人机或全部展示

### Requirement: debug 模式
系统 SHALL 提供 debug 模式，允许开发时快速预览而无需连接 MQTT broker。

#### Scenario: 开启 debug 模式
- **WHEN** 前端 URL 包含 `?debug=1` 或环境变量启用 debug
- **THEN** 前端使用模拟数据运行，不依赖后端和 MQTT broker
