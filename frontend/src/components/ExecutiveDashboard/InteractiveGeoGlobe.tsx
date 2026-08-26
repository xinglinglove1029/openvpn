import { Focus, Minus, MousePointer2, Pause, Play, Plus, RotateCcw } from 'lucide-react';
import { useEffect, useMemo, useRef, useState } from 'react';
import type { DashboardGeoPoint } from '@/types';
import { cn } from '@/lib/utils';
import { useTheme, type ThemeKey } from '@/store/theme';

export type GeoGlobeView = 'world' | 'china' | 'country';

export type GeoGlobeCountry = {
  name: string;
  longitude: number;
  latitude: number;
};

export type GeoGlobeMarker = {
  id: string;
  label: string;
  count: number;
  longitude: number;
  latitude: number;
  point: DashboardGeoPoint;
};

type GlobeCommand = 'zoom-in' | 'zoom-out' | 'reset';

type InteractiveGeoGlobeProps = {
  markers: GeoGlobeMarker[];
  view: GeoGlobeView;
  tone?: 'emerald' | 'sky' | 'violet';
  className?: string;
  emptyMessage?: string;
  onMarkerSelect?: (marker: GeoGlobeMarker) => void;
  // Country labels remain available when the selected data source has no
  // geolocated records, so operators can still inspect administrative maps.
  onCountrySelect?: (country: GeoGlobeCountry) => void;
};

const TONE_COLORS = {
  emerald: '#20c997',
  sky: '#2096f3',
  violet: '#8067ff',
};

// Theme colors decorate controls, signals and the surrounding space. The earth
// itself intentionally uses physical imagery so its continents and oceans stay
// recognisable in every console theme.
const THEME_GLOBE_COLORS: Record<ThemeKey, { orbit: string; stars: string; light: string }> = {
  midnight: { orbit: '#93c5fd', stars: '#bfdbfe', light: '#dbeafe' },
  aurora: { orbit: '#c4b5fd', stars: '#e9d5ff', light: '#f5d0fe' },
  emerald: { orbit: '#86efac', stars: '#d1fae5', light: '#d1fae5' },
  daylight: { orbit: '#60a5fa', stars: '#bfdbfe', light: '#dbeafe' },
  'amber-glass': { orbit: '#fcd34d', stars: '#fef3c7', light: '#fde68a' },
  'deep-blue': { orbit: '#93c5fd', stars: '#bfdbfe', light: '#dbeafe' },
};

const EARTH_TEXTURE_BASE = `${import.meta.env.BASE_URL}maps/earth/`;

type RenderQuality = {
  antialias: boolean;
  decoration: boolean;
  frameInterval: number;
  globeHeightSegments: number;
  globeWidthSegments: number;
  markerSegments: number;
  maxAnisotropy: number;
  maxFlightMarkers: number;
  pixelRatio: number;
  starCount: number;
};

// The operations screen can be opened on laptops, thin clients and small
// management servers. Keep the rich experience on capable devices while
// avoiding a 4K/60 FPS renderer on 1-core / 2 GB installations.
function getRenderQuality(): RenderQuality {
  const device = navigator as Navigator & { deviceMemory?: number };
  const reducedMotion = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false;
  const constrained = reducedMotion || (device.hardwareConcurrency ?? 4) <= 2 || (device.deviceMemory ?? 4) <= 2;

  return constrained
    ? {
        antialias: false,
        decoration: false,
        frameInterval: 1000 / 24,
        globeHeightSegments: 32,
        globeWidthSegments: 48,
        markerSegments: 12,
        maxAnisotropy: 1,
        maxFlightMarkers: 3,
        pixelRatio: 1,
        starCount: 160,
      }
    : {
        antialias: true,
        decoration: true,
        frameInterval: 1000 / 30,
        globeHeightSegments: 48,
        globeWidthSegments: 72,
        markerSegments: 16,
        maxAnisotropy: 4,
        maxFlightMarkers: 6,
        pixelRatio: 1.25,
        starCount: 360,
      };
}

type CountryLabel = GeoGlobeCountry;

// Country labels are intentionally limited to large and commonly recognised
// countries/regions. Showing every sovereign state at this globe size would
// overlap heavily and make the geographic view harder to read.
const WORLD_COUNTRY_LABELS: CountryLabel[] = [
  { name: '加拿大', longitude: -106.35, latitude: 56.13 },
  { name: '美国', longitude: -98.58, latitude: 39.83 },
  { name: '墨西哥', longitude: -102.55, latitude: 23.63 },
  { name: '巴西', longitude: -51.93, latitude: -14.24 },
  { name: '阿根廷', longitude: -63.62, latitude: -38.42 },
  { name: '英国', longitude: -3.44, latitude: 55.38 },
  { name: '法国', longitude: 2.21, latitude: 46.23 },
  { name: '德国', longitude: 10.45, latitude: 51.17 },
  { name: '西班牙', longitude: -3.75, latitude: 40.46 },
  { name: '意大利', longitude: 12.57, latitude: 41.87 },
  { name: '俄罗斯', longitude: 105.32, latitude: 61.52 },
  { name: '土耳其', longitude: 35.24, latitude: 38.96 },
  { name: '埃及', longitude: 30.8, latitude: 26.82 },
  { name: '南非', longitude: 22.94, latitude: -30.56 },
  { name: '沙特', longitude: 45.08, latitude: 23.89 },
  { name: '印度', longitude: 78.96, latitude: 20.59 },
  { name: '中国', longitude: 104.2, latitude: 35.86 },
  { name: '日本', longitude: 138.25, latitude: 36.2 },
  { name: '韩国', longitude: 127.77, latitude: 36.5 },
  { name: '印度尼西亚', longitude: 113.92, latitude: -0.79 },
  { name: '澳大利亚', longitude: 133.78, latitude: -25.27 },
];

