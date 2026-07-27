<template>
  <div id="app">
    <!-- Cesium 容器 -->
    <div class="cesium-container">
      <div id="cesiumContainer"></div>

      <!-- 左侧悬浮面板：航线信息 -->
      <div class="float-panel float-left" :class="{ collapsed: collapsed }">
        <button class="float-toggle" @click="collapsed = !collapsed">
          {{ collapsed ? '▶' : '◀' }}
        </button>
        <div class="sidebar-header">
          <h2>低空飞行数据生成器</h2>
        </div>
        <!-- 上半部分：上传 + 列表 -->
        <div class="panel-top">
          <div class="upload-section">
            <h3>航线上传</h3>
            <label class="file-upload-btn">
              选择 KMZ 文件
              <input type="file" accept=".kmz" multiple @change="handleFileUpload" />
            </label>
          </div>
          <div class="drone-list">
            <h3>航线列表</h3>
            <div v-if="droneList.length === 0" class="empty">暂无无人机</div>
            <div v-for="drone in droneList" :key="drone.id"
                 class="drone-item" :class="{ active: selectedDroneId === drone.id }"
                 @click="selectDrone(drone.id)">
              <div class="drone-item-main">
                <span class="drone-status-dot" :class="drone.status"></span>
                <span class="drone-name">{{ drone.name }}</span>
                <span class="drone-speed">{{ drone.speed?.toFixed(1) || '0' }} m/s</span>
                <button class="drone-delete-btn" @click.stop="removeDrone(drone.id)" title="删除航线">×</button>
              </div>
              <div class="drone-item-sn">{{ drone.id }}</div>
            </div>
          </div>
        </div>
        <!-- 下半部分：无人机详情 -->
        <div class="panel-bottom">
        <div v-if="selectedDrone" class="status-panel">
          <h3>{{ selectedDrone.name }}</h3>
          <div class="status-badge" :class="selectedDrone.status">
            {{ statusText(selectedDrone.status) }}
          </div>
          <div v-if="selectedDrone.status === 'hovering' && selectedDrone.hoverTimeRemaining > 0" class="hover-timer">
            悬停剩余 {{ selectedDrone.hoverTimeRemaining.toFixed(1) }}s
          </div>
          <div v-if="selectedDrone.windWarning" class="wind-warning">
            风力超限！风速 {{ selectedDrone.windSpeed }} m/s > 抗风 {{ selectedDrone.maxWindResistance }} m/s，无人机偏离航线
          </div>
          <div class="meta-section">
            <div class="meta-row" v-if="selectedDrone.droneModelName">
              <span class="label">机型</span>
              <span class="value">{{ selectedDrone.droneModelName }}</span>
            </div>
            <div class="meta-row" v-if="selectedDrone.payloadModelName">
              <span class="label">负载</span>
              <span class="value">{{ selectedDrone.payloadModelName }}</span>
            </div>
            <div class="meta-row" v-if="selectedDrone.takeOffSecurityHeight">
              <span class="label">起飞安全高度</span>
              <span class="value">{{ selectedDrone.takeOffSecurityHeight }} m</span>
            </div>
            <div class="meta-row" v-if="selectedDrone.globalTransitionalSpeed">
              <span class="label">过渡爬升速度</span>
              <span class="value">{{ selectedDrone.globalTransitionalSpeed }} m/s</span>
            </div>
            <div class="meta-row" v-if="selectedDrone.autoFlightSpeed">
              <span class="label">航线飞行速度</span>
              <span class="value">{{ selectedDrone.autoFlightSpeed }} m/s</span>
            </div>
            <div class="meta-row" v-if="selectedDrone.totalDistance">
              <span class="label">航线总长</span>
              <span class="value">{{ (selectedDrone.totalDistance / 1000).toFixed(2) }} km</span>
            </div>
            <div class="meta-row" v-if="selectedDrone.finishAction">
              <span class="label">结束动作</span>
              <span class="value">{{ selectedDrone.finishAction === 'goHome' ? '返航' : selectedDrone.finishAction }}</span>
            </div>
            <div class="meta-row" v-if="selectedDrone.flyToWaylineMode">
              <span class="label">爬升模式</span>
              <span class="value">{{ selectedDrone.flyToWaylineMode === 'safely' ? '安全模式' : '倾斜飞行' }}</span>
            </div>
          </div>
          <div class="meta-section" v-if="selectedDrone.firmwareVersion || selectedDrone.batteryPercent > 0">
            <div class="meta-row" v-if="selectedDrone.firmwareVersion">
              <span class="label">固件版本</span>
              <span class="value">{{ selectedDrone.firmwareVersion }}</span>
            </div>
            <div class="meta-row" v-if="selectedDrone.batteryPercent > 0 || selectedDrone.windSpeed > 0">
              <span class="label">电池电量</span>
              <span class="value">{{ selectedDrone.batteryPercent }}%</span>
            </div>
            <div class="meta-row" v-if="selectedDrone.windSpeed > 0">
              <span class="label">风速</span>
              <span class="value" :class="{ 'wind-danger': selectedDrone.windWarning }">
                {{ selectedDrone.windSpeed }} m/s {{ windDirLabel(selectedDrone.windDirection) }}
                <span v-if="selectedDrone.windWarning">⚠</span>
              </span>
            </div>
            <div class="meta-row" v-if="selectedDrone.obstacleAvoidance">
              <span class="label">避障</span>
              <span class="value">{{ selectedDrone.obstacleAvoidance }}</span>
            </div>
          </div>
          <div class="status-grid">
            <div class="status-row">
              <span class="label">经度</span>
              <span class="value">{{ selectedDrone.lng?.toFixed(6) ?? '--' }}</span>
            </div>
            <div class="status-row">
              <span class="label">纬度</span>
              <span class="value">{{ selectedDrone.lat?.toFixed(6) ?? '--' }}</span>
            </div>
            <div class="status-row">
              <span class="label">椭球高度</span>
              <span class="value">{{ selectedDrone.alt?.toFixed(1) ?? '--' }} m</span>
            </div>
            <div class="status-row">
              <span class="label">海拔高度</span>
              <span class="value">{{ selectedDrone.heightAboveTakeoff?.toFixed(1) ?? '--' }} m</span>
            </div>
            <div class="status-row">
              <span class="label">飞行速度</span>
              <span class="value">{{ selectedDrone.speed?.toFixed(1) ?? '--' }} m/s</span>
            </div>
            <div class="status-row">
              <span class="label">航向角</span>
              <span class="value">{{ selectedDrone.heading?.toFixed(1) ?? '--' }}°</span>
            </div>
            <div class="status-row">
              <span class="label">当前航点</span>
              <span class="value">{{ selectedDrone.waypointIndex ?? '--' }} / {{ selectedDrone.totalWaypoints ?? '--' }}</span>
            </div>
            <div class="status-row">
              <span class="label">进度</span>
              <span class="value">{{ selectedDrone.progress?.toFixed(1) ?? '--' }}%</span>
            </div>
          </div>
          <div class="progress-bar">
            <div class="progress-fill" :style="{ width: (selectedDrone.progress || 0) + '%' }"></div>
          </div>
        </div>
        </div>
      </div>

      <!-- 右侧悬浮面板：仿真控制 -->
      <div class="float-panel float-right" :class="{ collapsed: collapsedRight }">
        <button class="float-toggle float-toggle-right" @click="collapsedRight = !collapsedRight">
          {{ collapsedRight ? '◀' : '▶' }}
        </button>
        <div class="control-panel">
          <h3>仿真控制</h3>
          <div class="control-buttons">
            <button class="ctrl-btn start" @click="startSimulation"
              :disabled="!selectedDroneId || selectedDrone?.status === 'running'">▶ 开始</button>
            <button class="ctrl-btn pause" @click="pauseSimulation"
              :disabled="!selectedDroneId || selectedDrone?.status !== 'running'">⏸ 暂停</button>
            <button class="ctrl-btn resume" @click="resumeSimulation"
              :disabled="!selectedDroneId || selectedDrone?.status !== 'paused'">▶ 继续</button>
            <button class="ctrl-btn stop" @click="stopSimulation"
              :disabled="!selectedDroneId || selectedDrone?.status === 'idle'">⏹ 停止</button>
            <button class="ctrl-btn rth" @click="returnHome"
              :disabled="!selectedDroneId || (selectedDrone?.status !== 'running' && selectedDrone?.status !== 'hovering')">🏠 返航</button>
          </div>
          <div class="speed-control">
            <label>仿真速度</label>
            <div class="speed-buttons">
              <button v-for="s in speeds" :key="s.value" class="speed-btn"
                :class="{ active: speed === s.value }" @click="setSpeed(s.value)">{{ s.label }}</button>
            </div>
          </div>
          <div class="wind-control">
            <label>风力模拟 <span class="wind-resist-tip">(抗风 {{ selectedDrone?.maxWindResistance || 12 }} m/s)</span></label>
            <div class="wind-dial-row">
              <!-- 风向罗盘 -->
              <div class="wind-compass"
                @mousedown="startWindDial"
                @mousemove="moveWindDial"
                @mouseup="endWindDial"
                @mouseleave="endWindDial"
                @touchstart.prevent="startWindDial"
                @touchmove.prevent="moveWindDial"
                @touchend="endWindDial">
                <svg viewBox="0 0 100 100" class="compass-svg">
                  <!-- 刻度标记 -->
                  <g v-for="deg in [0,45,90,135,180,225,270,315]" :key="deg">
                    <line
                      :x1="50 + 34 * Math.cos((deg - 90) * Math.PI / 180)"
                      :y1="50 + 34 * Math.sin((deg - 90) * Math.PI / 180)"
                      :x2="50 + 40 * Math.cos((deg - 90) * Math.PI / 180)"
                      :y2="50 + 40 * Math.sin((deg - 90) * Math.PI / 180)"
                      stroke="rgba(255,255,255,0.3)" stroke-width="1.5" stroke-linecap="round" />
                    <text
                      :x="50 + 27 * Math.cos((deg - 90) * Math.PI / 180)"
                      :y="50 + 27 * Math.sin((deg - 90) * Math.PI / 180)"
                      text-anchor="middle" dominant-baseline="central"
                      fill="rgba(255,255,255,0.5)" font-size="8">{{ deg === 0 ? 'N' : deg === 90 ? 'E' : deg === 180 ? 'S' : deg === 270 ? 'W' : '' }}</text>
                  </g>
                  <!-- 风向箭头 -->
                  <polygon
                    :points="windArrowPoints"
                    fill="#00ffff" />
                  <circle cx="50" cy="50" r="4" fill="#00ffff" opacity="0.6" />
                </svg>
              </div>
            </div>
            <div class="wind-input-row">
              <input type="number" v-model.number="windDirInput" class="wind-dir-input"
                :disabled="!selectedDroneId" min="0" max="359" step="1" @change="onWindDirChange" />
              <span class="wind-dir-unit">°</span>
              <input type="number" v-model.number="windSpeedInput" class="wind-speed-input"
                :disabled="!selectedDroneId" min="0" max="50" step="0.1" placeholder="风速" />
              <span class="wind-unit">m/s</span>
              <button class="ctrl-btn wind-apply" @click="applyWind(windSpeedInput)" :disabled="!selectedDroneId">应用</button>
            </div>
            <button class="ctrl-btn wind-off" @click="applyWind(0)" :disabled="!selectedDroneId || windSpeed === 0">关闭风力</button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, onUnmounted, ref, computed, type Ref } from 'vue'
