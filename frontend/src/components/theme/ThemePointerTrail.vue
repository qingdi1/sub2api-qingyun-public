<template>
  <canvas ref="canvasRef" class="theme-pointer-trail" aria-hidden="true"></canvas>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { getUiStyle, type UiStyleId } from '@/themes/catalog'

type ParticleShape =
  | 'dot'
  | 'brush'
  | 'bubble'
  | 'ribbon'
  | 'ember'
  | 'leaf'
  | 'petal'
  | 'star'
  | 'spark'
  | 'pixel'

type TrailBlendMode = 'source-over' | 'multiply' | 'screen'

interface TrailConfig {
  shape: ParticleShape
  colors: string[]
  spawn: number
  life: number
  size: number
  speed: number
  gravity: number
  drag: number
  opacity: number
  blend: TrailBlendMode
}

interface TrailParticle {
  x: number
  y: number
  previousX: number
  previousY: number
  velocityX: number
  velocityY: number
  life: number
  maxLife: number
  size: number
  angle: number
  spin: number
  color: string
  shape: ParticleShape
  gravity: number
  drag: number
  opacity: number
  blend: TrailBlendMode
}

const TRAILS: Record<UiStyleId, TrailConfig> = {
  classic: {
    shape: 'dot', colors: ['#14b8a6', '#06b6d4', '#64748b'], spawn: 2,
    life: 24, size: 3.2, speed: 0.55, gravity: -0.006, drag: 0.94, opacity: 0.68, blend: 'source-over'
  },
  ink: {
    shape: 'brush', colors: ['#286b53', '#3e372b', '#a63d32'], spawn: 1,
    life: 42, size: 11, speed: 0.32, gravity: -0.012, drag: 0.97, opacity: 0.24, blend: 'multiply'
  },
  ocean: {
    shape: 'bubble', colors: ['#0284c7', '#0ea5e9', '#7dd3fc'], spawn: 2,
    life: 36, size: 5, speed: 0.48, gravity: -0.026, drag: 0.97, opacity: 0.62, blend: 'source-over'
  },
  aurora: {
    shape: 'ribbon', colors: ['#7c3aed', '#22d3ee', '#4ade80'], spawn: 3,
    life: 34, size: 4.5, speed: 0.72, gravity: -0.009, drag: 0.95, opacity: 0.58, blend: 'screen'
  },
  sunset: {
    shape: 'ember', colors: ['#ea580c', '#f59e0b', '#fde68a'], spawn: 3,
    life: 32, size: 3.8, speed: 0.78, gravity: -0.032, drag: 0.955, opacity: 0.74, blend: 'screen'
  },
  forest: {
    shape: 'leaf', colors: ['#15803d', '#65a30d', '#84cc16'], spawn: 1,
    life: 44, size: 6.5, speed: 0.5, gravity: 0.018, drag: 0.97, opacity: 0.62, blend: 'source-over'
  },
  rose: {
    shape: 'petal', colors: ['#db2777', '#f43f5e', '#f9a8d4'], spawn: 2,
    life: 40, size: 6, speed: 0.48, gravity: 0.014, drag: 0.972, opacity: 0.58, blend: 'source-over'
  },
  midnight: {
    shape: 'star', colors: ['#818cf8', '#c084fc', '#38bdf8'], spawn: 3,
    life: 30, size: 4.2, speed: 0.62, gravity: -0.008, drag: 0.94, opacity: 0.82, blend: 'screen'
  },
  citrus: {
    shape: 'spark', colors: ['#eab308', '#22c55e', '#fef08a'], spawn: 3,
    life: 27, size: 3.8, speed: 0.9, gravity: 0.014, drag: 0.93, opacity: 0.76, blend: 'source-over'
  },
  slate: {
    shape: 'pixel', colors: ['#334155', '#0ea5e9', '#94a3b8'], spawn: 2,
    life: 23, size: 4, speed: 0.42, gravity: 0, drag: 0.9, opacity: 0.62, blend: 'source-over'
  }
}

const canvasRef = ref<HTMLCanvasElement | null>(null)
const particles: TrailParticle[] = []
let context: CanvasRenderingContext2D | null = null
let animationFrame = 0
let currentStyle: UiStyleId = 'classic'
let lastX = -100
let lastY = -100
let lastSpawnAt = 0
let motionQuery: MediaQueryList | null = null
let pointerQuery: MediaQueryList | null = null
let themeObserver: MutationObserver | null = null

