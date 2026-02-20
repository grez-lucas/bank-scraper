// Package browser provides utilities for browser automation with Rod.
package browser

import (
	"encoding/json"
	"fmt"

	"github.com/go-rod/rod"
)

// flattenShadowDOMJS is a JavaScript function that recursively walks the DOM,
// inlining shadow DOM content and iframe documents into a single parseable
// HTML document.
//
// BBVA's 2026 redesign uses Web Components (Polymer/Cells framework) with
// deeply nested shadow roots. A plain page.HTML() only returns empty custom
// element shells — the actual data (accounts, balances, transactions) is
// hidden behind multiple layers of shadow roots with iframes interleaved.
//
// This script walks every element depth-first. For elements with a shadowRoot,
// it wraps the shadow content in a <div data-shadow-root="true">. For iframes,
// it accesses contentDocument and inlines the content in a
// <div data-captured-iframe="true">.
//
// Critical ordering: iframes inside shadow roots must be inlined BEFORE the
// parent shadow root is serialized. Once shadow content is read as innerHTML,
// live iframe contentDocument references become dead. The depth-first,
// bottom-up approach ensures this.
const flattenShadowDOMJS = `() => {
	const MAX_DEPTH = 100;
	let shadowCount = 0;
	let iframeCount = 0;

	function flattenNode(node, depth) {
		if (depth > MAX_DEPTH) return;

		// Process child nodes first (depth-first)
		const children = Array.from(node.childNodes);
		for (const child of children) {
			if (child.nodeType === Node.ELEMENT_NODE) {
				flattenElement(child, depth);
			}
		}
	}

	function flattenElement(el, depth) {
		if (depth > MAX_DEPTH) return;

		// Handle iframes — inline their content document
		if (el.tagName === 'IFRAME') {
			inlineIframe(el, depth);
			return;
		}

		// If the element has a shadow root, process it
		if (el.shadowRoot) {
			flattenShadow(el, depth);
			return;
		}

		// Regular element — recurse into children
		flattenNode(el, depth + 1);
	}

	function flattenShadow(host, depth) {
		const shadow = host.shadowRoot;
		if (!shadow) return;

		// First: recurse into shadow children (depth-first, bottom-up)
		// This ensures iframes inside the shadow root are inlined before
		// we serialize the shadow content.
		const shadowChildren = Array.from(shadow.childNodes);
		for (const child of shadowChildren) {
			if (child.nodeType === Node.ELEMENT_NODE) {
				flattenElement(child, depth + 1);
			}
		}

		// Now serialize the (already-flattened) shadow content
		const container = document.createElement('div');
		container.setAttribute('data-shadow-root', 'true');
		container.setAttribute('data-shadow-host', host.tagName.toLowerCase());

		// Copy shadow DOM styles
		shadow.querySelectorAll('style').forEach(function(style) {
			const s = document.createElement('style');
			s.setAttribute('data-from-shadow', 'true');
			s.textContent = style.textContent;
			container.appendChild(s);
		});

		// Copy shadow DOM content (excluding <style> tags already copied)
		for (const child of Array.from(shadow.childNodes)) {
			if (child.nodeType === Node.ELEMENT_NODE && child.tagName === 'STYLE') {
				continue;
			}
			try {
				container.appendChild(child.cloneNode(true));
			} catch(e) {
				// Skip nodes that can't be cloned
			}
		}

		shadowCount++;

		// Append flattened shadow content after the host element's light DOM children
		host.appendChild(container);
	}

	function inlineIframe(iframe, depth) {
		try {
			const iframeDoc = iframe.contentDocument || (iframe.contentWindow && iframe.contentWindow.document);
			if (!iframeDoc || !iframeDoc.documentElement) {
				markIframeError(iframe, 'no contentDocument available');
				return;
			}

			// Recurse into the iframe document first (depth-first)
			flattenNode(iframeDoc.documentElement, depth + 1);

			// Now serialize the (already-flattened) iframe content
			const container = iframe.ownerDocument.createElement('div');
			container.setAttribute('data-captured-iframe', 'true');
			container.setAttribute('data-iframe-src', iframe.src || '');
			container.setAttribute('data-iframe-id', iframe.id || '');
			container.setAttribute('data-iframe-name', iframe.name || '');

			// Copy styles from iframe head
			if (iframeDoc.head) {
				iframeDoc.head.querySelectorAll('style').forEach(function(style) {
					const s = iframe.ownerDocument.createElement('style');
					s.setAttribute('data-from-iframe', 'true');
					s.textContent = style.textContent;
					container.appendChild(s);
				});
			}

			// Copy iframe body content
			if (iframeDoc.body) {
				container.innerHTML += iframeDoc.body.innerHTML;
			}

			iframeCount++;
			iframe.parentNode.replaceChild(container, iframe);
		} catch(e) {
			markIframeError(iframe, e.message);
		}
	}

	function markIframeError(iframe, message) {
		try {
			const errDiv = iframe.ownerDocument.createElement('div');
			errDiv.setAttribute('data-captured-iframe', 'true');
			errDiv.setAttribute('data-iframe-error', message);
			errDiv.setAttribute('data-iframe-src', iframe.src || '');
			errDiv.textContent = '[iframe not accessible: ' + message + ']';
			iframe.parentNode.replaceChild(errDiv, iframe);
		} catch(e) {
			// Can't even create error marker — skip silently
		}
	}

	// Start flattening from the document root
	flattenNode(document.documentElement, 0);

	return JSON.stringify({
		html: document.documentElement.outerHTML,
		shadowCount: shadowCount,
		iframeCount: iframeCount
	});
}`

// flattenResult holds the parsed JSON response from the JS flattener.
type flattenResult struct {
	HTML        string `json:"html"`
	ShadowCount int    `json:"shadowCount"`
	IframeCount int    `json:"iframeCount"`
}

// FlattenShadowDOM executes JavaScript on the page to recursively inline all
// shadow DOM content and iframe documents into a single HTML string.
//
// This is necessary for BBVA's 2026 post-login pages which use Web Components
// (Polymer/Cells) with deeply nested shadow roots. A plain page.HTML() only
// returns empty custom element shells.
//
// The function modifies the live DOM by appending flattened shadow content and
// replacing iframes with div containers. This is acceptable for fixture capture
// since the user navigates to a new page between each capture step.
//
// Returns the merged HTML string, counts of shadow roots and iframes flattened,
// and any error. On JS eval failure, falls back to plain page.HTML().
func FlattenShadowDOM(page *rod.Page) (html string, shadowCount int, iframeCount int, err error) {
	res, evalErr := page.Eval(flattenShadowDOMJS)
	if evalErr != nil {
		// Fallback: return plain HTML if JS eval fails
		html, err = page.HTML()
		if err != nil {
			return "", 0, 0, fmt.Errorf("flatten shadow DOM eval failed and fallback HTML failed: %w", err)
		}
		return html, 0, 0, nil
	}

	var result flattenResult
	if err := json.Unmarshal([]byte(res.Value.Str()), &result); err != nil {
		// Fallback: return plain HTML if JSON parse fails
		html, htmlErr := page.HTML()
		if htmlErr != nil {
			return "", 0, 0, fmt.Errorf("flatten shadow DOM JSON parse failed and fallback HTML failed: %w", htmlErr)
		}
		return html, 0, 0, nil
	}

	return result.HTML, result.ShadowCount, result.IframeCount, nil
}
