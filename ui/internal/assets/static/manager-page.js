// Manager page bootstrap: reads the JSON configuration rendered by the Go
// template and mounts one manager component into the labeled root.
import { mountManager } from "./components.js";

const root = document.querySelector('[data-tb-mount="manager"]');
const config = JSON.parse(document.getElementById("tb-config").textContent);
try {
  mountManager(root, config);
} catch (error) {
  root.replaceChildren(document.createTextNode(`The manager could not start: ${error.message}`));
  root.setAttribute("role", "alert");
}
