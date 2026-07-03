<template>
  <canvas
    ref="canvasRef"
    class="morph-canvas"
    aria-hidden="true"
  />
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'

type MorphState = {
  progress: number
  x: number
  y: number
  active: boolean
  energy: number
}

const props = defineProps<{
  organicSrc: string
  mechanicalSrc: string
}>()

const emit = defineEmits<{
  ready: []
}>()

const canvasRef = ref<HTMLCanvasElement | null>(null)
const targetState: MorphState = { progress: 0.5, x: 0.5, y: 0.64, active: false, energy: 0 }
const renderedState: MorphState = { ...targetState }

let gl: WebGLRenderingContext | null = null
let program: WebGLProgram | null = null
let animationFrame = 0
let resizeObserver: ResizeObserver | null = null
let startedAt = 0
let reducedMotion = false

const vertexShaderSource = `
  attribute vec2 aPosition;
  varying vec2 vUv;

  void main() {
    vUv = aPosition * 0.5 + 0.5;
    gl_Position = vec4(aPosition, 0.0, 1.0);
  }
`

const fragmentShaderSource = `
  precision highp float;

  varying vec2 vUv;
  uniform sampler2D uOrganic;
  uniform sampler2D uMechanical;
  uniform vec2 uResolution;
  uniform vec2 uImageResolution;
  uniform vec2 uCursor;
  uniform float uProgress;
  uniform float uActive;
  uniform float uEnergy;
  uniform float uTime;

  float hash(vec2 p) {
    p = fract(p * vec2(123.34, 456.21));
    p += dot(p, p + 45.32);
    return fract(p.x * p.y);
  }

  float noise(vec2 p) {
    vec2 i = floor(p);
    vec2 f = fract(p);
    f = f * f * (3.0 - 2.0 * f);
    return mix(
      mix(hash(i), hash(i + vec2(1.0, 0.0)), f.x),
      mix(hash(i + vec2(0.0, 1.0)), hash(i + vec2(1.0, 1.0)), f.x),
      f.y
    );
  }

  float fbm(vec2 p) {
    float value = 0.0;
    float amplitude = 0.5;
    mat2 rotation = mat2(0.8, 0.6, -0.6, 0.8);
    for (int i = 0; i < 5; i++) {
      value += amplitude * noise(p);
      p = rotation * p * 2.03 + 17.17;
      amplitude *= 0.5;
    }
    return value;
  }

  vec2 coverUv(vec2 uv) {
    float viewAspect = uResolution.x / max(uResolution.y, 1.0);
    float imageAspect = uImageResolution.x / max(uImageResolution.y, 1.0);

    if (viewAspect < imageAspect) {
      uv.x = (uv.x - 0.5) * (viewAspect / imageAspect) + 0.5;
    } else {
      uv.y = (uv.y - 0.5) * (imageAspect / viewAspect) + 0.5;
    }
    return clamp(uv, 0.001, 0.999);
  }

  void main() {
    vec2 uv = vUv;
    vec2 cursorDelta = uv - uCursor;
    cursorDelta.x *= uResolution.x / max(uResolution.y, 1.0);
    float cursorDistance = length(cursorDelta);
    float proximity = exp(-cursorDistance * cursorDistance * 18.0) * uActive;

    float slowNoise = fbm(vec2(uv.x * 4.2, uv.y * 6.4) + vec2(uTime * 0.045, -uTime * 0.025));
    float fineNoise = fbm(vec2(uv.x * 18.0, uv.y * 24.0) + vec2(-uTime * 0.08, uTime * 0.055));
    float fibreNoise = fbm(vec2(uv.x * 58.0, uv.y * 72.0) + uTime * 0.035);

    float frontier = mix(-0.14, 1.14, uProgress);
    float surfaceNoise = (slowNoise - 0.5) * 0.16 + (fineNoise - 0.5) * 0.045;
    float breathing = sin(uTime * 2.1 + slowNoise * 8.0) * 0.016 * uActive;
    float growthBulge = proximity * (0.13 + uEnergy * 0.055 + breathing);
    float signedGrowth = frontier - uv.x + surfaceNoise + growthBulge;
    float organicAmount = smoothstep(-0.055, 0.065, signedGrowth);

    vec2 pageUv = vec2(uv.x, 1.0 - uv.y);
    float subjectTop = 0.15 - pageUv.x * 0.15 - pageUv.x * pageUv.x * 0.25;
    float subjectBottom = 1.15 - pageUv.x * 0.08 - pageUv.x * pageUv.x * 0.15;
    float subjectMask = smoothstep(subjectTop - 0.025, subjectTop + 0.025, pageUv.y)
      * (1.0 - smoothstep(subjectBottom - 0.025, subjectBottom + 0.04, pageUv.y));

    float lens = smoothstep(0.32, 0.0, cursorDistance) * uActive;
    float lensScale = 1.0 - lens * (0.035 + uEnergy * 0.012);
    vec2 warpedUv = uCursor + (uv - uCursor) * lensScale;
    warpedUv += vec2(
      (fineNoise - 0.5) * 0.006,
      (slowNoise - 0.5) * 0.009
    ) * lens;

    vec4 mechanical = texture2D(uMechanical, coverUv(uv));
    vec4 organic = texture2D(uOrganic, coverUv(warpedUv));
    vec3 color = mix(mechanical.rgb, organic.rgb, organicAmount * subjectMask);

    float frontierBand = exp(-abs(signedGrowth) * 23.0) * uActive * subjectMask;
    float softMoss = frontierBand * (0.08 + fineNoise * 0.13);
    float looseFibres = smoothstep(0.7, 0.94, fibreNoise) * frontierBand * (0.12 + uEnergy * 0.18);
    vec3 mossDark = vec3(0.22, 0.29, 0.10);
    vec3 mossLight = vec3(0.45, 0.53, 0.20);
    color = mix(color, mossDark, softMoss);
    color = mix(color, mossLight, looseFibres);

    float highlight = pow(max(0.0, 1.0 - cursorDistance * 3.4), 3.0)
      * uActive * subjectMask * 0.06;
    color += vec3(0.13, 0.16, 0.07) * highlight;

    gl_FragColor = vec4(color, 1.0);
  }
`

