// Package ui serves the browser user interface of the ADS-B test bench.
//
// # Pages and assets
//
// [NewManager] returns the simulator manager page and [NewAircraftDisplay]
// the received-aircraft page. Each validates its complete configuration,
// renders its page once, and serves it at the handler's relative root for
// GET and HEAD. [Assets] returns the relative handler of the embedded
// browser bundle: native JavaScript modules, styles, the pinned Leaflet 1.9.4
// renderer, a third-party notices page and license texts. No Node runtime,
// CDN, or external font service is used at run time.
//
// A host mounts each handler below its own prefix and strips that prefix
// once, for example:
//
//	mux.Handle("/bench/a/manager/", http.StripPrefix("/bench/a/manager", manager))
//	mux.Handle("/bench/a/assets/", http.StripPrefix("/bench/a/assets", ui.Assets()))
//
// # Configuration
//
// [ManagerConfig] and [AircraftDisplayConfig] are complete and explicit; no
// constructor or parser supplies a missing value. APIBaseURL and
// AssetBaseURL are root-relative paths or absolute http(s) URLs that the
// browser must resolve to the page's own origin. The manager's API base
// addresses the simulator API; the aircraft display's API base addresses the
// display backend, never the simulator. [ParseManagerSettings] and
// [ParseAircraftDisplaySettings] strictly parse the serializable settings
// without URL bases, which a composing application derives from its own
// mount prefix.
//
// A page renders its configuration as JSON inside a
// <script type="application/json"> element escaped by html/template; it is
// never interpolated into executable script. Pages send a restrictive
// Content-Security-Policy that allows only same-origin scripts, styles and
// requests, plus images from a configured tile origin.
//
// # Browser components
//
// Hosts that build their own layout import "<asset base>components.js" and
// call mountManager(root, config) or mountAircraftDisplay(root, config) with
// the same JSON configuration the pages render. A mount validates the
// configuration before touching the DOM or sending a request, owns only its
// root element, requests, timers, listeners and map, and returns a handle
// whose destroy() is idempotent: it aborts in-flight requests, stops
// polling, removes listeners and the map, and releases the root.
//
// # Manager
//
// The manager reads GET metadata and GET truth from the simulator API and
// enables controls only when both name the same run. Controls send absolute
// assignments with the observed run ID: aircraft count (0 removes every
// aircraft) and speed in hundredths of real time (100 is real time, 0
// pauses). Pause sends 0; Resume sends the last positive speed confirmed in
// the current run, or the configured ResumeSpeedHundredths when none was
// observed. Drafts are kept on validation, network and server errors; a
// timed-out command has an unknown outcome and is never replayed. A detected
// restart clears the previous run's state and requires review of dirty
// drafts before they are submitted to the new run. The "Generated frames"
// panel shows the bounded transmission history exactly as the API returns
// it, separate from received evidence.
//
// The station editor creates, replaces, enables/disables and removes
// stations with complete settings: ID, reception, latitude and longitude
// (degrees), site elevation and antenna height (metres), antenna gain (dBi),
// sensitivity (dBm), system loss (dB) and frame-loss probability (0..1). A
// new form starts with empty inputs; zero and "disabled" are explicit values.
// Replacement and removal carry the revision the draft was written against.
// A revision conflict, a server-side change, a removal by another client or
// a replaced run keeps the draft, shows the server values beside it, and
// requires "Reapply draft to current revision" plus a new submit; nothing is
// retried or rebased automatically. Station IDs cannot be reused within one
// run. Coverage values are labeled as synthetic reception-model estimates.
//
// # Aircraft display
//
// The aircraft display reads GET stations and POST observations from the
// display backend, starting with the exact configured StationIDs (an empty
// selection is explicit). It renders its own refresh responses and never
// the backend's shared GET snapshot. Every received field is shown with its
// exact virtual age (snapshot now minus observedAt); a field the backend
// omitted is "unavailable", never zero. Track status applies FreshFor and
// LostAfter to the age of the last received frame: fresh at age <= FreshFor,
// stale up to LostAfter, lost beyond. An aircraft that disappears between
// two successful snapshots of one selection is listed once as "No retained
// evidence" without any old measurement. A failed refresh keeps only that
// selection's last successful view, labeled transport-stale with its real
// update age; virtual ages do not move. A selection change or a confirmed
// replacement run discards incompatible data; a selected station missing
// from the catalog is flagged and never replaced automatically.
//
// The map uses the embedded Leaflet renderer and keeps the user's viewport:
// data never re-centers it, only "Fit received aircraft" does. Markers exist
// only for valid received positions of tracks that are not lost; direction
// is drawn only from a received ground track. Each selected enabled station
// gets a circle of its published effective coverage radius (1 NM = 1852 m),
// labeled with the reference pressure altitude as a synthetic estimate.
// With Tiles nil the map requests no tile; tile failures never affect the
// table, selector or inspector. Positions beyond the Web Mercator latitude
// limit are drawn clipped and flagged, and their exact coordinates stay in
// the table.
//
// The reception inspector shows the selected aircraft's field evidence
// exactly as received: both CPR frames of a position, every receiver copy,
// and the station revision and settings recorded at reception time. Station
// history is paged with the returned cursor, unchanged, while hasMore is
// true. Source retention gaps stay as persistent rows; the browser keeps at
// most MaxHistoryRecords loaded rows and labels its own trimming separately.
// A cursor conflict or a replacement run requires an explicit reset, and a
// removed station's loaded rows stay visible as historical. Frames can be
// copied; clipboard failures are reported.
//
// Components poll with one outstanding cycle at a time and mark their
// wrapper aria-busy="true" while a cycle runs. Revisions,
// sequences and nanosecond values stay exact as strings or BigInt. Real
// update age and virtual simulation time are always labeled separately.
package ui