import { useCesium } from './composables/useCesium'
import { useTrajectory, type TrajectoryData } from './composables/useTrajectory'
import { useDebug } from './composables/useDebug'
import { useMQTT } from './composables/useMQTT'
import axios from 'axios'

const { viewer, init: initCesium, destroy: destroyCesium } = useCesium('cesiumContainer')
const { addTrajectory, updateDronePosition, clearPreview, removeTrajectory, flyToTrajectory, resetPath, getTakeOffPoint } = useTrajectory(viewer)
const { isDebug, init: initDebug, startDebugLoop, stopDebugLoop, getDebugWaypoints } = useDebug()
const { connect: connectMQTT, subscribeRaw, publish: publishMQTT, disconnect: disconnectMQTT } = useMQTT('ws://localhost:1884')

const collapsed = ref(false)
const collapsedRight = ref(false)

const speed = ref(1)
const speeds = [
  { label: '1x', value: 1 },
  { label: '2x', value: 2 },
  { label: '5x', value: 5 },
]

function setSpeed(v: number) {
  speed.value = v
  if (selectedDroneId.value) {
    axios.post('/api/sim/speed', { droneId: selectedDroneId.value, speed: v }).catch(() => {})
  }
}

const windSpeed = ref(0)
const windDirection = ref(0)
const windSpeedInput = ref(0)
const windDirInput = ref(0)
const windDialActive = ref(false)

