import {
  Ion,
  Viewer,
  Cartesian3,
  Math as CesiumMath,
  UrlTemplateImageryProvider,
  WebMercatorTilingScheme,
  GeoJsonDataSource,
  Color,
  Rectangle,
  type DataSource,
} from 'cesium'
import { ref, watch, type Ref } from 'vue'

// Cesium Ion Token
Ion.defaultAccessToken = 'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJqdGkiOiI5YmUzMTEzOC01MzlmLTRhN2MtOTAzMy1lOGEyYmU5MmFlMzIiLCJpZCI6NjM5NDEsImlhdCI6MTYzMzQ5NTIxN30.9u5kgq1kN6Z1GhuSQ4QbBUSdR9sY2XVbfEiZak-HN3Y'

// 天地图 Token
const TIANDITU_TOKEN = '643e236518d4875f1d2d9232af78f835'

/** 瓦片信息 */
interface TileInfo {
  file: string
  west: number
  east: number
  south: number
  north: number
  count: number
  sizeMB: string
}

/** 给 dataSource 中的建筑实体设置挤出样式（分帧处理，避免卡顿） */
function styleDataSource(dataSource: DataSource) {
  const entities = dataSource.entities.values
  if (entities.length === 0) return

  const BATCH_SIZE = 500

  function applyBatch(startIndex: number) {
    const end = Math.min(startIndex + BATCH_SIZE, entities.length)
    for (let i = startIndex; i < end; i++) {
      const entity = entities[i]
      if (!entity.polygon) continue
      const props = entity.properties
      const h = props?.height?.getValue()
      const height = h && h > 0 ? h : 10

      entity.polygon.extrudedHeight = height
      entity.polygon.material = Color.fromCssColorString('#aaccdd').withAlpha(0.5)
      // 不绘制轮廓线，大幅降低 GPU 负载
      entity.polygon.outline = false
    }

    if (end < entities.length) {
      requestAnimationFrame(() => applyBatch(end))
    }
  }

  applyBatch(0)
}

