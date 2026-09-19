// Package assets embeds the browser bundle served by package ui.
//
// The bundle has three parts:
//
//   - templates/: html/template page documents, read with Template.
//   - static/: native JavaScript modules, styles, the notices page, and the
//     pinned Leaflet renderer under static/leaflet/. They are served at the
//     root of the asset base.
//   - licenses/: third-party license texts, served below licenses/.
//
// Only an explicit allowlist of file patterns is embedded. Lookup serves
// exactly those files by their path relative to the asset base. Folder documentation is not embedded. No Node runtime, build
// step, or network access is needed at run time.
package assets
