# 低空飞行数据生成器 — 前端

基于 Vue 3 + TypeScript + CesiumJS 的无人机航线仿真 3D 可视化前端。

## 技术栈

- Vue 3.5 + TypeScript
- CesiumJS 1.143（天地图底图）
- MQTT.js WebSocket 客户端
- Vite 8 构建

## 开发

```bash
cd frontend
npm install
npm run dev
```

访问 `http://localhost:5171`。

Debug 模式（离线，不依赖后端）：`http://localhost:5171?debug=1`

## 构建

```bash
npm run build
# 产物在 dist/
```

## 目录结构

```
src/
├── App.vue                    # 主界面
├── style.css                  # 样式
└── composables/
    ├── useCesium.ts           # Cesium Viewer 初始化
    ├── useTrajectory.ts       # Entity 管理（无人机、轨迹、航点）
    ├── useMQTT.ts             # MQTT WebSocket 客户端
    └── useDebug.ts            # Debug 离线模式
```

## 依赖的后端服务

- HTTP API：`http://localhost:8081`（上传 KMZ、状态查询、速度/风力设置）
- MQTT WebSocket：`ws://localhost:1884`（遥测订阅 + 飞行指令）
