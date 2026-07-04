<script setup>
import { computed } from 'vue'

// A labeled load bar that turns amber at 60% and red at 80%.
const props = defineProps({
  pct: { type: Number, default: 0 },
  label: { type: String, default: '' },
})

const color = computed(() => {
  if (props.pct >= 80) return 'var(--err)'
  if (props.pct >= 60) return '#e0a52e'
  return 'var(--accent)'
})
</script>

<template>
  <div style="display:flex;align-items:center;gap:0.4rem">
    <div class="bar" style="flex:1;margin:0"><i :style="{ width: Math.min(100, pct) + '%', background: color }"></i></div>
    <span style="width:3.6rem;text-align:right;font-size:0.78rem" :style="{ color }">{{ label || (pct + '%') }}</span>
  </div>
</template>
