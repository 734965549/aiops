<template>
  <canvas
    ref="canvasRef"
    class="sardine-school"
    aria-hidden="true"
  />
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import sardineSpriteUrl from '@/assets/sardine-realistic.png'

interface Sardine {
  angle: number
  orbit: number
  verticalOffset: number
  size: number
  speed: number
  direction: 1 | -1
  alpha: number
  phase: number
  bank: number
  tone: 0 | 1 | 2
}

const props = withDefaults(defineProps<{
  count?: number
  seed?: number
  centerX?: number
  centerY?: number
  radiusX?: number
  radiusY?: number
  fishScale?: number
  sidebarOffset?: boolean
}>(), {
  count: 160,
  seed: 20260702,
  centerX: 0.5,
  centerY: 0.5,
  radiusX: 0.36,
  radiusY: 0.33,
  fishScale: 1,
  sidebarOffset: false
})

const canvasRef = ref<HTMLCanvasElement | null>(null)
const sardines: Sardine[] = []
let spriteImage: HTMLImageElement | null = null
let shadowSprite: HTMLCanvasElement | null = null
let midtoneSprite: HTMLCanvasElement | null = null
let resizeObserver: ResizeObserver | null = null
let intersectionObserver: IntersectionObserver | null = null
let animationFrame = 0
let previousTime = 0
let motionTime = 0
let viewportWidth = 0
let viewportHeight = 0
let reducedMotion = false
let isVisible = true

function seededRandom(seed: number) {
  let value = seed >>> 0
  return () => {
    value += 0x6D2B79F5
    let result = value
    result = Math.imul(result ^ (result >>> 15), result | 1)
    result ^= result + Math.imul(result ^ (result >>> 7), result | 61)
    return ((result ^ (result >>> 14)) >>> 0) / 4294967296
  }
}

function createSchool() {
  const random = seededRandom(props.seed)
  const count = viewportWidth < 700 ? Math.round(props.count * 0.62) : props.count
  const laneCount = 5
  const lanePopulation = Math.ceil(count / laneCount)
  sardines.length = 0

  for (let index = 0; index < count; index += 1) {
    const lane = index % laneCount
    const laneIndex = Math.floor(index / laneCount)
    const toneRoll = random()
    sardines.push({
      angle: (laneIndex / lanePopulation) * Math.PI * 2 + lane * 0.16 + (random() - 0.5) * 0.22,
      orbit: 0.68 + lane * 0.105 + (random() - 0.5) * 0.05,
      verticalOffset: (random() - 0.5) * Math.min(18, viewportHeight * 0.035),
      size: (0.42 + random() * 0.9) * props.fishScale,
      speed: 0.000085 + random() * 0.00004,
      direction: 1,
      alpha: 0.46 + random() * 0.42,
      phase: random() * Math.PI * 2,
      bank: 0.5 + random() * 0.5,
      tone: toneRoll < 0.68 ? 0 : toneRoll < 0.9 ? 1 : 2
    })
  }
}

function createTintedSprite(image: HTMLImageElement, color: string) {
  const sprite = document.createElement('canvas')
  sprite.width = image.naturalWidth
  sprite.height = image.naturalHeight
  const context = sprite.getContext('2d')
  if (!context) return sprite
  context.drawImage(image, 0, 0)
  context.globalCompositeOperation = 'source-atop'
  context.fillStyle = color
  context.fillRect(0, 0, sprite.width, sprite.height)
  return sprite
}

function loadSprite() {
  const image = new Image()
  image.decoding = 'async'
  image.onload = () => {
    spriteImage = image
    shadowSprite = createTintedSprite(image, 'rgba(2, 29, 43, 0.82)')
    midtoneSprite = createTintedSprite(image, 'rgba(45, 83, 94, 0.56)')
    drawScene(0)
  }
  image.src = sardineSpriteUrl
}

function resizeCanvas() {
  const canvas = canvasRef.value
  if (!canvas) return

  const rect = canvas.getBoundingClientRect()
  viewportWidth = Math.max(1, Math.round(rect.width))
  viewportHeight = Math.max(1, Math.round(rect.height))
  const pixelRatio = Math.min(window.devicePixelRatio || 1, 1.75)
  canvas.width = Math.round(viewportWidth * pixelRatio)
  canvas.height = Math.round(viewportHeight * pixelRatio)
  const context = canvas.getContext('2d')
  context?.setTransform(pixelRatio, 0, 0, pixelRatio, 0, 0)
  createSchool()
  drawScene(0)
}