// 罗盘箭头顶点（风向来源方向，箭头指向圆心表示风从该方向来）
const windArrowPoints = computed(() => {
  const rad = (windDirection.value - 90) * Math.PI / 180
  const tip = 42; const base = 22; const half = 6
  const cx = 50; const cy = 50
  const x1 = cx + tip * Math.cos(rad)
  const y1 = cy + tip * Math.sin(rad)
  const x2 = cx + base * Math.cos(rad + Math.PI) + half * Math.cos(rad + Math.PI / 2)
  const y2 = cy + base * Math.sin(rad + Math.PI) + half * Math.sin(rad + Math.PI / 2)
  const x3 = cx + base * Math.cos(rad + Math.PI) + half * Math.cos(rad - Math.PI / 2)
  const y3 = cy + base * Math.sin(rad + Math.PI) + half * Math.sin(rad - Math.PI / 2)
  return `${x1},${y1} ${x2},${y2} ${x3},${y3}`
})

function getAngleFromEvent(e: MouseEvent | Touch) {
  const svg = (e.target as Element).closest('.wind-compass')?.querySelector('svg')
  if (!svg) return null
  const rect = svg.getBoundingClientRect()
  const cx = rect.left + rect.width / 2
  const cy = rect.top + rect.height / 2
  const angle = Math.atan2(e.clientY - cy, e.clientX - cx) * 180 / Math.PI + 90
  return ((angle % 360) + 360) % 360
}

