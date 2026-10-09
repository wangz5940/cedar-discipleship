import { ref } from 'vue';

export const installPrompt = ref(null);
export const installed = ref(false);

export function installPwaSupport() {
  const standalone = window.matchMedia('(display-mode: standalone)');
  installed.value = standalone.matches || navigator.standalone === true;
  const offer = event => { event.preventDefault(); installPrompt.value = event; };
  const complete = () => { installed.value = true; installPrompt.value = null; };
  window.addEventListener('beforeinstallprompt', offer);
  window.addEventListener('appinstalled', complete);
  if (window.isSecureContext && 'serviceWorker' in navigator) {
    void navigator.serviceWorker.register('/learning-notifications-sw.js').catch(() => {});
  }
  return () => {
    window.removeEventListener('beforeinstallprompt', offer);
    window.removeEventListener('appinstalled', complete);
  };
}

export async function requestAppInstall() {
  const event = installPrompt.value;
  if (!event) return 'manual';
  await event.prompt();
  const result = await event.userChoice;
  if (installPrompt.value === event) installPrompt.value = null;
  return result.outcome;
}