function setMorphState(state: MorphState) {
  targetState.progress = Math.max(0, Math.min(1, state.progress))
  targetState.x = Math.max(0, Math.min(1, state.x))
  targetState.y = Math.max(0, Math.min(1, state.y))
  targetState.active = state.active
  targetState.energy = Math.max(0, Math.min(1, state.energy))
}

function compileShader(context: WebGLRenderingContext, type: number, source: string) {
  const shader = context.createShader(type)
  if (!shader) throw new Error('Unable to create WebGL shader')
  context.shaderSource(shader, source)
  context.compileShader(shader)
  if (!context.getShaderParameter(shader, context.COMPILE_STATUS)) {
    const message = context.getShaderInfoLog(shader) || 'Unable to compile WebGL shader'
    context.deleteShader(shader)
    throw new Error(message)
  }
  return shader
}

function createProgram(context: WebGLRenderingContext) {
  const vertexShader = compileShader(context, context.VERTEX_SHADER, vertexShaderSource)
  const fragmentShader = compileShader(context, context.FRAGMENT_SHADER, fragmentShaderSource)
  const nextProgram = context.createProgram()
  if (!nextProgram) throw new Error('Unable to create WebGL program')
  context.attachShader(nextProgram, vertexShader)
  context.attachShader(nextProgram, fragmentShader)
  context.linkProgram(nextProgram)
  context.deleteShader(vertexShader)
  context.deleteShader(fragmentShader)
  if (!context.getProgramParameter(nextProgram, context.LINK_STATUS)) {
    throw new Error(context.getProgramInfoLog(nextProgram) || 'Unable to link WebGL program')
  }
  return nextProgram
}

function loadImage(src: string) {
  return new Promise<HTMLImageElement>((resolve, reject) => {
    const image = new Image()
    image.decoding = 'async'
    image.onload = () => resolve(image)
    image.onerror = () => reject(new Error(`Unable to load morph texture: ${src}`))
    image.src = src
  })
}

function createTexture(context: WebGLRenderingContext, image: HTMLImageElement, textureUnit: number) {
  const texture = context.createTexture()
  if (!texture) throw new Error('Unable to create WebGL texture')
  context.activeTexture(textureUnit)
  context.bindTexture(context.TEXTURE_2D, texture)
  context.pixelStorei(context.UNPACK_FLIP_Y_WEBGL, 1)
  context.texParameteri(context.TEXTURE_2D, context.TEXTURE_WRAP_S, context.CLAMP_TO_EDGE)
  context.texParameteri(context.TEXTURE_2D, context.TEXTURE_WRAP_T, context.CLAMP_TO_EDGE)
  context.texParameteri(context.TEXTURE_2D, context.TEXTURE_MIN_FILTER, context.LINEAR)
  context.texParameteri(context.TEXTURE_2D, context.TEXTURE_MAG_FILTER, context.LINEAR)
  context.texImage2D(context.TEXTURE_2D, 0, context.RGBA, context.RGBA, context.UNSIGNED_BYTE, image)
  return texture
}