function startWindDial(e: MouseEvent | TouchEvent) {
  if (!selectedDroneId.value) return
  windDialActive.value = true
  const ev = 'touches' in e ? e.touches[0] : e
  const angle = getAngleFromEvent(ev as MouseEvent)
  if (angle != null) {
    windDirection.value = Math.round(angle)
    windDirInput.value = windDirection.value
  }
}

function moveWindDial(e: MouseEvent | TouchEvent) {
  if (!windDialActive.value) return
  const ev = 'touches' in e ? e.touches[0] : e
  const angle = getAngleFromEvent(ev as MouseEvent)
  if (angle != null) {
    windDirection.value = Math.round(angle)
    windDirInput.value = windDirection.value
  }
}

function endWindDial() {
  windDialActive.value = false
}

function onWindDirChange() {
  let v = windDirInput.value
  if (isNaN(v)) v = 0
  v = ((v % 360) + 360) % 360
  windDirection.value = v
  windDirInput.value = v
}

function windDirLabel(deg: number): string {
  const dirs = ['北', '东北', '东', '东南', '南', '西南', '西', '西北']
  const idx = Math.round(deg / 45) % 8
  return dirs[idx]
}

function applyWind(s: number) {
  windSpeed.value = s
  windSpeedInput.value = s
  if (!selectedDroneId.value) return
  axios.post('/api/sim/wind', {
    droneId: selectedDroneId.value,
    speed: s,
    direction: windDirection.value,
  }).then(() => {
    const drone = droneList.value.find(d => d.id === selectedDroneId.value)
    if (drone) {
      drone.windSpeed = s
      drone.windDirection = windDirection.value
      drone.windWarning = s > (drone.maxWindResistance || 12)
      selectedDrone.value = drone
    }
  }).catch(() => {})
}

interface DroneInfo {
  id: string
  name: string
  lng: number
  lat: number
  alt: number
  heightAboveTakeoff: number
  speed: number
  heading: number
  status: string
  waypointIndex: number
  totalWaypoints: number
  progress: number
  currentAction: string
  hoverTimeRemaining: number
  // 航线元数据
  droneModelName: string
  payloadModelName: string
  takeOffSecurityHeight: number
  globalTransitionalSpeed: number
  autoFlightSpeed: number
  totalDistance: number
  finishAction: string
  flyToWaylineMode: string
  // 设备状态（来自 state topic）
  batteryPercent: number
  firmwareVersion: string
  obstacleAvoidance: string
  // 风力模拟
  windSpeed: number
  windDirection: number
  windWarning: boolean
  maxWindResistance: number
  // 仿真轮次（用于过滤旧遥测数据）
  runGeneration: number
}

function statusText(status: string): string {
  const map: Record<string, string> = {
    'idle': '待命',
    'running': '飞行中',
    'paused': '已暂停',
    'completed': '已完成',
    'hovering': '悬停中',
  }
  return map[status] || status
}

const droneList: Ref<DroneInfo[]> = ref([])
const selectedDroneId: Ref<string | null> = ref(null)
const selectedDrone = ref<DroneInfo | null>(null)

function selectDrone(id: string) {
  selectedDroneId.value = id
  selectedDrone.value = droneList.value.find((d) => d.id === id) || null
  flyToTrajectory(id)
}

