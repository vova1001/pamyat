import { defineConfig } from 'astro/config';
import node from '@astrojs/node';

// Сайт рендерится на сервере при каждом запросе: изменения из кабинета видны гостям сразу.
export default defineConfig({
  output: 'server',
  adapter: node({ mode: 'standalone' }),
  server: { host: true, port: 4321 },
  security: { checkOrigin: false }, // формы — на API бэкенда, у Astro своих POST нет
  devToolbar: { enabled: false },
  vite: { worker: { format: 'es' } }, // воркер карты MapLibre — ES-модуль
});
