import { useEffect, useRef } from 'react';

export function HeroOrbitScene() {
  const mountRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    const mount = mountRef.current;
    if (!mount) return;

    let cleanup: (() => void) | undefined;
    let disposed = false;

    import('three').then((THREE) => {
      if (disposed || !mountRef.current) return;

    const scene = new THREE.Scene();
    const camera = new THREE.PerspectiveCamera(42, 1, 0.1, 100);
    camera.position.set(0, 0.15, 5.6);

    const renderer = new THREE.WebGLRenderer({ alpha: true, antialias: true });
    renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
    renderer.outputColorSpace = THREE.SRGBColorSpace;
    mount.appendChild(renderer.domElement);

    const rootStyles = getComputedStyle(document.documentElement);
    const accent = new THREE.Color(rootStyles.getPropertyValue('--accent').trim() || '#6df3ff');
    const accentTwo = new THREE.Color(rootStyles.getPropertyValue('--accent-2').trim() || '#8ea2ff');
    const accentThree = new THREE.Color(rootStyles.getPropertyValue('--accent-3').trim() || '#7658ff');

    const planet = new THREE.Mesh(
      new THREE.SphereGeometry(1, 72, 72),
      new THREE.MeshPhysicalMaterial({ color: accent, roughness: 0.38, metalness: 0.08, transmission: 0.18, thickness: 0.65, clearcoat: 0.6, clearcoatRoughness: 0.2 })
    );
    scene.add(planet);

    const atmosphere = new THREE.Mesh(
      new THREE.SphereGeometry(1.08, 72, 72),
      new THREE.MeshBasicMaterial({ color: accentTwo, transparent: true, opacity: 0.18, blending: THREE.AdditiveBlending })
    );
    scene.add(atmosphere);

    const orbitGroup = new THREE.Group();
    const orbitMaterial = new THREE.MeshBasicMaterial({ color: accentTwo, transparent: true, opacity: 0.42, side: THREE.DoubleSide });
    const innerOrbit = new THREE.Mesh(new THREE.TorusGeometry(1.9, 0.01, 12, 180), orbitMaterial);
    innerOrbit.rotation.x = Math.PI / 2.8;
    const outerOrbit = new THREE.Mesh(new THREE.TorusGeometry(2.75, 0.006, 12, 220), new THREE.MeshBasicMaterial({ color: accent, transparent: true, opacity: 0.24, side: THREE.DoubleSide }));
    outerOrbit.rotation.x = Math.PI / 2;
    orbitGroup.add(innerOrbit, outerOrbit);
    scene.add(orbitGroup);

    const particleCount = 120;
    const particlePositions = new Float32Array(particleCount * 3);
    for (let index = 0; index < particleCount; index += 1) {
      const radius = 2.5 + Math.random() * 1.45;
      const angle = Math.random() * Math.PI * 2;
      particlePositions[index * 3] = Math.cos(angle) * radius;
      particlePositions[index * 3 + 1] = (Math.random() - 0.5) * 1.9;
      particlePositions[index * 3 + 2] = Math.sin(angle) * radius * 0.72;
    }
    const particleGeometry = new THREE.BufferGeometry();
    particleGeometry.setAttribute('position', new THREE.BufferAttribute(particlePositions, 3));
    const particles = new THREE.Points(particleGeometry, new THREE.PointsMaterial({ color: accentThree, size: 0.025, transparent: true, opacity: 0.52, blending: THREE.AdditiveBlending }));
    scene.add(particles);

    scene.add(new THREE.AmbientLight(0xffffff, 1.1));
    const keyLight = new THREE.PointLight(0xffffff, 4.4, 12);
    keyLight.position.set(-1.4, 1.8, 2.8);
    scene.add(keyLight);
    const rimLight = new THREE.PointLight(accentTwo, 3.2, 10);
    rimLight.position.set(2.8, -1.8, 2.2);
    scene.add(rimLight);

    const pointer = { x: 0, y: 0 };
    const handlePointerMove = (event: PointerEvent) => {
      const rect = mount.getBoundingClientRect();
      pointer.x = ((event.clientX - rect.left) / rect.width - 0.5) * 0.6;
      pointer.y = ((event.clientY - rect.top) / rect.height - 0.5) * 0.4;
    };

    let animationFrame = 0;
    const resize = () => {
      const width = mount.clientWidth || 320;
      const height = mount.clientHeight || 280;
      renderer.setSize(width, height, false);
      camera.aspect = width / height;
      camera.updateProjectionMatrix();
    };
    const animate = () => {
      animationFrame = window.requestAnimationFrame(animate);
      const elapsed = performance.now() * 0.001;
      planet.rotation.y = elapsed * 0.36;
      planet.rotation.x = Math.sin(elapsed * 0.38) * 0.08;
      atmosphere.rotation.y = -elapsed * 0.22;
      orbitGroup.rotation.z = elapsed * 0.16;
      particles.rotation.y = -elapsed * 0.05;
      scene.rotation.x += (pointer.y - scene.rotation.x) * 0.035;
      scene.rotation.y += (pointer.x - scene.rotation.y) * 0.035;
      renderer.render(scene, camera);
    };

    resize();
    animate();
    window.addEventListener('resize', resize);
    mount.addEventListener('pointermove', handlePointerMove);

      cleanup = () => {
      window.cancelAnimationFrame(animationFrame);
      window.removeEventListener('resize', resize);
      mount.removeEventListener('pointermove', handlePointerMove);
      if (renderer.domElement.parentElement === mount) mount.removeChild(renderer.domElement);
      planet.geometry.dispose();
      atmosphere.geometry.dispose();
      innerOrbit.geometry.dispose();
      outerOrbit.geometry.dispose();
      particleGeometry.dispose();
      if (Array.isArray(planet.material)) planet.material.forEach((material) => material.dispose()); else planet.material.dispose();
      if (Array.isArray(atmosphere.material)) atmosphere.material.forEach((material) => material.dispose()); else atmosphere.material.dispose();
      orbitMaterial.dispose();
      outerOrbit.material.dispose();
      particles.material.dispose();
      renderer.dispose();
    };
    });

    return () => {
      disposed = true;
      cleanup?.();
    };
  }, []);

  return <div className="hero-orbit-scene" ref={mountRef} />;
}