async function removeDrone(id: string) {
  const drone = droneList.value.find(d => d.id === id)
  if (!drone) return

  // 如果在执行中，通过 MQTT 通知后端停止
  if (drone.status === 'running' || drone.status === 'paused') {
    sendFlightCommand(id, 'flight_task_terminate')
  }

  // 从后端引擎中彻底删除
  try {
    await axios.delete(`/api/sim/drone/${encodeURIComponent(id)}`)
  } catch (e) {
    // 忽略
  }

  // 清除选中状态
  if (selectedDroneId.value === id) {
    selectedDroneId.value = null
    selectedDrone.value = null
  }

  // 移除 Cesium 实体
  removeTrajectory(id)

  // 从列表中移除
  droneList.value = droneList.value.filter(d => d.id !== id)
}

async function handleFileUpload(e: Event) {
  const files = (e.target as HTMLInputElement).files
  if (!files) return

  const formData = new FormData()
  for (let i = 0; i < files.length; i++) {
    formData.append('files', files[i])
  }

  try {
    const res = await axios.post('/api/upload', formData)
    if (res.data.success && Array.isArray(res.data.data)) {
      for (const traj of res.data.data) {
        const tp: TrajectoryData = traj as TrajectoryData
        addTrajectory(tp)
        droneList.value.push({
          id: tp.id,
          name: tp.name,
          lng: tp.waypoints[0]?.lng || 0,
          lat: tp.waypoints[0]?.lat || 0,
          alt: tp.waypoints[0]?.ellipsoidHeight || 0,
          heightAboveTakeoff: 0,
          speed: 0,
          heading: 0,
          status: 'idle',
          waypointIndex: 0,
          totalWaypoints: tp.waypoints.length,
          progress: 0,
          currentAction: '',
          hoverTimeRemaining: 0,
          droneModelName: tp.droneModelName || '',
          payloadModelName: tp.payloadModelName || '',
          takeOffSecurityHeight: tp.takeOffSecurityHeight || 0,
          globalTransitionalSpeed: tp.globalTransitionalSpeed || 0,
          autoFlightSpeed: tp.autoFlightSpeed || 0,
          totalDistance: tp.totalDistance || 0,
          finishAction: tp.finishAction || '',
          flyToWaylineMode: tp.flyToWaylineMode || '',
          batteryPercent: 0,
          firmwareVersion: '',
          obstacleAvoidance: '',
          windSpeed: 0,
          windDirection: 0,
          windWarning: false,
          maxWindResistance: 12,
          runGeneration: 0,
        })

      // 通过 MQTT 获取实时遥测 + 设备状态
      try {
        await connectMQTT()
        for (const traj of res.data.data) {
          subscribeRaw(`thing/product/${traj.id}/osd`, (_topic, payload) => {
            handleOSDMessage(traj.id, payload)
          })
          subscribeRaw(`thing/product/${traj.id}/state`, (_topic, payload) => {
            handleStateMessage(traj.id, payload)
          })
        }
      } catch (mqttErr) {
        console.warn('MQTT 连接失败:', mqttErr)
      }
    }
  }
  } catch (err) {
    console.error('上传失败:', err)
  }
}

async function startSimulation() {
  if (!selectedDroneId.value) return

  // 重新开始仿真时清除旧轨迹并复位位置到起飞点
  resetPath(selectedDroneId.value)
  const drone = droneList.value.find((d) => d.id === selectedDroneId.value)
  const takeoff = getTakeOffPoint(selectedDroneId.value)
  if (drone && takeoff) {
    drone.lat = takeoff[0]
    drone.lng = takeoff[1]
    drone.alt = takeoff[2]
    drone.heightAboveTakeoff = 0
    drone.speed = 0
    drone.heading = 0
    drone.progress = 0
    drone.waypointIndex = 0
    drone.windWarning = false
  }

  // 调试模式直接本地循环
  if (isDebug.value) {
    const drone = droneList.value.find((d) => d.id === selectedDroneId.value)
    if (drone) {
      drone.status = 'running'
      selectedDrone.value = drone
    }
    startDebugLoop(
      (wp, index, total) => {
        updateDronePosition(selectedDroneId.value!, wp.lng, wp.lat, wp.alt, wp.heading)
        const d = droneList.value.find((d) => d.id === selectedDroneId.value)
        if (d) {
          d.lat = wp.lat
          d.lng = wp.lng
          d.alt = wp.alt
          d.heightAboveTakeoff = wp.height
          d.speed = wp.speed
          d.heading = wp.heading
          d.waypointIndex = index + 1
          d.totalWaypoints = total
          d.progress = ((index + 1) / total) * 100
        }
        if (selectedDroneId.value === selectedDroneId.value) {
          selectedDrone.value = droneList.value.find((d) => d.id === selectedDroneId.value) || null
        }
      },
      speed
    )
    return
  }

  sendFlightCommand(selectedDroneId.value, 'flight_task_execute')
}

