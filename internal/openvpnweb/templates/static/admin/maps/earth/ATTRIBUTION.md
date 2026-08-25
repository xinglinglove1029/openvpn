# Earth texture attribution

The bundled `earth_*_1024.jpg` files are locally resized 1024 × 512 derivatives of the public Earth texture set from the Three.js examples collection:

- `earth_day_1024.jpg`
- `earth_night_1024.jpg`
- `earth_bump_roughness_clouds_1024.jpg`

Source: Three.js examples (`examples/textures/planets`) / NASA Earth imagery. The lower-resolution derivatives are shipped locally so the dashboard continues rendering when it cannot reach third-party hosts while avoiding unnecessary 4K GPU memory use. The globe implementation uses Three.js under its MIT License.

GlobeStream3D (`hululuuuuu/GlobeStream3D`, MIT) was evaluated as a reference for its Three.js component architecture. This project keeps its own React integration and administrative-map drill-down flow so online IP details, permissions, and the existing province/city/district interaction remain connected.