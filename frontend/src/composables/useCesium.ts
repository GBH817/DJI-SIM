import {
  Ion,
  Viewer,
  Cartesian3,
  Math as CesiumMath,
  UrlTemplateImageryProvider,
  WebMercatorTilingScheme,
} from 'cesium'
import { ref, type Ref } from 'vue'

// Cesium Ion Token
Ion.defaultAccessToken = 'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJqdGkiOiI5YmUzMTEzOC01MzlmLTRhN2MtOTAzMy1lOGEyYmU5MmFlMzIiLCJpZCI6NjM5NDEsImlhdCI6MTYzMzQ5NTIxN30.9u5kgq1kN6Z1GhuSQ4QbBUSdR9sY2XVbfEiZak-HN3Y'

// 天地图 Token
const TIANDITU_TOKEN = '643e236518d4875f1d2d9232af78f835'

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
        destination: Cartesian3.fromDegrees(113.935092, 22.531564, 5000),
        orientation: {
          heading: CesiumMath.toRadians(0),
          pitch: CesiumMath.toRadians(-45),
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

  return { viewer, init, destroy }
}