async function pauseSimulation() {
  if (!selectedDroneId.value) return

  if (isDebug.value) {
    stopDebugLoop()
    const drone = droneList.value.find((d) => d.id === selectedDroneId.value)
    if (drone) {
      drone.status = 'paused'
      selectedDrone.value = drone
    }
    return
  }

  sendFlightCommand(selectedDroneId.value, 'flight_task_pause')
}

async function resumeSimulation() {
  if (!selectedDroneId.value) return

  if (isDebug.value) {
    const drone = droneList.value.find((d) => d.id === selectedDroneId.value)
    if (drone) {
      drone.status = 'running'
      selectedDrone.value = drone
    }
    startDebugLoop(
      (wp, index, total) => {
        updateDronePosition(selectedDroneId.value!, wp.lng, wp.lat, wp.alt, wp.heading)
        const d = droneList.value.find((d) => d.id === selectedDroneId.value)
        if (d) {
          d.lat = wp.lat
          d.lng = wp.lng
          d.alt = wp.alt
          d.heightAboveTakeoff = wp.height
          d.speed = wp.speed
          d.heading = wp.heading
          d.waypointIndex = index + 1
          d.totalWaypoints = total
          d.progress = ((index + 1) / total) * 100
        }
        if (selectedDroneId.value === selectedDroneId.value) {
          selectedDrone.value = droneList.value.find((d) => d.id === selectedDroneId.value) || null
        }
      },
      speed
    )
    return
  }

  sendFlightCommand(selectedDroneId.value, 'flight_task_resume')
}

async function stopSimulation() {
  if (!selectedDroneId.value) return

  if (isDebug.value) {
    stopDebugLoop()
    const debugWps = getDebugWaypoints()
    const drone = droneList.value.find((d) => d.id === selectedDroneId.value)
    if (drone) {
      drone.status = 'idle'
      drone.lng = debugWps[0].lng
      drone.lat = debugWps[0].lat
      drone.alt = debugWps[0].alt
      drone.heightAboveTakeoff = debugWps[0].height
      drone.speed = debugWps[0].speed
      drone.heading = debugWps[0].heading
      drone.waypointIndex = 0
      drone.progress = 0
      drone.windWarning = false
      selectedDrone.value = drone
    }
    updateDronePosition(selectedDroneId.value, debugWps[0].lng, debugWps[0].lat, debugWps[0].alt, debugWps[0].heading)
    resetPath(selectedDroneId.value)
    return
  }

  sendFlightCommand(selectedDroneId.value, 'flight_task_terminate')

  // 停止后立即复位前端显示位置到起飞点
  resetPath(selectedDroneId.value)
  const drone = droneList.value.find((d) => d.id === selectedDroneId.value)
  const takeoff = getTakeOffPoint(selectedDroneId.value)
  if (drone && takeoff) {
    drone.status = 'idle'
    drone.lat = takeoff[0]
    drone.lng = takeoff[1]
    drone.alt = takeoff[2]
    drone.heightAboveTakeoff = 0
    drone.speed = 0
    drone.heading = 0
    drone.progress = 0
    drone.waypointIndex = 0
    drone.windWarning = false
    if (selectedDroneId.value === drone.id) {
      selectedDrone.value = drone
    }
  }
}

// 返航：终止当前航线并从当前位置飞回起飞点
async function returnHome() {
  if (!selectedDroneId.value) return

  if (isDebug.value) {
    stopDebugLoop()
    const drone = droneList.value.find((d) => d.id === selectedDroneId.value)
    if (drone) drone.status = 'idle'
    return
  }

  sendFlightCommand(selectedDroneId.value, 'flight_task_return_home')
}

// 发送 MQTT 指令到 thing/product/{sn}/services
function sendFlightCommand(droneId: string, method: string, extra?: Record<string, any>) {
  const msg = {
    tid: crypto.randomUUID(),
    bid: crypto.randomUUID(),
    timestamp: Date.now(),
    data: { method, ...extra },
  }
  publishMQTT(`thing/product/${droneId}/services`, msg)
}