export function useCesium(containerId: string) {
  const viewer: Ref<Viewer | null> = ref(null)

  function init() {
    if (viewer.value) {
      return
    }

    viewer.value = new Viewer(containerId, {
      animation: false,
      timeline: false,
      baseLayerPicker: false,
      fullscreenButton: false,
      geocoder: false,
      homeButton: false,
      infoBox: false,
      sceneModePicker: false,
      selectionIndicator: false,
      navigationHelpButton: false,
      // 禁止 Cesium 创建默认 Ion 图层
      baseLayer: false,
    })

    const tilingScheme = new WebMercatorTilingScheme()

    // 天地图卫星影像（Web Mercator 版本）
    viewer.value.imageryLayers.addImageryProvider(
      new UrlTemplateImageryProvider({
        url: `https://t{s}.tianditu.gov.cn/DataServer?T=img_w&X={x}&Y={y}&L={z}&tk=${TIANDITU_TOKEN}`,
        subdomains: ['0', '1', '2', '3', '4', '5', '6', '7'],
        maximumLevel: 18,
        tilingScheme,
      }),
    )

    // 天地图卫星标注（Web Mercator 版本）
    viewer.value.imageryLayers.addImageryProvider(
      new UrlTemplateImageryProvider({
        url: `https://t{s}.tianditu.gov.cn/DataServer?T=cia_w&X={x}&Y={y}&L={z}&tk=${TIANDITU_TOKEN}`,
        subdomains: ['0', '1', '2', '3', '4', '5', '6', '7'],
        maximumLevel: 18,
        tilingScheme,
      }),
    )

    // 在场景首次渲染后设置相机位置
    const removeHandler = viewer.value.scene.postRender.addEventListener(() => {
      removeHandler()
      viewer.value!.camera.setView({
        destination: Cartesian3.fromDegrees(113.935085694, 22.527709233, 987),
        orientation: {
          heading: CesiumMath.toRadians(2.355),
          pitch: CesiumMath.toRadians(-70.66),
          roll: 0,
        },
      })
    })
  }

  function destroy() {
    if (viewer.value) {
      viewer.value.destroy()
      viewer.value = null
    }
  }

  // ──── 建筑瓦片分区加载 ────

  const buildingsVisible = ref(false)

  let tileIndex: TileInfo[] = []
  const loadedTiles = new Set<string>()
  let updateTimer: ReturnType<typeof setTimeout> | null = null
  /** 正在加载的瓦片数（防止并发过多） */
  let loadingCount = 0
  const MAX_CONCURRENT = 1
  const THROTTLE_MS = 300

  // 相机高度阈值（米）：低于此值才加载建筑，高于此值卸载全部
  const HEIGHT_LOAD = 4000
  const HEIGHT_UNLOAD = 5000

  /** 获取当前相机高度（米） */
  function getCameraHeight(): number {
    if (!viewer.value) return Infinity
    const cartographic = viewer.value.camera.positionCartographic
    return cartographic.height
  }

  /** 加载瓦片索引 */
  async function loadTileIndex() {
    if (tileIndex.length > 0) return
    try {
      const resp = await fetch('/tiles/tiles.json')
      tileIndex = await resp.json()
      console.log(`瓦片索引加载完成: ${tileIndex.length} 个瓦片`)
    } catch (err) {
      console.error('瓦片索引加载失败:', err)
    }
  }

  /** 判断矩形是否与当前视野有交集 */
  function isTileVisible(tile: TileInfo): boolean {
    if (!viewer.value) return false
    const rect = viewer.value.camera.computeViewRectangle()
    if (!rect) return false

    return !(
      tile.east  < CesiumMath.toDegrees(rect.west) ||
      tile.west  > CesiumMath.toDegrees(rect.east) ||
      tile.north < CesiumMath.toDegrees(rect.south) ||
      tile.south > CesiumMath.toDegrees(rect.north)
    )
  }

  /** 加载单个瓦片 */
  async function loadTile(tile: TileInfo) {
    if (loadedTiles.has(tile.file)) return

    loadingCount++
    try {
      const dataSource = await GeoJsonDataSource.load(
        `/tiles/${tile.file}`,
        {
          name: tile.file,
          clampToGround: false,
        },
      )
      styleDataSource(dataSource)
      viewer.value!.dataSources.add(dataSource)
      loadedTiles.add(tile.file)
    } catch (err) {
      console.error(`加载 ${tile.file} 失败:`, err)
    } finally {
      loadingCount--
    }
  }

  /** 卸载单个瓦片 */
  function unloadTile(tileName: string) {
    if (!viewer.value) return
    const sources = viewer.value.dataSources.getByName(tileName)
    for (const ds of sources) {
      viewer.value.dataSources.remove(ds, true)
    }
    loadedTiles.delete(tileName)
  }

  /** 卸载所有已加载瓦片 */
  function unloadAllTiles() {
    for (const name of loadedTiles) {
      unloadTile(name)
    }
  }

  /** 延迟加载队列 */
  let loadQueue: TileInfo[] = []
  let queueRunning = false

  async function processLoadQueue() {
    if (queueRunning) return
    queueRunning = true

    while (loadQueue.length > 0) {
      if (loadingCount >= MAX_CONCURRENT) {
        await new Promise(r => setTimeout(r, 100))
        continue
      }
      const tile = loadQueue.shift()!
      // 加载前再次确认仍在视野内且高度合适
      if (isTileVisible(tile) && getCameraHeight() < HEIGHT_UNLOAD) {
        await loadTile(tile)
        // 每个瓦片加载后短暂喘息
        await new Promise(r => setTimeout(r, 100))
      }
    }

    queueRunning = false
  }

  /** 根据当前视野更新瓦片加载/卸载 */
  function updateVisibleTiles() {
    if (!viewer.value || tileIndex.length === 0) return

    // 开关关闭：卸载所有建筑
    if (!buildingsVisible.value) {
      if (loadedTiles.size > 0) {
        unloadAllTiles()
        loadQueue = []
      }
      return
    }

    const camHeight = getCameraHeight()

    // 相机太高：卸载全部建筑
    if (camHeight > HEIGHT_UNLOAD) {
      if (loadedTiles.size > 0) {
        unloadAllTiles()
        loadQueue = []
        console.log('相机过高，卸载全部建筑')
      }
      return
    }

    // 相机低于加载阈值才加载
    if (camHeight > HEIGHT_LOAD) {
      // 在阈值区间内，保持现状不动
      return
    }

    // 收集需要卸载的
    for (const name of loadedTiles) {
      const tile = tileIndex.find(t => t.file === name)
      if (!tile || !isTileVisible(tile)) {
        unloadTile(name)
      }
    }

    // 收集需要加载的
    const toLoad = tileIndex.filter(
      t => isTileVisible(t) && !loadedTiles.has(t.file)
    )

    if (toLoad.length > 0) {
      // 按建筑数量升序排列，优先加载小瓦片
      toLoad.sort((a, b) => a.count - b.count)
      loadQueue.push(...toLoad)
      processLoadQueue()
    }
  }

  /** 启动建筑分区加载（在 init 后调用） */
  async function loadShenzhenBuildings() {
    if (!viewer.value) return
    await loadTileIndex()

    // 初次加载
    updateVisibleTiles()

    // 监听相机移动，节流更新
    viewer.value.camera.changed.addEventListener(() => {
      if (updateTimer) clearTimeout(updateTimer)
      updateTimer = setTimeout(() => {
        updateVisibleTiles()
      }, THROTTLE_MS)
    })

    // 监听开关变化，打开时立即触发加载
    watch(buildingsVisible, (val) => {
      if (val) {
        updateVisibleTiles()
      }
    })
  }

  return { viewer, init, destroy, loadShenzhenBuildings, buildingsVisible }
}
