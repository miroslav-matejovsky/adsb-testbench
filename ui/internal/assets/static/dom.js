// Safe DOM construction.
//
// Every string becomes a text node or an attribute value set through the DOM
// API. Nothing here assigns innerHTML, so server data can never become markup
// or script.

/**
 * Creates an element. attributes maps names to values: "class" and other
 * names are set with setAttribute, "on*" keys are ignored (listeners are
 * registered through the component lifecycle), and null/false values are
 * skipped. children are nodes, strings or null.
 */
export function h(tag, attributes = {}, ...children) {
  const element = document.createElement(tag);
  for (const [name, value] of Object.entries(attributes)) {
    if (value === null || value === undefined || value === false || name.startsWith("on")) {
      continue;
    }
    element.setAttribute(name, value === true ? "" : String(value));
  }
  append(element, children);
  return element;
}

/** Appends nodes and strings (as text) to parent, skipping null. */
export function append(parent, children) {
  for (const child of children.flat()) {
    if (child === null || child === undefined || child === false) {
      continue;
    }
    parent.append(child instanceof Node ? child : document.createTextNode(String(child)));
  }
  return parent;
}

/** Replaces every child of parent. */
export function replaceChildren(parent, ...children) {
  parent.replaceChildren();
  return append(parent, children);
}

/** Sets the text of node only when it changed, preserving selection. */
export function setText(node, text) {
  const value = String(text);
  if (node.textContent !== value) {
    node.textContent = value;
  }
}

let nextId = 0;

/** Returns a document-unique element ID with the given prefix. */
export function uniqueId(prefix) {
  nextId += 1;
  return `${prefix}-${nextId}`;
}
