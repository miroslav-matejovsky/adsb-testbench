# UI internals

`assets` embeds the browser bundle served by package `ui`: page templates,
native JavaScript modules and styles, the pinned Leaflet renderer, and
third-party license texts. Its contract is documented in `assets/doc.go`, and
each embedded folder has its own `README.md`.
