import fs from 'fs'
import path from 'path'

const inputFile = path.resolve('data/shenzhen_buildings.geojson')
const outputDir = path.resolve('frontend/public/tiles')

// 深圳区域范围
const LON_MIN = 113.75
const LON_MAX = 114.35
const LAT_MIN = 22.45
const LAT_MAX = 22.85

// 10×10 网格
const GRID_COLS = 10
const GRID_ROWS = 10

const dLon = (LON_MAX - LON_MIN) / GRID_COLS
const dLat = (LAT_MAX - LAT_MIN) / GRID_ROWS

console.log('Reading GeoJSON...')
const geojson = JSON.parse(fs.readFileSync(inputFile, 'utf-8'))
console.log(`Total buildings: ${geojson.features.length}`)

// 初始化网格桶
const grid = Array.from({ length: GRID_ROWS }, () =>
  Array.from({ length: GRID_COLS }, () => [])
)

for (const feature of geojson.features) {
  const coords = feature.geometry.coordinates[0]
  // 取第一个点作为归属判断
  const [lon, lat] = coords[0]

  if (lon < LON_MIN || lon > LON_MAX || lat < LAT_MIN || lat > LAT_MAX) continue

  const col = Math.min(Math.floor((lon - LON_MIN) / dLon), GRID_COLS - 1)
  const row = Math.min(Math.floor((lat - LAT_MIN) / dLat), GRID_ROWS - 1)

  grid[row][col].push(feature)
}

// 确保输出目录存在
fs.mkdirSync(outputDir, { recursive: true })

// 写入每个瓦片
const tileIndex = []

for (let row = 0; row < GRID_ROWS; row++) {
  for (let col = 0; col < GRID_COLS; col++) {
    const features = grid[row][col]
    if (features.length === 0) continue

    const tileName = `tile_${row}_${col}.json`
    const tilePath = path.join(outputDir, tileName)

    const tileGeojson = {
      type: 'FeatureCollection',
      features,
    }

    fs.writeFileSync(tilePath, JSON.stringify(tileGeojson))
    const sizeMB = (fs.statSync(tilePath).size / (1024 * 1024)).toFixed(2)

    const west = LON_MIN + col * dLon
    const east = LON_MIN + (col + 1) * dLon
    const south = LAT_MIN + row * dLat
    const north = LAT_MIN + (row + 1) * dLat

    tileIndex.push({
      file: tileName,
      west,
      east,
      south,
      north,
      count: features.length,
      sizeMB,
    })

    console.log(`  ${tileName}: ${features.length} buildings, ${sizeMB} MB [${west.toFixed(4)},${east.toFixed(4)} × ${south.toFixed(4)},${north.toFixed(4)}]`)
  }
}

// 写入索引
fs.writeFileSync(path.join(outputDir, 'tiles.json'), JSON.stringify(tileIndex, null, 2))
console.log(`\nDone! ${tileIndex.length} tiles written to ${outputDir}`)
