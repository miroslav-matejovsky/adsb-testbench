// Aircraft display page bootstrap: reads the JSON configuration rendered by
// the Go template and mounts one aircraft display component.
import { mountAircraftDisplay } from "./components.js";

const root = document.querySelector('[data-tb-mount="aircraft"]');
const config = JSON.parse(document.getElementById("tb-config").textContent);
try {
  mountAircraftDisplay(root, config);
} catch (error) {
  root.replaceChildren(document.createTextNode(`The aircraft display could not start: ${error.message}`));
  root.setAttribute("role", "alert");
}