function resizeCanvas() {
  const canvas = canvasRef.value
  if (!canvas || !gl) return
  const bounds = canvas.getBoundingClientRect()
  const pixelRatio = Math.min(window.devicePixelRatio || 1, 1.6)
  const width = Math.max(1, Math.round(bounds.width * pixelRatio))
  const height = Math.max(1, Math.round(bounds.height * pixelRatio))
  if (canvas.width !== width || canvas.height !== height) {
    canvas.width = width
    canvas.height = height
  }
  gl.viewport(0, 0, width, height)
}

function uniform(name: string) {
  if (!gl || !program) return null
  return gl.getUniformLocation(program, name)
}

function render(now: number) {
  if (!gl || !program || !canvasRef.value) return

  const smoothing = renderedState.active ? 0.14 : 0.1
  renderedState.progress += (targetState.progress - renderedState.progress) * smoothing
  renderedState.x += (targetState.x - renderedState.x) * 0.16
  renderedState.y += (targetState.y - renderedState.y) * 0.16
  renderedState.energy += (targetState.energy - renderedState.energy) * 0.18
  renderedState.active = targetState.active

  gl.useProgram(program)
  gl.uniform2f(uniform('uResolution'), canvasRef.value.width, canvasRef.value.height)
  gl.uniform2f(uniform('uCursor'), renderedState.x, 1 - renderedState.y)
  gl.uniform1f(uniform('uProgress'), renderedState.progress)
  gl.uniform1f(uniform('uActive'), renderedState.active ? 1 : 0)
  gl.uniform1f(uniform('uEnergy'), renderedState.energy)
  gl.uniform1f(uniform('uTime'), reducedMotion ? 0 : (now - startedAt) / 1000)
  gl.drawArrays(gl.TRIANGLE_STRIP, 0, 4)
  animationFrame = window.requestAnimationFrame(render)
}

async function initialise() {
  const canvas = canvasRef.value
  if (!canvas) return
  const context = canvas.getContext('webgl', {
    alpha: false,
    antialias: true,
    depth: false,
    powerPreference: 'high-performance'
  })
  if (!context) return

  gl = context
  program = createProgram(context)
  const [organicImage, mechanicalImage] = await Promise.all([
    loadImage(props.organicSrc),
    loadImage(props.mechanicalSrc)
  ])

  context.useProgram(program)
  const positionBuffer = context.createBuffer()
  context.bindBuffer(context.ARRAY_BUFFER, positionBuffer)
  context.bufferData(
    context.ARRAY_BUFFER,
    new Float32Array([-1, -1, 1, -1, -1, 1, 1, 1]),
    context.STATIC_DRAW
  )
  const positionLocation = context.getAttribLocation(program, 'aPosition')
  context.enableVertexAttribArray(positionLocation)
  context.vertexAttribPointer(positionLocation, 2, context.FLOAT, false, 0, 0)

  createTexture(context, organicImage, context.TEXTURE0)
  createTexture(context, mechanicalImage, context.TEXTURE1)
  context.uniform1i(uniform('uOrganic'), 0)
  context.uniform1i(uniform('uMechanical'), 1)
  context.uniform2f(uniform('uImageResolution'), organicImage.naturalWidth, organicImage.naturalHeight)

  resizeObserver = new ResizeObserver(resizeCanvas)
  resizeObserver.observe(canvas)
  resizeCanvas()
  reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches
  startedAt = performance.now()
  animationFrame = window.requestAnimationFrame(render)
  emit('ready')
}

onMounted(() => {
  initialise().catch(() => {
    gl = null
    program = null
  })
})

onBeforeUnmount(() => {
  if (animationFrame) window.cancelAnimationFrame(animationFrame)
  resizeObserver?.disconnect()
})

defineExpose({ setMorphState })
</script>

<style scoped>
.morph-canvas {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  display: block;
  pointer-events: none;
}
</style>