function randomBetween(min: number, max: number) {
  return min + Math.random() * (max - min)
}

function resizeCanvas() {
  const canvas = canvasRef.value
  if (!canvas || !context) return
  const ratio = Math.min(window.devicePixelRatio || 1, 2)
  canvas.width = Math.round(window.innerWidth * ratio)
  canvas.height = Math.round(window.innerHeight * ratio)
  canvas.style.width = `${window.innerWidth}px`
  canvas.style.height = `${window.innerHeight}px`
  context.setTransform(ratio, 0, 0, ratio, 0, 0)
}

function animationsAllowed() {
  return !motionQuery?.matches && Boolean(pointerQuery?.matches)
}

function syncTheme() {
  currentStyle = getUiStyle(document.documentElement.dataset.uiStyle).id
  if (canvasRef.value) canvasRef.value.dataset.trailStyle = currentStyle
}

function spawnParticles(x: number, y: number) {
  const config = TRAILS[currentStyle]
  for (let index = 0; index < config.spawn; index += 1) {
    const angle = Math.random() * Math.PI * 2
    const speed = randomBetween(config.speed * 0.35, config.speed)
    const life = Math.round(randomBetween(config.life * 0.72, config.life * 1.15))
    const offset = randomBetween(-4, 4)
    particles.push({
      x: x + Math.cos(angle) * offset,
      y: y + Math.sin(angle) * offset,
      previousX: x,
      previousY: y,
      velocityX: Math.cos(angle) * speed,
      velocityY: Math.sin(angle) * speed,
      life,
      maxLife: life,
      size: randomBetween(config.size * 0.62, config.size * 1.25),
      angle,
      spin: randomBetween(-0.08, 0.08),
      color: config.colors[Math.floor(Math.random() * config.colors.length)],
      shape: config.shape,
      gravity: config.gravity,
      drag: config.drag,
      opacity: config.opacity,
      blend: config.blend
    })
  }

  if (particles.length > 180) particles.splice(0, particles.length - 180)
  if (canvasRef.value) canvasRef.value.dataset.activeParticles = String(particles.length)
  if (!animationFrame) animationFrame = requestAnimationFrame(render)
}

function handlePointerMove(event: PointerEvent) {
  if (!animationsAllowed() || event.pointerType === 'touch') return
  const distance = Math.hypot(event.clientX - lastX, event.clientY - lastY)
  const now = Date.now()
  if (distance < 5 && now - lastSpawnAt < 24) return

  lastX = event.clientX
  lastY = event.clientY
  lastSpawnAt = now
  spawnParticles(event.clientX, event.clientY)
}

function drawParticle(particle: TrailParticle, alpha: number) {
  if (!context) return
  const ctx = context
  ctx.save()
  ctx.globalAlpha = alpha * particle.opacity
  ctx.globalCompositeOperation = particle.blend
  ctx.fillStyle = particle.color
  ctx.strokeStyle = particle.color
  ctx.translate(particle.x, particle.y)
  ctx.rotate(particle.angle)

  switch (particle.shape) {
    case 'brush':
      ctx.filter = 'blur(1.4px)'
      ctx.beginPath()
      ctx.ellipse(0, 0, particle.size * 1.35, particle.size * 0.34, 0, 0, Math.PI * 2)
      ctx.fill()
      break
    case 'bubble':
      ctx.lineWidth = Math.max(1, particle.size * 0.18)
      ctx.beginPath()
      ctx.arc(0, 0, particle.size, 0, Math.PI * 2)
      ctx.stroke()
      break
    case 'ribbon':
      ctx.lineCap = 'round'
      ctx.lineWidth = particle.size
      ctx.shadowColor = particle.color
      ctx.shadowBlur = 7
      ctx.beginPath()
      ctx.moveTo(particle.previousX - particle.x, particle.previousY - particle.y)
      ctx.lineTo(0, 0)
      ctx.stroke()
      break
    case 'ember':
      ctx.lineCap = 'round'
      ctx.lineWidth = Math.max(1, particle.size * 0.55)
      ctx.shadowColor = particle.color
      ctx.shadowBlur = 5
      ctx.beginPath()
      ctx.moveTo(0, particle.size * 1.8)
      ctx.lineTo(0, -particle.size * 0.7)
      ctx.stroke()
      break
    case 'leaf':
      ctx.beginPath()
      ctx.ellipse(0, 0, particle.size, particle.size * 0.42, 0, 0, Math.PI * 2)
      ctx.fill()
      ctx.strokeStyle = 'rgba(255, 255, 255, 0.42)'
      ctx.lineWidth = 0.7
      ctx.beginPath()
      ctx.moveTo(-particle.size * 0.65, 0)
      ctx.lineTo(particle.size * 0.65, 0)
      ctx.stroke()
      break
    case 'petal':
      ctx.beginPath()
      ctx.ellipse(0, 0, particle.size * 0.72, particle.size, 0, 0, Math.PI * 2)
      ctx.fill()
      break
    case 'star':
      ctx.lineCap = 'round'
      ctx.lineWidth = Math.max(1, particle.size * 0.32)
      ctx.shadowColor = particle.color
      ctx.shadowBlur = 8
      ctx.beginPath()
      ctx.moveTo(-particle.size, 0)
      ctx.lineTo(particle.size, 0)
      ctx.moveTo(0, -particle.size)
      ctx.lineTo(0, particle.size)
      ctx.stroke()
      break
    case 'spark':
      ctx.fillRect(-particle.size / 2, -particle.size / 2, particle.size, particle.size)
      break
    case 'pixel':
      ctx.fillRect(-particle.size / 2, -particle.size / 2, particle.size, particle.size)
      ctx.strokeStyle = 'rgba(255, 255, 255, 0.3)'
      ctx.lineWidth = 0.6
      ctx.strokeRect(-particle.size / 2, -particle.size / 2, particle.size, particle.size)
      break
    default:
      ctx.beginPath()
      ctx.arc(0, 0, particle.size, 0, Math.PI * 2)
      ctx.fill()
  }

  ctx.restore()
}