function globeVector(THREE: typeof import('three'), longitude: number, latitude: number, radius: number) {
  const lng = (longitude * Math.PI) / 180;
  const lat = (latitude * Math.PI) / 180;
  return new THREE.Vector3(
    radius * Math.cos(lat) * Math.sin(lng),
    radius * Math.sin(lat),
    radius * Math.cos(lat) * Math.cos(lng)
  );
}

function createCountryLabelSprite(THREE: typeof import('three'), label: string, color: string) {
  const canvas = document.createElement('canvas');
  const context = canvas.getContext('2d');
  if (!context) throw new Error('Canvas 2D context is unavailable for country labels.');

  const fontSize = 34;
  context.font = `600 ${fontSize}px ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif`;
  const paddingX = 18;
  const paddingY = 10;
  const textWidth = Math.ceil(context.measureText(label).width);
  canvas.width = textWidth + paddingX * 2;
  canvas.height = fontSize + paddingY * 2;

  context.font = `600 ${fontSize}px ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif`;
  context.textAlign = 'center';
  context.textBaseline = 'middle';
  context.lineJoin = 'round';
  context.shadowColor = 'rgba(2, 6, 23, 0.88)';
  context.shadowBlur = 11;
  context.lineWidth = 7;
  context.strokeStyle = 'rgba(2, 6, 23, 0.72)';
  context.strokeText(label, canvas.width / 2, canvas.height / 2 + 1);
  context.shadowBlur = 0;
  context.fillStyle = color;
  context.fillText(label, canvas.width / 2, canvas.height / 2 + 1);

  const texture = new THREE.CanvasTexture(canvas);
  texture.colorSpace = THREE.SRGBColorSpace;
  texture.minFilter = THREE.LinearFilter;
  const material = new THREE.SpriteMaterial({ map: texture, transparent: true, depthWrite: false, depthTest: false, opacity: 0.85 });
  const sprite = new THREE.Sprite(material);
  sprite.scale.set((canvas.width / canvas.height) * 0.27, 0.27, 1);
  sprite.renderOrder = 5;
  return { sprite, texture };
}

// A restrained dot field gives the sphere a geographic silhouette without
// requesting map tiles or sending audit data to a third party.
function createMarkerCountSprite(THREE: typeof import('three'), count: number, color: string) {
  const canvas = document.createElement('canvas');
  const context = canvas.getContext('2d');
  if (!context) throw new Error('Canvas 2D context is unavailable');
  const label = String(count);
  const fontSize = 36;
  context.font = `700 ${fontSize}px ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif`;
  const paddingX = 18;
  const paddingY = 10;
  canvas.width = Math.ceil(context.measureText(label).width) + paddingX * 2;
  canvas.height = fontSize + paddingY * 2;
  context.font = `700 ${fontSize}px ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif`;
  context.textAlign = 'center';
  context.textBaseline = 'middle';
  context.lineJoin = 'round';
  context.lineWidth = 8;
  context.strokeStyle = 'rgba(2, 6, 23, 0.88)';
  context.strokeText(label, canvas.width / 2, canvas.height / 2 + 1);
  context.fillStyle = color;
  context.fillText(label, canvas.width / 2, canvas.height / 2 + 1);
  const texture = new THREE.CanvasTexture(canvas);
  texture.colorSpace = THREE.SRGBColorSpace;
  texture.minFilter = THREE.LinearFilter;
  const material = new THREE.SpriteMaterial({ map: texture, transparent: true, depthWrite: false, depthTest: false });
  const sprite = new THREE.Sprite(material);
  sprite.scale.set((canvas.width / canvas.height) * 0.21, 0.21, 1);
  sprite.renderOrder = 8;
  return { sprite, texture };
}

