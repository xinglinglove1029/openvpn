import { useEffect, useRef } from 'react';
import type * as THREE from 'three';
import { useIsMobile } from '@/hooks/useIsMobile';

export function BackgroundScene() {
  const mountRef = useRef<HTMLDivElement | null>(null);
  const isMobile = useIsMobile();

  useEffect(() => {
    if (isMobile) return;

    const mount = mountRef.current;
    if (!mount) return;

    let cleanup: (() => void) | undefined;
    let disposed = false;

    import('three').then((THREE) => {
      if (disposed || !mountRef.current) return;

      const rootStyles = getComputedStyle(document.documentElement);
      const accent = new THREE.Color(rootStyles.getPropertyValue('--accent').trim() || '#6df3ff');
      const accentTwo = new THREE.Color(rootStyles.getPropertyValue('--accent-2').trim() || '#8ea2ff');

      const scene = new THREE.Scene();

      const camera = new THREE.PerspectiveCamera(55, 1, 0.1, 200);
      camera.position.set(0, 0.6, 9);
      camera.lookAt(0, 0, 0);

      const renderer = new THREE.WebGLRenderer({ alpha: true, antialias: true });
      renderer.setPixelRatio(Math.min(window.devicePixelRatio, 1.5));
      renderer.outputColorSpace = THREE.SRGBColorSpace;
      mount.appendChild(renderer.domElement);

      // ---- 粒子：分两层，远近错落
      function buildPoints(count: number, spread: number, size: number, color: THREE.Color, opacity: number) {
        const positions = new Float32Array(count * 3);
        for (let i = 0; i < count; i += 1) {
          positions[i * 3] = (Math.random() - 0.5) * spread;
          positions[i * 3 + 1] = (Math.random() - 0.5) * spread * 0.55;
          positions[i * 3 + 2] = (Math.random() - 0.5) * spread;
        }
        const geometry = new THREE.BufferGeometry();
        geometry.setAttribute('position', new THREE.BufferAttribute(positions, 3));
        const material = new THREE.PointsMaterial({
          color,
          size,
          transparent: true,
          opacity,
          blending: THREE.AdditiveBlending,
          depthWrite: false,
        });
        return { points: new THREE.Points(geometry, material), geometry, material };
      }

      const farLayer = buildPoints(220, 18, 0.04, accent, 0.55);
      const nearLayer = buildPoints(110, 12, 0.07, accentTwo, 0.7);
      scene.add(farLayer.points);
      scene.add(nearLayer.points);

      // ---- 透视线条网格（地面）
      const grid = new THREE.GridHelper(40, 36, accent, accentTwo);
      (grid.material as THREE.LineBasicMaterial).transparent = true;
      (grid.material as THREE.LineBasicMaterial).opacity = 0.18;
      grid.position.y = -2.2;
      scene.add(grid);

      // ---- 漂浮几何体（点缀）
      const ring = new THREE.Mesh(
        new THREE.TorusGeometry(1.2, 0.01, 10, 120),
        new THREE.MeshBasicMaterial({ color: accent, transparent: true, opacity: 0.45 }),
      );
      ring.position.set(-2.6, 0.4, -1.2);
      const ring2 = new THREE.Mesh(
        new THREE.TorusGeometry(0.9, 0.01, 10, 120),
        new THREE.MeshBasicMaterial({ color: accentTwo, transparent: true, opacity: 0.4 }),
      );
      ring2.position.set(2.8, -0.4, -1.6);
      const ico = new THREE.Mesh(
        new THREE.IcosahedronGeometry(0.45, 0),
        new THREE.MeshBasicMaterial({ color: accent, wireframe: true, transparent: true, opacity: 0.55 }),
      );
      ico.position.set(1.6, 0.9, -0.6);
      const ico2 = new THREE.Mesh(
        new THREE.OctahedronGeometry(0.32, 0),
        new THREE.MeshBasicMaterial({ color: accentTwo, wireframe: true, transparent: true, opacity: 0.55 }),
      );
      ico2.position.set(-1.4, -0.6, -0.4);
      scene.add(ring, ring2, ico, ico2);

      // ---- 灯光（仅影响 wireframe 颜色，PointLight 让 AdditiveBlending 更柔和）
      scene.add(new THREE.AmbientLight(0xffffff, 1.2));
      const pl = new THREE.PointLight(accentTwo, 1.2, 12);
      pl.position.set(2, 2, 2);
      scene.add(pl);

      // ---- 鼠标视差
      const pointer = { x: 0, y: 0 };
      const handlePointerMove = (event: PointerEvent) => {
        pointer.x = (event.clientX / window.innerWidth - 0.5) * 2;
        pointer.y = (event.clientY / window.innerHeight - 0.5) * 2;
      };

      let animationFrame = 0;
      const resize = () => {
        const width = mount.clientWidth || window.innerWidth;
        const height = mount.clientHeight || window.innerHeight;
        renderer.setSize(width, height, false);
        camera.aspect = width / height;
        camera.updateProjectionMatrix();
      };
      const animate = () => {
        animationFrame = window.requestAnimationFrame(animate);
        const elapsed = performance.now() * 0.001;

        // 远层粒子缓慢顺时针
        farLayer.points.rotation.y = elapsed * 0.04;
        farLayer.points.rotation.x = Math.sin(elapsed * 0.18) * 0.04;
        // 近层粒子逆时针
        nearLayer.points.rotation.y = -elapsed * 0.07;
        nearLayer.points.rotation.z = Math.cos(elapsed * 0.22) * 0.05;

        // 几何体自转 + 浮动
        ring.rotation.z = elapsed * 0.35;
        ring.rotation.x = Math.PI / 2 + Math.sin(elapsed * 0.3) * 0.12;
        ring.position.y = 0.4 + Math.sin(elapsed * 0.45) * 0.08;

        ring2.rotation.z = -elapsed * 0.4;
        ring2.position.y = -0.4 + Math.cos(elapsed * 0.4) * 0.1;

        ico.rotation.x = elapsed * 0.5;
        ico.rotation.y = elapsed * 0.35;
        ico.position.y = 0.9 + Math.sin(elapsed * 0.55) * 0.12;

        ico2.rotation.x = -elapsed * 0.4;
        ico2.rotation.y = elapsed * 0.6;
        ico2.position.y = -0.6 + Math.cos(elapsed * 0.6) * 0.1;

        // 网格微微晃动
        grid.position.z = (elapsed * 0.18) % 1.1 - 0.55;

        // 视差：相机轻微跟随鼠标
        const targetX = pointer.x * 0.45;
        const targetY = -pointer.y * 0.3;
        camera.position.x += (targetX - camera.position.x) * 0.04;
        camera.position.y += (targetY - camera.position.y) * 0.04;
        camera.lookAt(0, 0, 0);

        renderer.render(scene, camera);
      };

      resize();
      animate();
      window.addEventListener('resize', resize);
      window.addEventListener('pointermove', handlePointerMove);

      cleanup = () => {
        window.cancelAnimationFrame(animationFrame);
        window.removeEventListener('resize', resize);
        window.removeEventListener('pointermove', handlePointerMove);
        if (renderer.domElement.parentElement === mount) mount.removeChild(renderer.domElement);

        [farLayer, nearLayer].forEach((layer) => {
          layer.geometry.dispose();
          layer.material.dispose();
        });
        (grid.geometry as THREE.BufferGeometry).dispose();
        (grid.material as THREE.Material).dispose();
        [ring, ring2, ico, ico2].forEach((mesh) => {
          mesh.geometry.dispose();
          if (Array.isArray(mesh.material)) mesh.material.forEach((m) => m.dispose());
          else mesh.material.dispose();
        });
        renderer.dispose();
      };
    });

    return () => {
      disposed = true;
      cleanup?.();
    };
  }, [isMobile]);

  if (isMobile) {
    return (
      <div
        className="app-bg-scene"
        aria-hidden="true"
        style={{
          background:
            'radial-gradient(ellipse at 20% 20%, color-mix(in srgb, var(--accent) 18%, transparent) 0%, transparent 55%), radial-gradient(ellipse at 80% 80%, color-mix(in srgb, var(--accent-2, var(--accent)) 14%, transparent) 0%, transparent 55%)',
        }}
      />
    );
  }

  return <div className="app-bg-scene" ref={mountRef} aria-hidden="true" />;
}