function drawSardine(
  context: CanvasRenderingContext2D,
  fish: Sardine,
  x: number,
  y: number,
  heading: number,
  scale: number,
  alpha: number
) {
  const length = 34 * scale
  const height = length * 0.184 * fish.bank
  context.save()
  context.translate(x, y)
  context.rotate(heading)
  context.globalAlpha = alpha
  const sprite = fish.tone === 0 ? shadowSprite : fish.tone === 1 ? midtoneSprite : spriteImage
  if (sprite) {
    context.drawImage(sprite, -length / 2, -height / 2, length, height)
  } else {
    context.beginPath()
    context.moveTo(length * 0.5, 0)
    context.quadraticCurveTo(0, -height * 0.75, -length * 0.38, 0)
    context.quadraticCurveTo(0, height * 0.75, length * 0.5, 0)
    context.moveTo(-length * 0.36, 0)
    context.lineTo(-length * 0.54, -height)
    context.lineTo(-length * 0.5, 0)
    context.lineTo(-length * 0.54, height)
    context.closePath()
    context.fillStyle = '#092f42'
    context.fill()
  }
  context.restore()
}

function drawScene(elapsed: number) {
  const canvas = canvasRef.value
  const context = canvas?.getContext('2d')
  if (!canvas || !context) return

  context.clearRect(0, 0, viewportWidth, viewportHeight)
  motionTime += elapsed
  const sidebarWidth = props.sidebarOffset && viewportWidth > 900 ? 228 : 0
  const contentWidth = viewportWidth - sidebarWidth
  const centerX = sidebarWidth + contentWidth * props.centerX
  const centerY = viewportHeight * props.centerY
  const radiusX = Math.max(180, contentWidth * props.radiusX)
  const radiusY = Math.max(110, viewportHeight * props.radiusY)

  sardines.forEach((fish) => {
    if (!reducedMotion) fish.angle += fish.speed * elapsed * fish.direction
    const wave = Math.sin(motionTime * 0.0007 + fish.phase) * 4
    const x = centerX + Math.cos(fish.angle) * (radiusX * fish.orbit + wave)
    const y = centerY + Math.sin(fish.angle) * (radiusY * fish.orbit + wave) + fish.verticalOffset
    const tangentX = -Math.sin(fish.angle) * radiusX * fish.direction
    const tangentY = Math.cos(fish.angle) * radiusY * fish.direction
    const wobble = Math.sin(motionTime * 0.003 + fish.phase) * 0.045
    const heading = Math.atan2(tangentY, tangentX) + wobble
    const perspective = 0.64 + ((Math.sin(fish.angle) + 1) / 2) * 0.58
    drawSardine(context, fish, x, y, heading, fish.size * perspective, fish.alpha)
  })
}

function animate(time: number) {
  const elapsed = previousTime ? Math.min(48, time - previousTime) : 16
  previousTime = time
  drawScene(elapsed)
  animationFrame = window.requestAnimationFrame(animate)
}

function updateAnimationState() {
  const shouldAnimate = !reducedMotion && isVisible && !document.hidden
  if (!shouldAnimate) {
    window.cancelAnimationFrame(animationFrame)
    animationFrame = 0
    return
  }
  if (!animationFrame) {
    previousTime = 0
    animationFrame = window.requestAnimationFrame(animate)
  }
}

onMounted(() => {
  const canvas = canvasRef.value
  if (!canvas) return
  reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches
  resizeObserver = new ResizeObserver(resizeCanvas)
  resizeObserver.observe(canvas)
  intersectionObserver = new IntersectionObserver(([entry]) => {
    isVisible = entry?.isIntersecting ?? true
    updateAnimationState()
  })
  intersectionObserver.observe(canvas)
  document.addEventListener('visibilitychange', updateAnimationState)
  loadSprite()
  resizeCanvas()
  updateAnimationState()
})

onBeforeUnmount(() => {
  resizeObserver?.disconnect()
  intersectionObserver?.disconnect()
  document.removeEventListener('visibilitychange', updateAnimationState)
  window.cancelAnimationFrame(animationFrame)
})
</script>

<style scoped>
.sardine-school {
  display: block;
  width: 100%;
  height: 100%;
  pointer-events: none;
}
</style>