// 从后端恢复已注册的无人机（页面刷新后恢复状态）
async function restoreDrones() {
  try {
    const res = await axios.get('/api/sim/trajectories')
    if (!res.data.success || !Array.isArray(res.data.data)) return
    const trajs: TrajectoryData[] = res.data.data

    if (trajs.length === 0) return

    // 渲染所有无人机到 Cesium
    for (const traj of trajs) {
      addTrajectory(traj)
      const tp = traj
      // 避免重复添加（调试无人机可能已经存在）
      if (droneList.value.find(d => d.id === tp.id)) continue

      droneList.value.push({
        id: tp.id,
        name: tp.name,
        lng: tp.waypoints[0]?.lng || 0,
        lat: tp.waypoints[0]?.lat || 0,
        alt: tp.waypoints[0]?.height || 0,
        heightAboveTakeoff: 0,
        speed: 0,
        heading: 0,
        status: 'idle',
        waypointIndex: 0,
        totalWaypoints: tp.waypoints.length,
        progress: 0,
        currentAction: '',
        hoverTimeRemaining: 0,
        droneModelName: tp.droneModelName || '',
        payloadModelName: tp.payloadModelName || '',
        takeOffSecurityHeight: tp.takeOffSecurityHeight || 0,
        globalTransitionalSpeed: tp.globalTransitionalSpeed || 0,
        autoFlightSpeed: tp.autoFlightSpeed || 0,
        totalDistance: tp.totalDistance || 0,
        finishAction: tp.finishAction || '',
        flyToWaylineMode: tp.flyToWaylineMode || '',
        batteryPercent: 0,
        firmwareVersion: '',
        obstacleAvoidance: '',
        windSpeed: 0,
        windDirection: 0,
        windWarning: false,
        maxWindResistance: 12,
        runGeneration: 0,
      })
    }

    // 立即获取一次当前状态，更新位置、运行状态、速度倍率
    try {
      const statusRes = await axios.get('/api/sim/status')
      if (statusRes.data.success && Array.isArray(statusRes.data.data)) {
        for (const status of statusRes.data.data) {
          const drone = droneList.value.find(d => d.id === status.id)
          if (!drone) continue
          drone.status = status.status
          drone.progress = status.progress
          drone.lat = status.lat
          drone.lng = status.lng
          drone.alt = status.alt
          drone.speed = status.speed
          drone.heading = status.heading
          drone.heightAboveTakeoff = status.heightAboveTakeoff || 0
          drone.currentAction = status.currentAction || ''
          drone.hoverTimeRemaining = status.hoverTimeRemaining || 0
          drone.windSpeed = status.windSpeed || 0
          drone.windDirection = status.windDirection || 0
          drone.windWarning = status.windWarning || false
          drone.maxWindResistance = status.maxWindResistance || 12
          drone.runGeneration = status.runGeneration || 0
          updateDronePosition(status.id, status.lng, status.lat, status.alt, status.heading)
          // 同步速度倍率
          if (status.speedMultiplier && status.speedMultiplier !== speed.value) {
            speed.value = status.speedMultiplier
          }
        }
      }
    } catch { /* ignore */ }

    // 连接 MQTT 订阅实时遥测 + 设备状态
    try {
      await connectMQTT()
      for (const traj of trajs) {
        subscribeRaw(`thing/product/${traj.id}/osd`, (_topic, payload) => {
          handleOSDMessage(traj.id, payload)
        })
        subscribeRaw(`thing/product/${traj.id}/state`, (_topic, payload) => {
          handleStateMessage(traj.id, payload)
        })
      }
    } catch (mqttErr) {
      console.warn('MQTT 连接失败:', mqttErr)
    }
  } catch (err) {
    console.warn('恢复无人机状态失败:', err)
  }
}