function render() {
  if (!context) return
  context.clearRect(0, 0, window.innerWidth, window.innerHeight)

  for (let index = particles.length - 1; index >= 0; index -= 1) {
    const particle = particles[index]
    particle.previousX = particle.x
    particle.previousY = particle.y
    particle.velocityX *= particle.drag
    particle.velocityY = particle.velocityY * particle.drag + particle.gravity
    particle.x += particle.velocityX
    particle.y += particle.velocityY
    particle.angle += particle.spin
    particle.life -= 1

    if (particle.life <= 0) {
      particles.splice(index, 1)
      continue
    }

    const progress = particle.life / particle.maxLife
    drawParticle(particle, Math.min(1, progress * 1.8) * progress)
  }

  animationFrame = particles.length ? requestAnimationFrame(render) : 0
  if (!animationFrame && canvasRef.value) canvasRef.value.dataset.activeParticles = '0'
}

function clearTrail() {
  particles.length = 0
  if (animationFrame) cancelAnimationFrame(animationFrame)
  animationFrame = 0
  if (canvasRef.value) canvasRef.value.dataset.activeParticles = '0'
  context?.clearRect(0, 0, window.innerWidth, window.innerHeight)
}

onMounted(() => {
  const canvas = canvasRef.value
  if (!canvas) return
  context = canvas.getContext('2d')
  if (!context) return

  motionQuery = window.matchMedia('(prefers-reduced-motion: reduce)')
  pointerQuery = window.matchMedia('(pointer: fine)')
  syncTheme()
  resizeCanvas()

  themeObserver = new MutationObserver(syncTheme)
  themeObserver.observe(document.documentElement, { attributes: true, attributeFilter: ['data-ui-style'] })
  window.addEventListener('pointermove', handlePointerMove, { passive: true })
  window.addEventListener('resize', resizeCanvas)
  motionQuery.addEventListener('change', clearTrail)
  pointerQuery.addEventListener('change', clearTrail)
})

onBeforeUnmount(() => {
  clearTrail()
  themeObserver?.disconnect()
  window.removeEventListener('pointermove', handlePointerMove)
  window.removeEventListener('resize', resizeCanvas)
  motionQuery?.removeEventListener('change', clearTrail)
  pointerQuery?.removeEventListener('change', clearTrail)
})
</script>

<style scoped>
.theme-pointer-trail {
  position: fixed;
  inset: 0;
  z-index: 80;
  display: block;
  width: 100vw;
  height: 100vh;
  pointer-events: none;
  contain: strict;
}

@media (pointer: coarse), (prefers-reduced-motion: reduce) {
  .theme-pointer-trail {
    display: none;
  }
}
</style>
