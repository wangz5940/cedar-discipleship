<script setup>
import { onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { fetchWithAuth } from '../legacy-app';

const props = defineProps({ src: { type: String, required: true }, alt: { type: String, default: '' }, lazy: { type: Boolean, default: false } });
const element = ref(null);
const resolved = ref(props.src);
const active = ref(!props.lazy);
const warning = ref('');
let controller = null;
let objectURL = '';
let observer = null;
let generation = 0;

function dataURL(blob) {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result);
    reader.onerror = () => reject(reader.error);
    reader.readAsDataURL(blob);
  });
}
async function resolveSVG() {
  const sequence = ++generation;
  controller?.abort();
  if (objectURL) { URL.revokeObjectURL(objectURL); objectURL = ''; }
  resolved.value = props.src;
  warning.value = '';
  if (!active.value || !/\.svg(?:[?#]|$)/i.test(props.src)) return;
  controller = new AbortController();
  const signal = controller.signal;
  const slideURL = new URL(props.src, window.location.href);
  const read = (url) => url.origin === window.location.origin && url.pathname.startsWith('/api/')
    ? fetchWithAuth(url.href, { signal }) : fetch(url.href, { signal, credentials: 'omit', cache: 'reload' });
  try {
    const response = await read(slideURL);
    if (!response.ok) throw new Error('slide_unavailable');
    const document = new DOMParser().parseFromString(await response.text(), 'image/svg+xml');
    if (document.querySelector('parsererror') || document.documentElement.localName !== 'svg') throw new Error('invalid_svg');
    document.querySelectorAll('script, foreignObject').forEach(node => node.remove());
    document.querySelectorAll('*').forEach(node => {
      [...node.attributes].filter(attribute => /^on/i.test(attribute.name) || /^javascript:/i.test(attribute.value)).forEach(attribute => node.removeAttribute(attribute.name));
    });
    // SVG displayed in an <img> cannot load external image references: embed their bytes.
    await Promise.all([...document.querySelectorAll('image')].map(async node => {
      const href = node.getAttribute('href') || node.getAttribute('xlink:href');
      if (!href || href.startsWith('data:')) return;
      const url = new URL(href, slideURL);
      if (!['http:', 'https:'].includes(url.protocol)) return;
      const image = await read(url);
      if (!image.ok) throw new Error('slide_image_unavailable');
      const embedded = await dataURL(await image.blob());
      node.setAttribute('href', embedded);
      node.setAttributeNS('http://www.w3.org/1999/xlink', 'xlink:href', embedded);
    }));
    if (sequence !== generation) return;
    objectURL = URL.createObjectURL(new Blob([new XMLSerializer().serializeToString(document)], { type: 'image/svg+xml' }));
    resolved.value = objectURL;
  } catch (error) {
    if (sequence === generation && error.name !== 'AbortError') {
      warning.value = '讲义中的图片未完全加载，请刷新重试。';
      if (import.meta.env.DEV) console.warn('讲义图片加载失败', error);
    }
  }
}
watch([() => props.src, active], resolveSVG, { immediate: true });
onMounted(() => {
  if (!props.lazy) return;
  observer = new IntersectionObserver(entries => {
    if (entries.some(entry => entry.isIntersecting)) { active.value = true; observer.disconnect(); }
  });
  observer.observe(element.value);
});
onBeforeUnmount(() => { generation += 1; controller?.abort(); observer?.disconnect(); if (objectURL) URL.revokeObjectURL(objectURL); });
</script>

<template>
  <img ref="element" :src="resolved" :alt="alt" :loading="lazy ? 'lazy' : 'eager'" :title="warning || undefined" />
</template>
