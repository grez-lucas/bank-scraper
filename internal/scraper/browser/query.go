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
// a CSS selector, recursively crossing shadow DOM boundaries.
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