export function InteractiveGeoGlobe({
  markers,
  view,
  tone = 'sky',
  className,
  emptyMessage = '当前范围没有可定位的区域数据。',
  onMarkerSelect,
  onCountrySelect,
}: InteractiveGeoGlobeProps) {
  const { theme } = useTheme();
  const mountRef = useRef<HTMLDivElement | null>(null);
  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  const controlsRef = useRef<((command: GlobeCommand) => void) | null>(null);
  const motionPausedRef = useRef(false);
  const [motionPaused, setMotionPaused] = useState(false);
  const [selected, setSelected] = useState<GeoGlobeMarker | null>(null);
  const [renderError, setRenderError] = useState(false);
  const markerSignature = useMemo(
    () => markers.map((marker) => `${marker.id}:${marker.count}:${marker.longitude}:${marker.latitude}`).join('|'),
    [markers]
  );

  useEffect(() => {
    setSelected(null);
  }, [markerSignature, view]);

  useEffect(() => {
    motionPausedRef.current = motionPaused;
  }, [motionPaused]);

  useEffect(() => {
    const mount = mountRef.current;
    const canvas = canvasRef.current;
    if (!mount || !canvas) return;

    let disposed = false;
    let cleanup: (() => void) | undefined;
    setRenderError(false);

    import('three')
      .then((THREE) => {
        if (disposed || mountRef.current !== mount || canvasRef.current !== canvas) return;

        try {
          const scene = new THREE.Scene();
          const camera = new THREE.PerspectiveCamera(42, 1, 0.1, 100);
          const defaultZoom = view === 'china' ? 4.25 : 4.7;
          const sceneCenter = new THREE.Vector3(0, 0, 0);
          const quality = getRenderQuality();
          camera.position.set(0, 0, defaultZoom);
          camera.lookAt(sceneCenter);

          const renderer = new THREE.WebGLRenderer({
            canvas,
            alpha: true,
            antialias: quality.antialias,
            // A low-power context is friendlier to integrated GPUs and avoids
            // contending with the OpenVPN server on small deployments.
            powerPreference: quality.decoration ? 'high-performance' : 'low-power',
          });
          renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, quality.pixelRatio));
          renderer.outputColorSpace = THREE.SRGBColorSpace;
          renderer.setClearColor(0x000000, 0);

          const palette = THEME_GLOBE_COLORS[theme];
          const accent = new THREE.Color(TONE_COLORS[tone]);
          const accentLight = accent.clone().lerp(new THREE.Color(palette.orbit), 0.5);
          const globe = new THREE.Group();
          scene.add(globe);

          // 1024px equirectangular assets are deliberately used here. The globe
          // is displayed in a ~500px panel, so 4K maps only consumed GPU memory
          // (roughly 64 MB per decoded texture) without a visible benefit.
          const textureLoader = new THREE.TextureLoader();
          const configureTexture = (texture: import('three').Texture, srgb = false) => {
            if (srgb) texture.colorSpace = THREE.SRGBColorSpace;
            texture.anisotropy = Math.min(quality.maxAnisotropy, renderer.capabilities.getMaxAnisotropy());
            texture.wrapS = THREE.ClampToEdgeWrapping;
            texture.wrapT = THREE.ClampToEdgeWrapping;
            return texture;
          };
          const earthDayTexture = configureTexture(textureLoader.load(`${EARTH_TEXTURE_BASE}earth_day_1024.jpg`), true);
          const earthNightTexture = quality.decoration
            ? configureTexture(textureLoader.load(`${EARTH_TEXTURE_BASE}earth_night_1024.jpg`), true)
            : null;
          const earthBumpTexture = quality.decoration
            ? configureTexture(textureLoader.load(`${EARTH_TEXTURE_BASE}earth_bump_roughness_clouds_1024.jpg`))
            : null;
          const surfaceMaterial = quality.decoration
            ? new THREE.MeshPhongMaterial({
                map: earthDayTexture,
                bumpMap: earthBumpTexture,
                bumpScale: 0.038,
                specularMap: earthBumpTexture,
                specular: new THREE.Color('#35506d'),
                shininess: 7,
                emissiveMap: earthNightTexture,
                emissive: new THREE.Color('#18233f'),
                emissiveIntensity: 0.34,
              })
            : new THREE.MeshBasicMaterial({ map: earthDayTexture });
          const surface = new THREE.Mesh(
            new THREE.SphereGeometry(1.42, quality.globeWidthSegments, quality.globeHeightSegments),
            surfaceMaterial
          );
          surface.rotation.y = Math.PI;
          globe.add(surface);

          // Keep the operations-console grid as a quiet overlay; it must not obscure
          // the terrain texture or turn the globe back into a synthetic wireframe.
          if (quality.decoration) {
            const grid = new THREE.Mesh(
              new THREE.SphereGeometry(1.435, 38, 24),
              new THREE.MeshBasicMaterial({ color: accentLight, transparent: true, opacity: 0.055, wireframe: true, depthWrite: false })
            );
            grid.rotation.y = Math.PI;
            globe.add(grid);
          }

          // Decorative scan rings are skipped on constrained devices. The
          // textured surface and interactive markers remain available.
          if (quality.decoration) {
            const scanRing = new THREE.Mesh(
              new THREE.TorusGeometry(1.61, 0.009, 10, 96),
              new THREE.MeshBasicMaterial({ color: accentLight, transparent: true, opacity: 0.28, blending: THREE.AdditiveBlending, depthWrite: false })
            );
            scanRing.rotation.x = Math.PI / 2;
            globe.add(scanRing);
          }

          const orbitGroup = new THREE.Group();
          const orbitOne = new THREE.Mesh(
            new THREE.TorusGeometry(1.92, 0.008, 8, quality.decoration ? 132 : 72),
            new THREE.MeshBasicMaterial({ color: accent, transparent: true, opacity: 0.45 })
          );
          orbitOne.rotation.x = Math.PI / 2.25;
          orbitGroup.add(orbitOne);
          if (quality.decoration) {
            const orbitTwo = new THREE.Mesh(
              new THREE.TorusGeometry(2.26, 0.006, 8, 132),
              new THREE.MeshBasicMaterial({ color: palette.orbit, transparent: true, opacity: 0.22 })
            );
            orbitTwo.rotation.x = Math.PI / 2.8;
            orbitTwo.rotation.z = 0.55;
            orbitGroup.add(orbitTwo);
            const satellite = new THREE.Group();
            const satelliteCore = new THREE.Mesh(
              new THREE.IcosahedronGeometry(0.052, 1),
              new THREE.MeshBasicMaterial({ color: '#ffffff', transparent: true, opacity: 0.95 })
            );
            const solarPanel = new THREE.Mesh(
              new THREE.BoxGeometry(0.16, 0.012, 0.06),
              new THREE.MeshBasicMaterial({ color: accent, transparent: true, opacity: 0.88 })
            );
            satellite.add(satelliteCore, solarPanel);
            orbitGroup.add(satellite);
          }
          scene.add(orbitGroup);

          const stars = new Float32Array(quality.starCount * 3);
          for (let index = 0; index < quality.starCount; index += 1) {
            const radius = 2.55 + Math.random() * 2.35;
            const phi = Math.acos(2 * Math.random() - 1);
            const theta = Math.random() * Math.PI * 2;
            stars[index * 3] = radius * Math.sin(phi) * Math.cos(theta);
            stars[index * 3 + 1] = radius * Math.cos(phi);
            stars[index * 3 + 2] = radius * Math.sin(phi) * Math.sin(theta);
          }
          const starGeometry = new THREE.BufferGeometry();
          starGeometry.setAttribute('position', new THREE.BufferAttribute(stars, 3));
          const starField = new THREE.Points(
            starGeometry,
            new THREE.PointsMaterial({
              color: palette.stars,
              size: 0.018,
              transparent: true,
              opacity: 0.62,
              depthWrite: false,
            })
          );
          scene.add(starField);

          const countryLabelSprites: import('three').Sprite[] = [];
          const countryLabelTextures: import('three').Texture[] = [];
          // Both live data markers and the always-visible country labels are
          // raycast targets. Labels provide a no-data drilldown path.
          const clickTargets: import('three').Object3D[] = [];
          if (view === 'world') {
            WORLD_COUNTRY_LABELS.forEach(({ name, longitude, latitude }) => {
              const { sprite, texture } = createCountryLabelSprite(THREE, name, palette.light);
              sprite.position.copy(globeVector(THREE, longitude, latitude, 1.57));
              sprite.userData.country = { name, longitude, latitude } satisfies GeoGlobeCountry;
              globe.add(sprite);
              clickTargets.push(sprite);
              countryLabelSprites.push(sprite);
              countryLabelTextures.push(texture);
            });
          }
          const globeWorldPosition = new THREE.Vector3();
          const labelWorldPosition = new THREE.Vector3();
          const labelNormal = new THREE.Vector3();
          const cameraDirection = new THREE.Vector3();

          scene.add(new THREE.HemisphereLight('#dceeff', '#071226', 1.18));
          const sunLight = new THREE.DirectionalLight('#fff4dc', 3.0);
          sunLight.position.set(-3.8, 2.7, 4.6);
          scene.add(sunLight);
          const rimLight = new THREE.PointLight(accent, 2.1, 8);
          rimLight.position.set(2.8, -1.4, 2.6);
          scene.add(rimLight);

          // Curved data links and moving signal particles make the global view
          // readable as a live network rather than a static collection of pins.
          const flightParticles: Array<{ curve: import('three').CatmullRomCurve3; particle: import('three').Mesh; speed: number; offset: number }> = [];
          const flightMarkers = [...markers].sort((a, b) => b.count - a.count).slice(0, quality.maxFlightMarkers);
          if (flightMarkers.length > 1) {
            const hub = flightMarkers[0];
            flightMarkers.slice(1).forEach((marker, index) => {
              if (Math.abs(marker.longitude - hub.longitude) < 0.1 && Math.abs(marker.latitude - hub.latitude) < 0.1) return;
              const start = globeVector(THREE, hub.longitude, hub.latitude, 1.48);
              const end = globeVector(THREE, marker.longitude, marker.latitude, 1.48);
              const midpoint = start.clone().add(end).normalize().multiplyScalar(1.72 + Math.min(0.2, start.distanceTo(end) * 0.06));
              const curve = new THREE.CatmullRomCurve3([start, midpoint, end]);
              const path = new THREE.Line(
                new THREE.BufferGeometry().setFromPoints(curve.getPoints(quality.decoration ? 34 : 20)),
                new THREE.LineBasicMaterial({ color: accentLight, transparent: true, opacity: 0.34, blending: THREE.AdditiveBlending, depthWrite: false })
              );
              globe.add(path);
              const particle = new THREE.Mesh(
                new THREE.SphereGeometry(0.018, quality.markerSegments, quality.markerSegments),
                new THREE.MeshBasicMaterial({ color: '#ffffff', transparent: true, opacity: 0.95, blending: THREE.AdditiveBlending })
              );
              globe.add(particle);
              flightParticles.push({ curve, particle, speed: 0.075 + index * 0.011, offset: index / Math.max(1, flightMarkers.length - 1) });
            });
          }

          const markerLabelTextures: import('three').Texture[] = [];
          const markerRoots: Array<{
            root: import('three').Group;
            marker: GeoGlobeMarker;
            halo: import('three').Mesh;
          }> = [];
          const maxCount = Math.max(...markers.map((marker) => marker.count), 1);

          markers.forEach((marker) => {
            const size = 0.052 + Math.min(0.07, (marker.count / maxCount) * 0.07);
            const root = new THREE.Group();
            const anchor = globeVector(THREE, marker.longitude, marker.latitude, 1.472);
            const outward = anchor.clone().normalize();
            root.position.copy(anchor);
            root.quaternion.setFromUnitVectors(new THREE.Vector3(0, 0, 1), outward);

            const column = new THREE.Line(
              new THREE.BufferGeometry().setFromPoints([
                new THREE.Vector3(0, 0, -0.008),
                new THREE.Vector3(0, 0, 0.15 + size),
              ]),
              new THREE.LineBasicMaterial({ color: accent, transparent: true, opacity: 0.76 })
            );
            const halo = new THREE.Mesh(
              new THREE.RingGeometry(size * 0.95, size * 1.42, quality.markerSegments * 2),
              new THREE.MeshBasicMaterial({
                color: accent,
                transparent: true,
                opacity: 0.34,
                side: THREE.DoubleSide,
                blending: THREE.AdditiveBlending,
              })
            );
            halo.position.z = 0.012;
            const core = new THREE.Mesh(
              new THREE.SphereGeometry(size * 0.55, quality.markerSegments, quality.markerSegments),
              new THREE.MeshBasicMaterial({ color: '#ffffff', transparent: true, opacity: 0.98 })
            );
            core.position.z = 0.15 + size;
            const beacon = new THREE.Mesh(
              new THREE.SphereGeometry(size * 0.24, quality.markerSegments, quality.markerSegments),
              new THREE.MeshBasicMaterial({ color: accent, transparent: true, opacity: 0.98 })
            );
            beacon.position.z = 0.15 + size;
            const hitArea = new THREE.Mesh(
              new THREE.SphereGeometry(Math.max(0.09, size * 1.3), 12, 12),
              new THREE.MeshBasicMaterial({ transparent: true, opacity: 0, depthWrite: false })
            );
            hitArea.position.z = 0.15 + size;
            hitArea.userData.marker = marker;
            const countLabel = createMarkerCountSprite(THREE, marker.count, '#ffffff');
            countLabel.sprite.position.z = 0.24 + size * 2.1;
            markerLabelTextures.push(countLabel.texture);
            root.add(column, halo, core, beacon, countLabel.sprite, hitArea);
            globe.add(root);
            clickTargets.push(hitArea);
            markerRoots.push({ root, marker, halo });
          });

          const raycaster = new THREE.Raycaster();
          const pointer = new THREE.Vector2();
          let animationFrame = 0;
          let resizeFrame = 0;
          let lastRenderAt = 0;
          let lastWidth = 0;
          let lastHeight = 0;
          let dragging = false;
          let pointerStart = { x: 0, y: 0 };
          let lastPointer = { x: 0, y: 0 };
          let targetX = view === 'china' ? 0.11 : 0.04;
          let targetY = view === 'china' ? (-104 * Math.PI) / 180 : -0.55;
          let currentZoom = defaultZoom;
          let targetZoom = defaultZoom;
          let pauseUntil = performance.now() + 1350;
          let selectedId = '';
          const reducedMotion = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false;

          const focusMarker = (marker: GeoGlobeMarker) => {
            targetY = (-marker.longitude * Math.PI) / 180;
            targetX = Math.max(-0.42, Math.min(0.42, ((marker.latitude * Math.PI) / 180) * 0.34));
            targetZoom = view === 'china' ? 3.72 : 3.92;
            selectedId = marker.id;
            pauseUntil = performance.now() + 4000;
            setSelected(marker);
            onMarkerSelect?.(marker);
          };

          const reset = () => {
            targetX = view === 'china' ? 0.11 : 0.04;
            targetY = view === 'china' ? (-104 * Math.PI) / 180 : -0.55;
            targetZoom = defaultZoom;
            selectedId = '';
            setSelected(null);
            pauseUntil = performance.now() + 800;
          };

          controlsRef.current = (command) => {
            if (command === 'zoom-in') targetZoom = Math.max(3.05, targetZoom - 0.42);
            if (command === 'zoom-out') targetZoom = Math.min(6.3, targetZoom + 0.42);
            if (command === 'reset') reset();
            pauseUntil = performance.now() + 1600;
          };

          // Use the actual painted box instead of a fixed fallback. Safari can
          // initialise ResizeObserver before a flex/grid item receives its final
          // width; stretching that first backing buffer is what caused the globe
          // to appear off-centre on macOS.
          const resize = () => {
            resizeFrame = 0;
            const rect = mount.getBoundingClientRect();
            const width = Math.max(1, Math.round(rect.width));
            const height = Math.max(1, Math.round(rect.height));
            if (width === lastWidth && height === lastHeight) return;
            lastWidth = width;
            lastHeight = height;
            renderer.setSize(width, height, false);
            renderer.domElement.style.width = `${width}px`;
            renderer.domElement.style.height = `${height}px`;
            camera.aspect = width / height;
            camera.updateProjectionMatrix();
            camera.lookAt(sceneCenter);
          };
          const scheduleResize = () => {
            if (resizeFrame) window.cancelAnimationFrame(resizeFrame);
            resizeFrame = window.requestAnimationFrame(resize);
          };

          const setRayPointer = (event: PointerEvent) => {
            const rect = mount.getBoundingClientRect();
            pointer.x = ((event.clientX - rect.left) / rect.width) * 2 - 1;
            pointer.y = -((event.clientY - rect.top) / rect.height) * 2 + 1;
          };

          const handlePointerDown = (event: PointerEvent) => {
            dragging = true;
            pointerStart = { x: event.clientX, y: event.clientY };
            lastPointer = { ...pointerStart };
            pauseUntil = performance.now() + 2600;
            mount.setPointerCapture?.(event.pointerId);
          };
          const handlePointerMove = (event: PointerEvent) => {
            if (!dragging) return;
            const dx = event.clientX - lastPointer.x;
            const dy = event.clientY - lastPointer.y;
            targetY += dx * 0.009;
            targetX = Math.max(-0.56, Math.min(0.56, targetX + dy * 0.006));
            lastPointer = { x: event.clientX, y: event.clientY };
            pauseUntil = performance.now() + 2600;
          };
          const handlePointerUp = (event: PointerEvent) => {
            const moved = Math.hypot(event.clientX - pointerStart.x, event.clientY - pointerStart.y);
            dragging = false;
            mount.releasePointerCapture?.(event.pointerId);
            if (moved > 7) return;
            setRayPointer(event);
            raycaster.setFromCamera(pointer, camera);
            const match = raycaster.intersectObjects(clickTargets, false)[0];
            const marker = match?.object.userData.marker as GeoGlobeMarker | undefined;
            if (marker) {
              focusMarker(marker);
              return;
            }
            const country = match?.object.userData.country as GeoGlobeCountry | undefined;
            if (country) {
              pauseUntil = performance.now() + 4000;
              onCountrySelect?.(country);
            }
          };
          const handleWheel = (event: WheelEvent) => {
            event.preventDefault();
            targetZoom = Math.max(3.05, Math.min(6.3, targetZoom + event.deltaY * 0.004));
            pauseUntil = performance.now() + 1800;
          };
          const handleKeyDown = (event: KeyboardEvent) => {
            if (event.key === 'ArrowLeft') targetY -= 0.18;
            else if (event.key === 'ArrowRight') targetY += 0.18;
            else if (event.key === 'ArrowUp' || event.key === '+') targetZoom = Math.max(3.05, targetZoom - 0.28);
            else if (event.key === 'ArrowDown' || event.key === '-') targetZoom = Math.min(6.3, targetZoom + 0.28);
            else if (event.key === 'Home') reset();
            else return;
            event.preventDefault();
            pauseUntil = performance.now() + 1600;
          };

          const observer = new ResizeObserver(scheduleResize);
          observer.observe(mount);
          mount.addEventListener('pointerdown', handlePointerDown);
          mount.addEventListener('pointermove', handlePointerMove);
          mount.addEventListener('pointerup', handlePointerUp);
          mount.addEventListener('pointercancel', handlePointerUp);
          mount.addEventListener('wheel', handleWheel, { passive: false });
          mount.addEventListener('keydown', handleKeyDown);
          resize();
          // Recheck after the browser completes flex/grid layout. This makes the
          // canvas centring deterministic after Safari fullscreen/toolbar changes.
          scheduleResize();

          const animate = (now: number) => {
            animationFrame = 0;
            if (disposed || document.hidden) return;
            if (now - lastRenderAt < quality.frameInterval) {
              animationFrame = window.requestAnimationFrame(animate);
              return;
            }
            lastRenderAt = now;
            const elapsed = now * 0.001;
            if (!dragging && !motionPausedRef.current && !reducedMotion && now > pauseUntil) targetY += quality.decoration ? 0.0019 : 0.00125;
            globe.rotation.x += (targetX - globe.rotation.x) * 0.08;
            globe.rotation.y += (targetY - globe.rotation.y) * 0.08;
            currentZoom += (targetZoom - currentZoom) * 0.1;
            camera.position.z = currentZoom;
            camera.lookAt(sceneCenter);
            orbitGroup.rotation.z = elapsed * (quality.decoration ? 0.12 : 0.055);
            orbitGroup.rotation.y = elapsed * (quality.decoration ? 0.05 : 0.025);
            starField.rotation.y = -elapsed * 0.012;
            globe.getWorldPosition(globeWorldPosition);
            countryLabelSprites.forEach((sprite) => {
              sprite.getWorldPosition(labelWorldPosition);
              const facing = labelNormal
                .copy(labelWorldPosition)
                .sub(globeWorldPosition)
                .normalize()
                .dot(cameraDirection.copy(camera.position).sub(labelWorldPosition).normalize());
              sprite.visible = facing > 0.11;
              if (sprite.visible) {
                (sprite.material as import('three').SpriteMaterial).opacity = Math.min(0.94, 0.34 + facing * 0.7);
              }
            });
            markerRoots.forEach(({ marker, halo }) => {
              const pulse = 1 + Math.sin(elapsed * 2.6 + marker.count) * 0.16;
              halo.scale.setScalar(marker.id === selectedId ? pulse * 1.42 : pulse);
              (halo.material as import('three').MeshBasicMaterial).opacity = marker.id === selectedId ? 0.7 : 0.34;
            });
            renderer.render(scene, camera);
            animationFrame = window.requestAnimationFrame(animate);
          };
          const handleVisibilityChange = () => {
            if (document.hidden) {
              if (animationFrame) window.cancelAnimationFrame(animationFrame);
              animationFrame = 0;
              return;
            }
            lastRenderAt = 0;
            if (!animationFrame) animationFrame = window.requestAnimationFrame(animate);
          };
          document.addEventListener('visibilitychange', handleVisibilityChange);
          if (!document.hidden) animationFrame = window.requestAnimationFrame(animate);

          cleanup = () => {
            window.cancelAnimationFrame(animationFrame);
            window.cancelAnimationFrame(resizeFrame);
            document.removeEventListener('visibilitychange', handleVisibilityChange);
            observer.disconnect();
            mount.removeEventListener('pointerdown', handlePointerDown);
            mount.removeEventListener('pointermove', handlePointerMove);
            mount.removeEventListener('pointerup', handlePointerUp);
            mount.removeEventListener('pointercancel', handlePointerUp);
            mount.removeEventListener('wheel', handleWheel);
            mount.removeEventListener('keydown', handleKeyDown);
            controlsRef.current = null;
            scene.traverse((object) => {
              const mesh = object as import('three').Mesh;
              mesh.geometry?.dispose?.();
              const material = mesh.material;
              if (Array.isArray(material)) material.forEach((item) => item.dispose());
              else material?.dispose?.();
            });
            countryLabelTextures.forEach((texture) => texture.dispose());
            markerLabelTextures.forEach((texture) => texture.dispose());
            earthDayTexture.dispose();
            earthNightTexture?.dispose();
            earthBumpTexture?.dispose();
            renderer.dispose();
          };
        } catch (error) {
          if (!disposed) {
            console.error('Unable to initialise the geographic WebGL globe.', error);
            setRenderError(true);
          }
        }
      })
      .catch((error) => {
        if (!disposed) {
          console.error('Unable to load the geographic WebGL globe.', error);
          setRenderError(true);
        }
      });

    return () => {
      disposed = true;
      cleanup?.();
    };
  }, [markerSignature, onCountrySelect, onMarkerSelect, theme, tone, view]);

  const sendControl = (command: GlobeCommand) => controlsRef.current?.(command);

  return (
    <div
      className={cn(
        'geo-globe-shell relative isolate h-[320px] overflow-hidden rounded-[1.35rem] border',
        className
      )}
    >
      <div className="geo-globe-grid pointer-events-none absolute inset-0 opacity-50" />
      <div
        ref={mountRef}
        tabIndex={0}
        aria-label="可交互的地理态势地球。拖拽旋转，滚轮缩放；点击国家名称可进入行政区地图，点击数据点可聚焦。"
        className={cn(
          'absolute inset-0 cursor-grab touch-none outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-inset active:cursor-grabbing',
          renderError && 'pointer-events-none cursor-default'
        )}
      >
        <canvas ref={canvasRef} className="geo-globe-canvas" />
      </div>

      {renderError ? (
        <div className="absolute inset-0 z-10 grid place-items-center px-8 text-center" role="alert">
          <div className="max-w-sm rounded-xl border border-amber-500/35 bg-card/95 px-4 py-3 text-sm leading-6 text-foreground shadow-lg backdrop-blur">
            当前浏览器无法初始化 WebGL 地球。请开启浏览器硬件加速或更新图形驱动后重试。
          </div>
        </div>
      ) : null}

      <div className="geo-globe-control pointer-events-none absolute left-3 top-3 flex items-center gap-2 rounded-full border px-2.5 py-1.5 text-[11px] font-medium text-foreground shadow-sm backdrop-blur">
        <span className="relative flex h-2 w-2">
          <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-400 opacity-75 motion-reduce:hidden" />
          <span className="relative inline-flex h-2 w-2 rounded-full bg-emerald-500" />
        </span>
        {view === 'china' ? '中国区域聚焦' : '全球地理态势'}
      </div>

      {view === 'world' && onCountrySelect ? (
        <label className="geo-globe-control absolute left-3 top-[3.55rem] z-10 flex items-center gap-2 rounded-lg border px-2.5 py-1.5 text-[11px] font-medium text-foreground shadow-sm backdrop-blur">
          <span className="whitespace-nowrap text-muted-foreground">国家下钻</span>
          <select
            aria-label="进入国家行政区地图"
            defaultValue=""
            onChange={(event) => {
              const country = WORLD_COUNTRY_LABELS.find((item) => item.name === event.target.value);
              if (country) onCountrySelect(country);
            }}
            className="max-w-[116px] cursor-pointer appearance-none bg-transparent pr-3 text-[11px] font-medium text-primary outline-none"
          >
            <option value="" disabled>选择国家</option>
            {WORLD_COUNTRY_LABELS.map((country) => <option key={country.name} value={country.name}>{country.name}</option>)}
          </select>
        </label>
      ) : null}

      <div className="geo-globe-control absolute right-3 top-3 flex overflow-hidden rounded-lg border shadow-sm backdrop-blur">
        <button
          type="button"
          aria-label="放大地球"
          onClick={() => sendControl('zoom-in')}
          className="grid h-8 w-8 place-items-center text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
        >
          <Plus className="h-4 w-4" />
        </button>
        <button
          type="button"
          aria-label="缩小地球"
          onClick={() => sendControl('zoom-out')}
          className="grid h-8 w-8 place-items-center border-x border-border text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
        >
          <Minus className="h-4 w-4" />
        </button>
        <button
          type="button"
          aria-label="复位地球视角"
          onClick={() => sendControl('reset')}
          className="grid h-8 w-8 place-items-center text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
        >
          <RotateCcw className="h-3.5 w-3.5" />
        </button>
      </div>

      <button
        type="button"
        onClick={() => setMotionPaused((value) => !value)}
        className="geo-globe-control absolute bottom-3 right-3 inline-flex items-center gap-1.5 rounded-lg border px-2 py-1.5 text-[11px] font-medium text-muted-foreground shadow-sm transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
      >
        {motionPaused ? <Play className="h-3.5 w-3.5" /> : <Pause className="h-3.5 w-3.5" />}
        {motionPaused ? '继续旋转' : '暂停旋转'}
      </button>

      {selected ? (
        <div className="geo-globe-control pointer-events-none absolute bottom-3 left-3 max-w-[min(72%,280px)] rounded-xl border px-3 py-2 shadow-lg backdrop-blur">
          <div className="flex items-center gap-2">
            <span className="grid h-6 w-6 shrink-0 place-items-center rounded-lg bg-primary/10 text-primary">
              <Focus className="h-3.5 w-3.5" />
            </span>
            <div className="min-w-0">
              <p className="truncate text-xs font-semibold text-foreground">{selected.label}</p>
              <p className="text-[11px] text-muted-foreground">已聚焦 · {selected.count} 个去重公网 IP</p>
            </div>
          </div>
        </div>
      ) : markers.length ? (
        <div className="geo-globe-control pointer-events-none absolute bottom-3 left-3 inline-flex items-center gap-1.5 rounded-lg border px-2.5 py-1.5 text-[11px] text-muted-foreground shadow-sm backdrop-blur">
          <MousePointer2 className="h-3.5 w-3.5 text-primary" />
          拖拽旋转 · 滚轮缩放 · 点击点位聚焦
        </div>
      ) : (
        <div className="pointer-events-none absolute inset-0 grid place-items-center px-8 text-center">
          <div className="geo-globe-control rounded-xl border px-4 py-3 text-sm text-muted-foreground shadow-sm backdrop-blur">
            {emptyMessage}
          </div>
        </div>
      )}
      <span className="sr-only" aria-live="polite">
        {selected ? `已聚焦 ${selected.label}，${selected.count} 个去重公网 IP。` : ''}
      </span>
    </div>
  );
}
