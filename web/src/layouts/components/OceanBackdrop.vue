<template>
  <div
    class="ocean-backdrop"
    aria-hidden="true"
  >
    <img
      class="ocean-image"
      :src="oceanImage"
      alt=""
    >
    <div class="surface-shimmer" />
    <SardineSchool
      class="backdrop-school"
      :count="68"
      :fish-scale="0.9"
      sidebar-offset
    />
    <div class="water-haze" />
    <div class="bubbles">
      <i
        v-for="bubble in bubbles"
        :key="bubble.left"
        :style="bubble"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import oceanImage from '@/assets/aiops-ocean-depth.jpg'
import SardineSchool from './SardineSchool.vue'

const bubbles = [
  { left: '8%', animationDelay: '-3s', animationDuration: '15s', width: '5px', height: '5px' },
  { left: '16%', animationDelay: '-11s', animationDuration: '19s', width: '9px', height: '9px' },
  { left: '31%', animationDelay: '-7s', animationDuration: '17s', width: '4px', height: '4px' },
  { left: '68%', animationDelay: '-14s', animationDuration: '22s', width: '7px', height: '7px' },
  { left: '78%', animationDelay: '-5s', animationDuration: '16s', width: '4px', height: '4px' },
  { left: '91%', animationDelay: '-17s', animationDuration: '24s', width: '10px', height: '10px' }
]
</script>

<style scoped>
.ocean-backdrop {
  position: fixed;
  inset: 0;
  z-index: 0;
  overflow: hidden;
  background: #063b45;
  pointer-events: none;
}

.ocean-image,
.surface-shimmer,
.water-haze,
.backdrop-school,
.bubbles {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
}

.ocean-image {
  object-fit: cover;
  object-position: 50% 46%;
  filter: saturate(0.92) contrast(1.03) brightness(0.84);
  transform: scale(1.035);
  animation: ocean-breathe 22s ease-in-out infinite alternate;
}

.surface-shimmer {
  opacity: 0.48;
  background:
    radial-gradient(ellipse 48% 26% at 56% -3%, rgba(220, 255, 246, 0.72), transparent 68%),
    repeating-radial-gradient(ellipse at 58% -10%, transparent 0 24px, rgba(209, 255, 242, 0.12) 27px, transparent 32px);
  mix-blend-mode: screen;
  animation: surface-current 16s ease-in-out infinite alternate;
}

.backdrop-school {
  z-index: 2;
  opacity: 0.92;
}

.water-haze {
  z-index: 3;
  background:
    radial-gradient(ellipse 32% 27% at 58% 50%, rgba(7, 58, 66, 0.05) 0%, rgba(5, 48, 58, 0.16) 58%, transparent 100%),
    linear-gradient(90deg, rgba(2, 28, 37, 0.48), transparent 22%, transparent 79%, rgba(1, 27, 35, 0.4)),
    linear-gradient(180deg, transparent 54%, rgba(1, 25, 34, 0.4));
}

.bubbles {
  z-index: 4;
}

.bubbles i {
  position: absolute;
  bottom: -18px;
  border: 1px solid rgba(220, 255, 249, 0.48);
  border-radius: 50%;
  box-shadow: inset 1px 1px 2px rgba(255, 255, 255, 0.35);
  animation: bubble-rise linear infinite;
}

@keyframes ocean-breathe {
  from { transform: scale(1.035) translate3d(-0.4%, 0, 0); }
  to { transform: scale(1.075) translate3d(0.5%, -0.7%, 0); }
}

@keyframes surface-current {
  from { transform: translate3d(-1.5%, 0, 0) scale(1.02); }
  to { transform: translate3d(1.5%, 1%, 0) scale(1.08); }
}

@keyframes bubble-rise {
  0% { opacity: 0; transform: translate3d(0, 0, 0) scale(0.65); }
  12% { opacity: 0.55; }
  82% { opacity: 0.34; }
  100% { opacity: 0; transform: translate3d(36px, -106vh, 0) scale(1.25); }
}

@media (max-width: 900px) {
  .ocean-image { object-position: 58% 45%; }
  .backdrop-school { opacity: 0.78; }
}

@media (prefers-reduced-motion: reduce) {
  .ocean-image,
  .surface-shimmer,
  .bubbles i {
    animation: none;
  }

  .bubbles { display: none; }
}
</style>
