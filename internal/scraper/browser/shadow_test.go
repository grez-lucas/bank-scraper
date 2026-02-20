package browser

import (
	"strings"
	"testing"

	"github.com/go-rod/rod"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupPage creates a Rod browser and page for testing. The browser connects
// to a headless Chromium instance. The page is closed via t.Cleanup.
func setupPage(t *testing.T) *rod.Page {
	t.Helper()

	browser := rod.New().MustConnect()
	t.Cleanup(func() { browser.MustClose() })

	page := browser.MustPage()
	t.Cleanup(func() { page.MustClose() })

	return page
}

func TestFlattenShadowDOM_NoShadow(t *testing.T) {
	page := setupPage(t)
	page.MustNavigate("about:blank").MustWaitLoad()
	page.MustEval(`() => {
		document.body.innerHTML = '<div id="plain"><p>Hello World</p></div>';
	}`)

	html, shadowCount, iframeCount, err := FlattenShadowDOM(page)

	require.NoError(t, err)
	assert.Equal(t, 0, shadowCount)
	assert.Equal(t, 0, iframeCount)
	assert.Contains(t, html, "Hello World")
	assert.Contains(t, html, `id="plain"`)
}

func TestFlattenShadowDOM_SingleShadow(t *testing.T) {
	page := setupPage(t)
	page.MustNavigate("about:blank").MustWaitLoad()
	page.MustEval(`() => {
		document.body.innerHTML = '<my-component></my-component>';
		const host = document.querySelector('my-component');
		const shadow = host.attachShadow({mode: 'open'});
		shadow.innerHTML = '<div class="shadow-content">Shadow Text</div>';
	}`)

	html, shadowCount, _, err := FlattenShadowDOM(page)

	require.NoError(t, err)
	assert.Equal(t, 1, shadowCount)
	assert.Contains(t, html, `data-shadow-root="true"`)
	assert.Contains(t, html, `data-shadow-host="my-component"`)
	assert.Contains(t, html, "Shadow Text")
}

func TestFlattenShadowDOM_NestedShadow(t *testing.T) {
	page := setupPage(t)
	page.MustNavigate("about:blank").MustWaitLoad()
	page.MustEval(`() => {
		document.body.innerHTML = '<outer-el></outer-el>';

		const outer = document.querySelector('outer-el');
		const outerShadow = outer.attachShadow({mode: 'open'});
		outerShadow.innerHTML = '<inner-el></inner-el>';

		const inner = outerShadow.querySelector('inner-el');
		const innerShadow = inner.attachShadow({mode: 'open'});
		innerShadow.innerHTML = '<span class="deep">Nested Content</span>';
	}`)

	html, shadowCount, _, err := FlattenShadowDOM(page)

	require.NoError(t, err)
	assert.Equal(t, 2, shadowCount)
	assert.Contains(t, html, `data-shadow-host="outer-el"`)
	assert.Contains(t, html, `data-shadow-host="inner-el"`)
	assert.Contains(t, html, "Nested Content")
}

func TestFlattenShadowDOM_LightDOMChildren(t *testing.T) {
	// Regression test: shadow host has light DOM children that are themselves
	// shadow hosts (Polymer/Cells pattern with <slot> projection). Before the
	// fix, flattenShadow only recursed into shadow.childNodes and skipped
	// host.childNodes, so the inner shadow content was lost.
	page := setupPage(t)
	page.MustNavigate("about:blank").MustWaitLoad()
	page.MustEval(`() => {
		document.body.innerHTML = '<outer-host><inner-host>Light text</inner-host></outer-host>';

		// Outer host: shadow with a <slot> that projects light DOM children
		const outer = document.querySelector('outer-host');
		outer.attachShadow({mode: 'open'}).innerHTML =
			'<div class="outer-layout"><slot></slot></div>';

		// Inner host (light DOM child of outer): has its own shadow with data
		const inner = document.querySelector('inner-host');
		inner.attachShadow({mode: 'open'}).innerHTML =
			'<table id="data-table"><tr><td>secret-data</td></tr></table>';
	}`)

	html, shadowCount, _, err := FlattenShadowDOM(page)

	require.NoError(t, err)
	assert.GreaterOrEqual(t, shadowCount, 2, "both shadow roots should be counted")
	assert.Contains(t, html, `data-shadow-host="inner-host"`,
		"inner shadow host should be flattened")
	assert.Contains(t, html, "secret-data",
		"data inside inner shadow root should be present")
	assert.Contains(t, html, "data-table",
		"selectors inside inner shadow root should be reachable")
}

func TestFlattenShadowDOM_ShadowStyles(t *testing.T) {
	page := setupPage(t)
	page.MustNavigate("about:blank").MustWaitLoad()
	page.MustEval(`() => {
		document.body.innerHTML = '<styled-el></styled-el>';
		const host = document.querySelector('styled-el');
		const shadow = host.attachShadow({mode: 'open'});
		shadow.innerHTML = '<style>.inner { color: red; }</style><div class="inner">Styled</div>';
	}`)

	html, shadowCount, _, err := FlattenShadowDOM(page)

	require.NoError(t, err)
	assert.Equal(t, 1, shadowCount)

	// Style should be copied with the data-from-shadow marker
	assert.True(t, strings.Contains(html, `data-from-shadow="true"`),
		"shadow styles should have data-from-shadow attribute")
	assert.Contains(t, html, "color: red")
	assert.Contains(t, html, "Styled")
}
