import { ref, type Ref } from 'vue'
import {
  Viewer,
  Entity,
  Cartesian3,
  Color,
  CallbackProperty,
  BillboardGraphics,
  PolylineDashMaterialProperty,
  PointGraphics,
  LabelGraphics,
  PolylineGraphics,
  type PositionProperty,
  HorizontalOrigin,
  VerticalOrigin,
  Math as CesiumMath,
} from 'cesium'

export interface Waypoint {
  index: number
  lng: number
  lat: number
  height: number
  ellipsoidHeight: number
  speed: number
  heading: number
}

export interface TrajectoryData {
  id: string
  name: string
  waypoints: Waypoint[]
  takeOffPoint: [number, number, number] // lat,lng,alt
  droneModelName: string
  payloadModelName: string
  takeOffSecurityHeight: number
  globalTransitionalSpeed: number
  autoFlightSpeed: number
  totalDistance: number
  finishAction: string
  flyToWaylineMode: string
}

interface TrajectoryEntry {
  data: TrajectoryData
  droneEntity: Entity
  pathEntity: Entity
  previewEntity: Entity | null
  takeOffEntity: Entity
  waypointEntities: Entity[]
  pathPositions: Cartesian3[] // 轨迹点累积数组
  _pathVersion: { v: number } // 版本号，用于 CallbackProperty 检测变化
}

