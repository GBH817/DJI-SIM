import fs from 'fs'
import path from 'path'

const inputFile = path.resolve('data/shenzhen_buildings.json')
const outputFile = path.resolve('data/shenzhen_buildings.geojson')

console.log('Reading OSM data...')
const raw = JSON.parse(fs.readFileSync(inputFile, 'utf-8'))

// Build node ID → [lon, lat] map
const nodeMap = new Map()
for (const el of raw.elements) {
  if (el.type === 'node') {
    nodeMap.set(el.id, [el.lon, el.lat])
  }
}

console.log(`Nodes: ${nodeMap.size}`)

// Convert ways to GeoJSON features
const features = []
let skipped = 0

for (const el of raw.elements) {
  if (el.type !== 'way') continue
  if (!el.tags || !el.tags.building) continue
  if (!el.nodes || el.nodes.length < 3) continue

  // Resolve coordinates
  const coords = []
  for (const nodeId of el.nodes) {
    const pos = nodeMap.get(nodeId)
    if (pos) {
      coords.push(pos)
    }
  }

  if (coords.length < 3) {
    skipped++
    continue
  }

  // Close ring if not closed
  const first = coords[0]
  const last = coords[coords.length - 1]
  if (first[0] !== last[0] || first[1] !== last[1]) {
    coords.push([...first])
  }

  // Extract height
  let height = null
  if (el.tags.height) {
    height = parseFloat(el.tags.height)
  }
  if (!height && el.tags['building:levels']) {
    height = parseFloat(el.tags['building:levels']) * 3
  }

  features.push({
    type: 'Feature',
    properties: {
      height: height || null,
      levels: el.tags['building:levels'] ? parseFloat(el.tags['building:levels']) : null,
      name: el.tags.name || null,
      building: el.tags.building,
    },
    geometry: {
      type: 'Polygon',
      coordinates: [coords],
    },
  })
}

const withHeight = features.filter(f => f.properties.height !== null).length
const withoutHeight = features.filter(f => f.properties.height === null).length

console.log(`Buildings: ${features.length} (with height: ${withHeight}, without height: ${withoutHeight}, skipped: ${skipped})`)

const geojson = {
  type: 'FeatureCollection',
  features,
}

fs.writeFileSync(outputFile, JSON.stringify(geojson))
const sizeMB = (fs.statSync(outputFile).size / (1024 * 1024)).toFixed(2)
console.log(`Written: ${outputFile} (${sizeMB} MB)`)
