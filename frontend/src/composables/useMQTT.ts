import { ref, type Ref } from 'vue'
import mqtt, { type MqttClient } from 'mqtt'

export interface TelemetryData {
  droneId: string
  lat: number
  lng: number
  alt: number           // 椭球高度
  heightAboveTakeoff: number
  speed: number
  heading: number
  status: string         // "idle"|"running"|"paused"|"completed"
  timestamp: number
  waypointIndex: number
  totalWaypoints: number
  progress: number
  windSpeed?: number     // 当前风速 (m/s)
  windDirection?: number // 风向 (度, 0=北)
  windWarning?: boolean  // 是否超过抗风限制
}

type TelemetryCallback = (data: TelemetryData) => void
type RawMessageCallback = (topic: string, payload: any) => void

// MQTT 通配符匹配：+ 匹配单层，# 匹配剩余所有层
function topicMatches(pattern: string, topic: string): boolean {
  const patternParts = pattern.split('/')
  const topicParts = topic.split('/')

  for (let i = 0; i < patternParts.length; i++) {
    if (patternParts[i] === '#') return true // # 匹配剩余全部
    if (i >= topicParts.length) return false
    if (patternParts[i] !== '+' && patternParts[i] !== topicParts[i]) return false
  }
  return patternParts.length === topicParts.length
}

export function useMQTT(brokerUrl?: string) {
  const client: Ref<MqttClient | null> = ref(null)
  const connected: Ref<boolean> = ref(false)
  const subscribedTopics: Set<string> = new Set()
  const callbacks: Map<string, TelemetryCallback[]> = new Map()
  const rawCallbacks: Map<string, RawMessageCallback[]> = new Map()

  // 默认 broker 地址
  const broker = brokerUrl || 'ws://localhost:9001'

  function connect(): Promise<void> {
    return new Promise((resolve, reject) => {
      client.value = mqtt.connect(broker, {
        clientId: 'drone-sim-frontend-' + Math.random().toString(16).slice(2, 8),
        clean: true,
        reconnectPeriod: 3000,
      })

      client.value.on('connect', () => {
        connected.value = true
        console.log('[MQTT] 已连接到', broker)
        resolve()
      })

      client.value.on('error', (err) => {
        console.error('[MQTT] 错误:', err)
        reject(err)
      })

      client.value.on('close', () => {
        connected.value = false
        console.log('[MQTT] 连接关闭')
      })

      client.value.on('message', (topic: string, payload: Buffer) => {
        const rawPayload = JSON.parse(payload.toString())

        // 分发给简化格式回调（精确匹配）
        const cbs = callbacks.get(topic)
        if (cbs) {
          cbs.forEach(cb => cb(rawPayload))
        }

        // 分发给原始格式回调（支持 MQTT 通配符 + 和 #）
        rawCallbacks.forEach((rawCbs, pattern) => {
          if (topicMatches(pattern, topic)) {
            rawCbs.forEach(cb => cb(topic, rawPayload))
          }
        })
      })

      // 超时 fallback
      setTimeout(() => {
        if (!connected.value) {
          reject(new Error('连接超时'))
        }
      }, 5000)
    })
  }

  function subscribe(droneId: string, callback: TelemetryCallback) {
    if (!client.value) return

    const telemetryTopic = `drone/${droneId}/telemetry`
    const statusTopic = `drone/${droneId}/status`

    // 订阅遥测 topic
    if (!subscribedTopics.has(telemetryTopic)) {
      client.value.subscribe(telemetryTopic, { qos: 0 })
      subscribedTopics.add(telemetryTopic)
    }

    // 注册回调
    if (!callbacks.has(telemetryTopic)) {
      callbacks.set(telemetryTopic, [])
    }
    callbacks.get(telemetryTopic)!.push(callback)
  }

  function unsubscribe(droneId: string, callback?: TelemetryCallback) {
    if (!client.value) return

    const telemetryTopic = `drone/${droneId}/telemetry`
    
    if (callback) {
      const cbs = callbacks.get(telemetryTopic)
      if (cbs) {
        const idx = cbs.indexOf(callback)
        if (idx > -1) cbs.splice(idx, 1)
      }
    } else {
      // 取消所有回调
      callbacks.delete(telemetryTopic)
      client.value.unsubscribe(telemetryTopic)
      subscribedTopics.delete(telemetryTopic)
    }
  }

  // subscribeRaw 订阅任意 topic，原始 JSON 回调
  function subscribeRaw(topic: string, callback: RawMessageCallback) {
    if (!client.value) return

    if (!subscribedTopics.has(topic)) {
      client.value.subscribe(topic, { qos: 0 })
      subscribedTopics.add(topic)
    }

    if (!rawCallbacks.has(topic)) {
      rawCallbacks.set(topic, [])
    }
    rawCallbacks.get(topic)!.push(callback)
  }

  function disconnect() {
    if (client.value) {
      client.value.end()
      client.value = null
      connected.value = false
    }
  }

  function publish(topic: string, payload: object) {
    if (!client.value) return false
    client.value.publish(topic, JSON.stringify(payload), { qos: 0 })
    return true
  }

  return { connect, disconnect, subscribe, subscribeRaw, unsubscribe, publish, connected }
}
