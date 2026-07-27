import { ref, type Ref } from 'vue'

export interface DebugWaypoint {
  lng: number
  lat: number
  alt: number
  height: number
  speed: number
  heading: number
}

// 深圳地区模拟航线数据
const DEBUG_WAYPOINTS: DebugWaypoint[] = [
  { lng: 113.934844, lat: 22.531368, alt: 114.58, height: 118, speed: 10, heading: 270 },
  { lng: 113.934107, lat: 22.531355, alt: 114.58, height: 118, speed: 10, heading: 225 },
  { lng: 113.933501, lat: 22.531511, alt: 114.58, height: 118, speed: 10, heading: 180 },
  { lng: 113.933721, lat: 22.531893, alt: 114.58, height: 118, speed: 10, heading: 45 },
  { lng: 113.933992, lat: 22.531720, alt: 114.58, height: 118, speed: 10, heading: 90 },
  { lng: 113.934475, lat: 22.531574, alt: 114.58, height: 118, speed: 10, heading: 45 },
  { lng: 113.935010, lat: 22.531597, alt: 114.58, height: 118, speed: 10, heading: 0 },
]

export function useDebug() {
  const isDebug = ref(false)
  const debugIndex = ref(0)
  const debugRunning = ref(false)
  let timer: ReturnType<typeof setInterval> | null = null

  // 检查 URL 参数 ?debug=1
  function init() {
    const params = new URLSearchParams(window.location.search)
    isDebug.value = params.get('debug') === '1'
  }

  function startDebugLoop(
    onUpdate: (wp: DebugWaypoint, index: number, total: number) => void,
    speed: Ref<number> = ref(1)
  ) {
    if (debugRunning.value) return
    debugRunning.value = true
    debugIndex.value = 0

    timer = setInterval(() => {
      const idx = debugIndex.value
      if (idx >= DEBUG_WAYPOINTS.length) {
        debugIndex.value = 0 // 循环
      }
      const wp = DEBUG_WAYPOINTS[debugIndex.value]
      onUpdate(wp, debugIndex.value, DEBUG_WAYPOINTS.length)
      debugIndex.value++
    }, 1000 / speed.value) // 基础间隔 1 秒，速度倍率调整
  }

  function stopDebugLoop() {
    debugRunning.value = false
    if (timer) {
      clearInterval(timer)
      timer = null
    }
  }

  function getDebugWaypoints() {
    return DEBUG_WAYPOINTS
  }

  return { isDebug, debugRunning, init, startDebugLoop, stopDebugLoop, getDebugWaypoints }
}
