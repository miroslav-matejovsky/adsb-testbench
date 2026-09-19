# Page templates

`html/template` documents rendered once by `ui.NewManager` and
`ui.NewAircraftDisplay` when a page handler is constructed.

| File | Page |
| --- | --- |
| `manager.html` | Simulator manager page with one manager component root |
| `aircraft.html` | Aircraft display page with one aircraft display component root |

Templates receive the asset base and the component configuration. The
configuration is rendered as JSON inside a `<script type="application/json">`
element, which `html/template` escapes for that context; it is never
interpolated into executable script. The page module reads that element and
mounts the component into the labeled root.
