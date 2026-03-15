package browser

import (
	"fmt"

	"github.com/go-rod/rod"
)

// DeepQueryJS is a reusable JS helper that walks the DOM depth-first,
// crossing shadow DOM, light DOM, and iframe boundaries to find elements
// by CSS selector. This mirrors the full traversal logic of FlattenShadowDOM.
//
// Three boundaries are crossed:
//  1. Shadow DOM  — node.shadowRoot.children
//  2. Light DOM   — node.children (slotted content in Polymer/Cells)
//  3. Iframes     — iframe.contentDocument (same-origin only)
//
// When root is a document, traversal starts from documentElement.
// When root is an element, walk() starts from that element directly.
const DeepQueryJS = `
function deepQuery(root, selector) {
	function walk(node) {
		if (node.matches && node.matches(selector)) return node;
		// Cross iframe boundary into its document
		if (node.tagName === 'IFRAME') {
			try {
				var doc = node.contentDocument || (node.contentWindow && node.contentWindow.document);
				if (doc && doc.documentElement) {
					var found = walk(doc.documentElement);
					if (found) return found;
				}
			} catch(e) {} // cross-origin — skip
			return null;
		}
		// Cross shadow DOM boundary
		if (node.shadowRoot) {
			for (var i = 0; i < node.shadowRoot.children.length; i++) {
				var found = walk(node.shadowRoot.children[i]);
				if (found) return found;
			}
		}
		// Walk light DOM / regular children
		for (var i = 0; i < node.children.length; i++) {
			var found = walk(node.children[i]);
			if (found) return found;
		}
		return null;
	}
	if (root.documentElement) return walk(root.documentElement);
	return walk(root);
}
`

// DeepQueryAllJS defines a JavaScript function that finds ALL elements matching
// a CSS selector, recursively crossing shadow DOM, light DOM, and iframe boundaries.
// Unlike deepQuery (first-match), this collects every match.
const DeepQueryAllJS = `
function deepQueryAll(root, selector) {
	const results = [];
	function walk(node) {
		if (node.nodeType !== 1 && node !== document) return;
		// Check if this element matches
		if (node !== root && node.matches && node.matches(selector)) {
			results.push(node);
		}
		// Cross iframe boundary into its document
		if (node.tagName === 'IFRAME') {
			try {
				var doc = node.contentDocument || (node.contentWindow && node.contentWindow.document);
				if (doc && doc.documentElement) {
					walk(doc.documentElement);
				}
			} catch(e) {} // cross-origin — skip
			return;
		}
		// Walk shadow DOM children
		if (node.shadowRoot) {
			for (const child of node.shadowRoot.childNodes) {
				walk(child);
			}
		}
		// Walk light DOM children
		for (const child of node.childNodes) {
			walk(child);
		}
	}
	walk(root);
	return results;
}
`

// DeepQueryExists returns true if an element matching the CSS selector exists
// anywhere in the page's DOM, including inside shadow roots.
func DeepQueryExists(page *rod.Page, selector string) bool {
	js := fmt.Sprintf(`() => {
		%s
		return deepQuery(document, '%s') !== null;
	}`, DeepQueryJS, selector)

	result, err := page.Eval(js)
	if err != nil {
		return false
	}
	return result.Value.Bool()
}

// DeepQueryClick finds an element matching the CSS selector through shadow DOM
// boundaries and clicks it. Returns true if the element was found and clicked.
func DeepQueryClick(page *rod.Page, selector string) bool {
	js := fmt.Sprintf(`() => {
		%s
		const el = deepQuery(document, '%s');
		if (!el) return false;
		el.click();
		return true;
	}`, DeepQueryJS, selector)

	result, err := page.Eval(js)
	if err != nil {
		return false
	}
	return result.Value.Bool()
}

// DeepQueryAttr returns the value of an attribute on the first element matching
// the CSS selector, searching through shadow roots. Returns empty string if the
// element or attribute is not found.
func DeepQueryAttr(page *rod.Page, selector string, attr string) string {
	js := fmt.Sprintf(`() => {
		%s
		const el = deepQuery(document, '%s');
		if (!el) return '';
		return el.getAttribute('%s') || '';
	}`, DeepQueryJS, selector, attr)

	result, err := page.Eval(js)
	if err != nil {
		return ""
	}
	return result.Value.Str()
}

// DeepQueryHTML finds an element via deepQuery, flattens its shadow DOM
// subtree in-place, then returns its outerHTML. This is much cheaper than
// flattening the entire page because only the target subtree is processed.
//
// The shadow flattening is necessary because outerHTML only serializes light
// DOM — content inside shadow roots (e.g., <tbody> rows in a web component
// table) would be invisible without inlining.
//
// WARNING: This mutates the live DOM of the matched element's subtree.
func DeepQueryHTML(page *rod.Page, selector string) (string, error) {
	js := fmt.Sprintf(`() => {
		%s
		const el = deepQuery(document, '%s');
		if (!el) return '';

		// Flatten shadow DOM within this subtree only
		function flattenSubtree(node) {
			if (!node || node.nodeType !== 1) return;
			// Process children first (depth-first, bottom-up)
			if (node.shadowRoot) {
				// Flatten shadow children first
				for (const child of Array.from(node.shadowRoot.childNodes)) {
					if (child.nodeType === 1) flattenSubtree(child);
				}
				// Flatten light DOM children (slotted content)
				for (const child of Array.from(node.childNodes)) {
					if (child.nodeType === 1) flattenSubtree(child);
				}
				// Inline shadow content into light DOM
				for (const child of Array.from(node.shadowRoot.childNodes)) {
					if (child.nodeType === 1 && child.tagName === 'STYLE') continue;
					try { node.appendChild(child.cloneNode(true)); } catch(e) {}
				}
			} else {
				for (const child of Array.from(node.childNodes)) {
					if (child.nodeType === 1) flattenSubtree(child);
				}
			}
		}
		flattenSubtree(el);
		return el.outerHTML;
	}`, DeepQueryJS, selector)

	result, err := page.Eval(js)
	if err != nil {
		return "", fmt.Errorf("deepQueryHTML eval: %w", err)
	}
	return result.Value.Str(), nil
}

// DeepQueryCountAll returns the number of elements matching selector across
// all shadow boundaries. Returns 0 on any error — non-blocking.
func DeepQueryCountAll(page *rod.Page, selector string) int {
	res, err := page.Eval(fmt.Sprintf(`() => {
		%s
		return deepQueryAll(document, '%s').length;
	}`, DeepQueryAllJS, selector))
	if err != nil {
		return 0
	}
	return res.Value.Int()
}
