import { createApp } from 'vue';
import { createPinia } from 'pinia';
import App from './App.vue';
import { installVersionRefresh } from './runtime/appVersion';
import {
  installAutomaticFeedbackReporting,
  reportAutomaticFeedback,
} from './runtime/errorFeedback';
import './styles.css';
import './styles/fonts.css';
import './styles/tokens.css';
import './styles/base.css';
import './styles/layout.css';
import './styles/composition.css';

installVersionRefresh();
installAutomaticFeedbackReporting();

const app = createApp(App);
app.config.errorHandler = (error, _instance, info) => {
  void reportAutomaticFeedback(error, {
    actionContext: `vue_${String(info || 'component_error')}`,
  });
};
const pinia = createPinia();
app.use(pinia);
app.mount('#app');

if (typeof window !== 'undefined') {
  window.__pinia__ = pinia;
}
