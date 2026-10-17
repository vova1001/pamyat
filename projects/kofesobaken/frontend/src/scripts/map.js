// Карта «Как нас найти» — по эскизу preview/map-render.html: обычная карта с улицами (OpenFreeMap liberty),
// подписи по-русски, дома полупрозрачные, наклон 35°; кафе — фирменная метка .mpin.
// Живую карту показываем, только когда реально загрузились и нарисовались улицы и их подписи.
// Иначе (нет сети, плитки не пришли, нет WebGL) остаётся снимок img/map-streets.jpg с меткой в центре.
(() => {
  const box = document.getElementById('map3d'); if (!box) return;
  const CAFE = [39.771960, 47.229234];
  const STYLE = 'https://tiles.openfreemap.org/styles/liberty';
  const VIEW = { center: CAFE, zoom: 16.4, pitch: 35, bearing: -10 };
  const ru = ['coalesce', ['get', 'name:ru'], ['get', 'name']];

  /** Стиль liberty: подписи по-русски, объёмные дома полупрозрачные. */
  function patch(st) {
    st.layers.forEach(l => {
      if (l.layout && l.layout['text-field']) l.layout['text-field'] = ru;
      if (l.type === 'fill-extrusion') l.paint['fill-extrusion-opacity'] = 0.55;
    });
    return st;
  }

  let started = false;
  // MapLibre (из npm, собирается с сайтом) и стиль грузятся, только когда карта близко к экрану.
  const start = async () => {
    if (started) return; started = true;
    try {
      const [mod, worker, st] = await Promise.all([
        import('maplibre-gl'),
        import('maplibre-gl/dist/maplibre-gl-worker.mjs?worker&url'),
        fetch(STYLE).then(r => { if (!r.ok) throw new Error('style ' + r.status); return r.json(); }),
        import('maplibre-gl/dist/maplibre-gl.css'),
      ]);
      const ml = mod.default || mod;
      ml.setWorkerUrl(worker.default);
      init(ml, patch(st));
    } catch (e) { /* нет сети или WebGL — остаётся снимок */ }
  };

  function init(ml, style) {
    let map;
    try {
      map = new ml.Map({ container: box.querySelector('.map__live'), style, ...VIEW, maxPitch: 60, minZoom: 13, maxZoom: 19, attributionControl: { compact: true }, cooperativeGestures: true, scrollZoom: false, fadeDuration: 0,
        locale: { 'CooperativeGesturesHandler.WindowsHelpText': 'Ctrl + колесо мыши — приблизить карту', 'CooperativeGesturesHandler.MacHelpText': '⌘ + колесо мыши — приблизить карту', 'CooperativeGesturesHandler.MobileHelpText': 'Двигайте карту двумя пальцами', 'NavigationControl.ZoomIn': 'Приблизить', 'NavigationControl.ZoomOut': 'Отдалить', 'NavigationControl.ResetBearing': 'Вернуть север' } });
    } catch (e) { return; } // нет WebGL — остаётся снимок
    window.kofeMap = map;
    map.addControl(new ml.NavigationControl({ visualizePitch: true }), 'top-right');
    map.on('error', e => console.warn('map:', e.error && e.error.message));

    const ids = sl => style.layers.filter(l => l['source-layer'] === sl).map(l => l.id);
    const roads = ids('transportation'), names = ids('transportation_name');
    let tiles = false, live = false;
    // плитки улиц пришли (событие загрузки плиток) → после отрисовки проверяем, что улицы и подписи реально на экране
    map.on('data', e => { if (e.dataType === 'source' && e.sourceId === 'openmaptiles' && e.tile) tiles = true; });
    const check = () => {
      if (live || !tiles) return;
      if (!map.queryRenderedFeatures({ layers: roads }).length || !map.queryRenderedFeatures({ layers: names }).length) return;
      live = true;
      box.classList.add('is-live');
      new ml.Marker({ element: box.querySelector('.mpin'), anchor: 'bottom' }).setLngLat(CAFE).addTo(map);
      box.querySelector('.maplibregl-ctrl-attrib')?.classList.remove('maplibregl-compact-show'); // свернуть подпись, чтобы не налезала на кнопку
    };
    map.on('idle', check);
  }

  if ('IntersectionObserver' in window) {
    const io = new IntersectionObserver(([e]) => { if (e.isIntersecting) { io.disconnect(); start(); } }, { rootMargin: '900px 0px' });
    io.observe(box);
  } else start();
})();
