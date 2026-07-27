# Checklist

## 基础功能
- [x] 后端 Go module 初始化成功，`go build` 无报错
- [x] `POST /api/upload` 支持多文件上传，正确解析 KMZ 中的 template.kml 和 waylines.wpml
- [x] RouteParser 接口 + ParserRegistry 注册表，支持按扩展名注册和查找
- [x] 轨迹插值器根据航点速度和距离计算航段时间，生成正确的中间位置
- [x] 仿真引擎按 100ms tick 推进，支持启动/暂停/继续/停止/返航
- [x] MQTT 遥测数据 DJI OSD 兼容格式 + 简化格式双写
- [x] 多机仿真独立 topic，互不干扰
- [x] 飞行控制统一走 MQTT `thing/product/{id}/services`（大疆官方 `flight_task_*` 指令）
- [x] `GET /api/sim/status` 返回所有仿真实例运行状态
- [x] `GET /api/sim/trajectories` 获取已注册航线数据
- [x] 前端 Vue + CesiumJS 正常加载 3D 地球（天地图底图）
- [x] 前端上传 KMZ 并在地球上显示规划航线预览（黄色虚线）
- [x] 仿真开始后无人机实时更新位置/朝向，飞行轨迹线实时绘制
- [x] 侧边栏左右悬浮面板：左侧航线信息，右侧仿真控制
- [x] 航线列表管理，可选择单机或全局视角
- [x] MQTT 订阅正常接收遥测，驱动 3D 更新
- [x] Debug 模式（`?debug=1`）不依赖后端和 MQTT 独立运行

## 风力模拟
- [x] 风向圆盘可自由调节，风力输入框
- [x] 超过抗风限制时位置漂移、对地速度变化
- [x] 离地 < 5m 不偏移（避免地面漂移）
- [x] 关闭风力后线性回归原航线（非瞬间跳回）
- [x] 所有风速下根据逆风/侧风/顺风动态调整电量消耗（向量物理模型）
- [x] `POST /api/sim/wind` + MQTT `sim_set_wind` 双通道控制

## 仿真控制
- [x] `POST /api/sim/speed` + MQTT `sim_set_speed` 双通道控制仿真倍速
- [x] 停止后重新开始从起飞点出发（`Stop` 同步化 + `OriginalSimWaypoints` 备份恢复）
- [x] 返航后重新开始恢复原始 KMZ 航线（非返航路径）
- [x] 停止后旧遥测不干扰（`runGeneration` 过滤机制）
- [x] `DELETE /api/sim/drone/:id` 移除无人机

## UI/UX
- [x] 系统名称改为"低空飞行数据生成器"
- [x] "无人机列表"改为"航线列表"
- [x] 左右独立悬浮面板，面板可折叠
- [x] 左侧标题固定不随列表滚动
- [x] 自定义滚动条样式（与深色主题统一）
- [x] 右侧仿真控制面板高度自适应
- [x] 航线详情：机型、负载、起飞模式、结束动作、电量、固件、避障状态

## API 文档
- [x] `API.md` 完整 HTTP + MQTT 接口文档
- [x] `SDD_工程模块说明` 更新至 v1.1