export function useTrajectory(viewer: Ref<Viewer | null>) {
  const trajectories: Ref<Map<string, TrajectoryEntry>> = ref(new Map())

  function getViewer(): Viewer {
    if (!viewer.value) {
      throw new Error('Viewer is not initialized')
    }
    return viewer.value
  }

  // 绘制规划航线预览（虚线），在仿真开始前显示
  function drawPreview(traj: TrajectoryData): Entity {
    const v = getViewer()
    const positions = traj.waypoints.map((wp) =>
      Cartesian3.fromDegrees(wp.lng, wp.lat, wp.ellipsoidHeight)
    )

    const previewEntity = v.entities.add({
      name: `${traj.name}-preview`,
      polyline: {
        positions: positions,
        width: 2,
        material: new PolylineDashMaterialProperty({
          color: Color.YELLOW.withAlpha(0.7),
          dashLength: 16,
        }),
        clampToGround: false,
      },
      show: true,
    })

    return previewEntity
  }

  // 生成无人机箭头图标（canvas → data URI），模块级单例避免重复创建
let _arrowImage: string | null = null
function getArrowImage(): string {
  if (_arrowImage) return _arrowImage
  const canvas = document.createElement('canvas')
  canvas.width = 32
  canvas.height = 32
  const ctx = canvas.getContext('2d')!
  ctx.fillStyle = '#00ffff'
  ctx.strokeStyle = '#008899'
  ctx.lineWidth = 1.5
  ctx.beginPath()
  ctx.moveTo(16, 3)   // 顶点
  ctx.lineTo(26, 26)  // 右下
  ctx.lineTo(16, 19)  // 凹口
  ctx.lineTo(6, 26)   // 左下
  ctx.closePath()
  ctx.fill()
  ctx.stroke()
  _arrowImage = canvas.toDataURL()
  return _arrowImage
}

// 创建无人机 Entity（箭头 + label）
function createDroneEntity(traj: TrajectoryData): Entity {
    const v = getViewer()
    // 起飞点存在则从起飞点开始，否则从第一个航点开始
    const [takeoffLat, takeoffLng, takeoffAlt] = traj.takeOffPoint
    let startLng: number, startLat: number, startHeight: number
    if (takeoffLat !== 0 || takeoffLng !== 0) {
      startLat = takeoffLat
      startLng = takeoffLng
      startHeight = takeoffAlt > 0 ? takeoffAlt : (traj.waypoints[0]?.ellipsoidHeight || 0)
    } else {
      const firstWp = traj.waypoints[0]
      startLng = firstWp.lng
      startLat = firstWp.lat
      startHeight = firstWp.ellipsoidHeight
    }
    const position = Cartesian3.fromDegrees(startLng, startLat, startHeight)

    const droneEntity = v.entities.add({
      name: traj.name,
      position: position,
      billboard: new BillboardGraphics({
        image: getArrowImage(),
        width: 24,
        height: 24,
        rotation: 0,
        verticalOrigin: VerticalOrigin.CENTER,
        horizontalOrigin: HorizontalOrigin.CENTER,
      }),
      label: new LabelGraphics({
        text: traj.name,
        font: '14px sans-serif',
        fillColor: Color.WHITE,
        outlineColor: Color.BLACK,
        outlineWidth: 2,
        style: 1, // FILL_AND_OUTLINE
        pixelOffset: new Cartesian3(0, -20),
        horizontalOrigin: HorizontalOrigin.CENTER,
        verticalOrigin: VerticalOrigin.BOTTOM,
      }),
      show: true,
    })

    return droneEntity
  }

  // 创建飞行轨迹线 Entity（使用 CallbackProperty 避免闪烁）
  function createPathEntity(traj: TrajectoryData, pathPositions: Cartesian3[], versionRef: { v: number }): Entity {
    const v = getViewer()
    const [takeoffLat, takeoffLng, takeoffAlt] = traj.takeOffPoint
    let startLng: number, startLat: number, startHeight: number
    if (takeoffLat !== 0 || takeoffLng !== 0) {
      startLat = takeoffLat
      startLng = takeoffLng
      startHeight = takeoffAlt > 0 ? takeoffAlt : (traj.waypoints[0]?.ellipsoidHeight || 0)
    } else {
      const firstWp = traj.waypoints[0]
      startLng = firstWp.lng
      startLat = firstWp.lat
      startHeight = firstWp.ellipsoidHeight
    }
    const initialPosition = Cartesian3.fromDegrees(startLng, startLat, startHeight)

    // 缓存上次渲染的版本号和位置数组，只在版本变化时重建
    let lastVersion = -1
    let cachedPositions: Cartesian3[] = [initialPosition]

    const pathEntity = v.entities.add({
      name: `${traj.name}-path`,
      polyline: new PolylineGraphics({
        positions: new CallbackProperty(() => {
          if (versionRef.v !== lastVersion) {
            lastVersion = versionRef.v
            cachedPositions = pathPositions.length >= 2 ? pathPositions.slice() : [initialPosition]
          }
          return cachedPositions
        }, false),
        width: 2,
        material: Color.fromCssColorString('#00ffff').withAlpha(0.8),
        clampToGround: false,
      }),
      show: true,
    })

    return pathEntity
  }

  // 创建航点标记（序号圆点 + label）
  function createWaypointMarkers(traj: TrajectoryData): Entity[] {
    const v = getViewer()
    const entities: Entity[] = []

    for (const wp of traj.waypoints) {
      const position = Cartesian3.fromDegrees(wp.lng, wp.lat, wp.ellipsoidHeight)
      const entity = v.entities.add({
        name: `${traj.name}-wp-${wp.index}`,
        position: position,
        point: new PointGraphics({
          pixelSize: 6,
          color: Color.YELLOW,
          outlineColor: Color.YELLOW.withAlpha(0.4),
          outlineWidth: 2,
        }),
        label: new LabelGraphics({
          text: String(wp.index),
          font: '11px sans-serif',
          fillColor: Color.YELLOW,
          outlineColor: Color.BLACK,
          outlineWidth: 2,
          style: 1,
          pixelOffset: new Cartesian3(0, -14),
          horizontalOrigin: HorizontalOrigin.CENTER,
          verticalOrigin: VerticalOrigin.BOTTOM,
        }),
        show: true,
      })
      entities.push(entity)
    }

    return entities
  }

  // 创建起飞点实体（绿色旗标）
  function createTakeOffEntity(traj: TrajectoryData): Entity {
    const v = getViewer()
    const [lat, lng, alt] = traj.takeOffPoint
    // 起飞点高度可能为0，给一个可见的高程偏移
    const displayAlt = alt > 0 ? alt : 5
    const position = Cartesian3.fromDegrees(lng, lat, displayAlt)

    return v.entities.add({
      name: `${traj.name}-takeoff`,
      position: position,
      point: new PointGraphics({
        pixelSize: 8,
        color: Color.GREEN,
        outlineColor: Color.GREEN.withAlpha(0.3),
        outlineWidth: 3,
      }),
      label: new LabelGraphics({
        text: '起飞点',
        font: '12px sans-serif',
        fillColor: Color.LIME,
        outlineColor: Color.BLACK,
        outlineWidth: 2,
        style: 1,
        pixelOffset: new Cartesian3(0, -16),
        horizontalOrigin: HorizontalOrigin.CENTER,
        verticalOrigin: VerticalOrigin.BOTTOM,
      }),
    })
  }

  // 更新无人机位置
  function updateDronePosition(
    droneId: string,
    lng: number,
    lat: number,
    alt: number,
    heading: number
  ) {
    const entry = trajectories.value.get(droneId)
    if (!entry) return

    const newPosition = Cartesian3.fromDegrees(lng, lat, alt)

    // 更新 drone 位置
    ;(entry.droneEntity.position as PositionProperty | undefined)?.setValue
      ? (entry.droneEntity.position as PositionProperty).setValue(newPosition)
      : (entry.droneEntity.position = newPosition)

    // 旋转 billboard 箭头指向飞行方向（补偿摄像机朝向）
    if (heading >= 0) {
      const b = entry.droneEntity.billboard
      if (b) {
        const camHeading = CesiumMath.toDegrees(viewer.value!.camera.heading)
        b.rotation = CesiumMath.toRadians(camHeading - heading)
      }
    }

    // 去重：避免相同位置重复添加；版本号递增触发 CallbackProperty 更新
    const lastPos = entry.pathPositions[entry.pathPositions.length - 1]
    if (!lastPos || !Cartesian3.equals(lastPos, newPosition)) {
      entry.pathPositions.push(newPosition)
      entry._pathVersion.v++
    }
  }

  // 添加新航线
  function addTrajectory(traj: TrajectoryData) {
    const previewEntity = drawPreview(traj)
    const droneEntity = createDroneEntity(traj)
    // 初始位置：起飞点或首航点
    const [takeoffLat, takeoffLng, takeoffAlt] = traj.takeOffPoint
    let startLng: number, startLat: number, startHeight: number
    if (takeoffLat !== 0 || takeoffLng !== 0) {
      startLat = takeoffLat
      startLng = takeoffLng
      startHeight = takeoffAlt > 0 ? takeoffAlt : (traj.waypoints[0]?.ellipsoidHeight || 0)
    } else {
      const firstWp = traj.waypoints[0]
      startLng = firstWp.lng
      startLat = firstWp.lat
      startHeight = firstWp.ellipsoidHeight
    }
    const initialPos = Cartesian3.fromDegrees(startLng, startLat, startHeight)
    const pathPositions: Cartesian3[] = [initialPos]
    const pathVersion = { v: 0 }
    const pathEntity = createPathEntity(traj, pathPositions, pathVersion)
    const takeOffEntity = createTakeOffEntity(traj)
    const waypointEntities = createWaypointMarkers(traj)

    trajectories.value.set(traj.id, {
      data: traj,
      droneEntity,
      pathEntity,
      previewEntity,
      takeOffEntity,
      waypointEntities,
      pathPositions,
      _pathVersion: pathVersion,
    })
  }

  // 开始仿真时清除预览线
  function clearPreview(droneId: string) {
    const entry = trajectories.value.get(droneId)
    if (!entry || !entry.previewEntity) return

    const v = getViewer()
    v.entities.remove(entry.previewEntity)
    entry.previewEntity = null
  }

  // 移除航线所有实体
  function removeTrajectory(droneId: string) {
    const entry = trajectories.value.get(droneId)
    if (!entry) return

    const v = getViewer()
    if (entry.previewEntity) {
      v.entities.remove(entry.previewEntity)
    }
    for (const wpEntity of entry.waypointEntities) {
      v.entities.remove(wpEntity)
    }
    v.entities.remove(entry.droneEntity)
    v.entities.remove(entry.pathEntity)
    v.entities.remove(entry.takeOffEntity)
    trajectories.value.delete(droneId)
  }

  // 聚焦相机到航线区域，倾斜角 60 度
  function flyToTrajectory(droneId: string) {
    const entry = trajectories.value.get(droneId)
    if (!entry) return

    const v = getViewer()
    const wps = entry.data.waypoints
    if (wps.length === 0) return

    // 计算包围盒
    let minLat = 90, maxLat = -90, minLng = 180, maxLng = -180
    for (const wp of wps) {
      if (wp.lat < minLat) minLat = wp.lat
      if (wp.lat > maxLat) maxLat = wp.lat
      if (wp.lng < minLng) minLng = wp.lng
      if (wp.lng > maxLng) maxLng = wp.lng
    }

    const centerLat = (minLat + maxLat) / 2
    const centerLng = (minLng + maxLng) / 2

    // 估算对角线距离（米）
    const dLat = (maxLat - minLat) * 111320
    const dLng = (maxLng - minLng) * 111320 * Math.cos(centerLat * Math.PI / 180)
    const diagonal = Math.sqrt(dLat * dLat + dLng * dLng)

    // 高度 = 对角线 * 0.8，最小 300m
    const altitude = Math.max(diagonal * 0.8, 300)

    v.camera.flyTo({
      destination: Cartesian3.fromDegrees(centerLng, centerLat, altitude),
      orientation: {
        heading: CesiumMath.toRadians(0),
        pitch: CesiumMath.toRadians(-60),
        roll: 0,
      },
      duration: 1.5,
    })
  }

  // 重置航线（重新开始仿真时清除旧轨迹）
  function resetPath(droneId: string) {
    const entry = trajectories.value.get(droneId)
    if (!entry) return

    // 原地清空数组
    entry.pathPositions.length = 0
    const [takeoffLat, takeoffLng, takeoffAlt] = entry.data.takeOffPoint
    let startLng: number, startLat: number, startHeight: number
    if (takeoffLat !== 0 || takeoffLng !== 0) {
      startLat = takeoffLat
      startLng = takeoffLng
      startHeight = takeoffAlt > 0 ? takeoffAlt : (entry.data.waypoints[0]?.ellipsoidHeight || 0)
    } else {
      const firstWp = entry.data.waypoints[0]
      startLng = firstWp.lng
      startLat = firstWp.lat
      startHeight = firstWp.ellipsoidHeight
    }
    const initialPos = Cartesian3.fromDegrees(startLng, startLat, startHeight)

    entry.pathPositions.push(initialPos)
    // 重置 drone 位置到起点
    entry.droneEntity.position = initialPos
    // 触发 CallbackProperty 刷新
    entry._pathVersion.v++
  }

  function getTakeOffPoint(droneId: string): [number, number, number] | null {
    const entry = trajectories.value.get(droneId)
    if (!entry) return null
    const [lat, lng, alt] = entry.data.takeOffPoint
    if (lat !== 0 || lng !== 0) {
      const startAlt = alt > 0 ? alt : (entry.data.waypoints[0]?.ellipsoidHeight || 0)
      return [lat, lng, startAlt]
    }
    const firstWp = entry.data.waypoints[0]
    if (firstWp) {
      return [firstWp.lat, firstWp.lng, firstWp.ellipsoidHeight]
    }
    return null
  }

  return {
    trajectories,
    drawPreview,
    createDroneEntity,
    createPathEntity,
    updateDronePosition,
    addTrajectory,
    clearPreview,
    removeTrajectory,
    flyToTrajectory,
    resetPath,
    getTakeOffPoint,
  }
}