function handleOSDMessage(droneId: string, payload: any) {
  const d = payload as any
  if (!d?.data) return

  const osd = d.data
  const actualDroneId = d.gateway || droneId
  const drone = droneList.value.find(d => d.id === actualDroneId)
  if (!drone) return

  // 用 runGeneration 过滤旧仿真遥测（避免停止后旧数据导致位置偏移）
  const incomingGen = osd.run_generation
  if (incomingGen != null) {
    if (incomingGen < drone.runGeneration) {
      return // 旧轮次的遥测，丢弃
    }
    if (incomingGen > drone.runGeneration) {
      drone.runGeneration = incomingGen // 更新为新轮次
    }
  }

  // 仅在活跃飞行状态下更新位置和动态数据
  const flightStatus = osd.flight_status
  const isActive = flightStatus === 'running' || flightStatus === 'hovering' || flightStatus === 'paused'

  // 位置更新（仅当坐标有效且处于活跃飞行状态时）
  if (osd.longitude && osd.latitude && isActive) {
    updateDronePosition(actualDroneId, osd.longitude, osd.latitude, osd.altitude ?? 0, osd.attitude_yaw)
    drone.lat = osd.latitude
    drone.lng = osd.longitude
    drone.alt = osd.altitude ?? drone.alt
  }

  if (isActive) {
    drone.heightAboveTakeoff = osd.height ?? drone.heightAboveTakeoff
    drone.speed = osd.horizontal_speed ?? drone.speed
    drone.heading = osd.attitude_yaw ?? drone.heading
    drone.waypointIndex = osd.waypoint_index ?? drone.waypointIndex
    drone.totalWaypoints = osd.total_waypoints ?? drone.totalWaypoints
    drone.progress = osd.progress ?? drone.progress
    drone.currentAction = osd.current_action || ''
    drone.hoverTimeRemaining = osd.hover_time_remaining ?? drone.hoverTimeRemaining
    drone.batteryPercent = osd.battery?.capacity_percent ?? drone.batteryPercent
    // 风力数据
    if (osd.wind_speed != null) drone.windSpeed = osd.wind_speed
    if (osd.wind_direction != null) drone.windDirection = osd.wind_direction
    if (osd.wind_warning != null) drone.windWarning = osd.wind_warning
  }

  // 状态更新
  if (flightStatus) {
    drone.status = flightStatus
  }

  if (selectedDroneId.value === actualDroneId) {
    selectedDrone.value = drone
  }
}

// 处理 thing/product/{id}/state 设备状态消息
function handleStateMessage(droneId: string, payload: any) {
  const d = payload as any
  if (!d?.data) return
  const state = d.data
  const actualDroneId = d.gateway || droneId
  const drone = droneList.value.find(d => d.id === actualDroneId)
  if (!drone) return

  drone.firmwareVersion = state.firmware_version || drone.firmwareVersion
  drone.batteryPercent = state.battery?.capacity_percent ?? drone.batteryPercent
  if (state.obstacle_avoidance) {
    const oa = state.obstacle_avoidance
    const parts: string[] = []
    if (oa.horizon) parts.push('水平')
    if (oa.upside) parts.push('上方')
    if (oa.downside) parts.push('下方')
    drone.obstacleAvoidance = parts.length > 0 ? parts.join('/') : ''
  }
  if (state.mode_code != null) {
    drone.status = modeCodeToStatus(state.mode_code, drone.status)
  }

  if (selectedDroneId.value === actualDroneId) {
    selectedDrone.value = drone
  }
}

function modeCodeToStatus(code: number, current: string): string {
  if (code === 5) return 'running' // wayline flight
  // mode_code=0 可能是 idle/paused/completed，保持 OSD 中更准确的状态
  return current || 'idle'
}

onMounted(async () => {
  initCesium()
  initDebug()

  if (isDebug.value) {
      const debugWps = getDebugWaypoints()
      const firstWp = debugWps[0]
      const debugTraj: TrajectoryData = {
        id: 'debug-drone',
        name: '调试无人机',
        waypoints: debugWps.map((wp) => ({
          lng: wp.lng,
          lat: wp.lat,
          height: wp.alt,
          index: 0,
          ellipsoidHeight: wp.alt,
          speed: wp.speed,
          heading: wp.heading,
        })),
        takeOffPoint: [firstWp.lat, firstWp.lng, firstWp.alt],
      }
    addTrajectory(debugTraj)
    droneList.value.push({
      id: 'debug-drone',
      name: '调试无人机',
      lng: debugWps[0].lng,
      lat: debugWps[0].lat,
      alt: debugWps[0].alt,
      heightAboveTakeoff: debugWps[0].height,
      speed: debugWps[0].speed,
      heading: debugWps[0].heading,
      status: 'idle',
      waypointIndex: 0,
      totalWaypoints: debugWps.length,
      progress: 0,
      windSpeed: 0,
      windDirection: 0,
      windWarning: false,
      maxWindResistance: 12,
      runGeneration: 0,
    })
    selectDrone('debug-drone')
  }

  // 从后端恢复已注册的无人机（页面刷新后恢复状态）
  await restoreDrones()
})

onUnmounted(() => {
  disconnectMQTT()
  stopDebugLoop()
  destroyCesium()
})
</script>
