package debug

import (
	"fmt"

	"github.com/go-rod/rod"
)

// AccountDiagSelectors holds the CSS selectors needed for the accounts
// page diagnostic. Caller provides these to avoid import cycles with
// the bbva package.
type AccountDiagSelectors struct {
	AccountRow         string
	AccountCard        string
	AnnouncementModal  string
}

// RunAccountsDiagnostic evaluates diagnostic JavaScript on the page that traces
// the DOM structure to find where deepQuery loses the path to account elements.
// It uses the same walk as FlattenShadowDOM to catalog shadow hosts and their
// children, plus tries deepQuery for specific selectors.
//
// Returns the diagnostic JSON string. The result is also written to disk as
// {operation}-{reason}.json.
func (c *Collector) RunAccountsDiagnostic(page *rod.Page, operation, reason, deepQueryJS string, sels AccountDiagSelectors) string {
	if c == nil {
		return ""
	}

	diagJS := fmt.Sprintf(`() => {
		%s
		const d = {};

		// 1. deepQuery results
		d.dqAccountRow = deepQuery(document, '%s') !== null;
		d.dqAnyTR = deepQuery(document, 'tr') !== null;
		d.dqAnyCard = deepQuery(document, '%s') !== null;
		d.dqModal = deepQuery(document, '%s') !== null;

		// 2. Walk using flattenShadowDOM's approach (childNodes + shadowRoot)
		// and collect all tag names found, plus shadow host paths.
		const tags = {};
		const shadowHosts = [];
		function walk2(node, path, depth) {
			if (depth > 50) return;
			const tag = node.tagName ? node.tagName.toLowerCase() : '#text';
			tags[tag] = (tags[tag] || 0) + 1;

			if (node.shadowRoot) {
				shadowHosts.push(path + '/' + tag + '#shadow');
				// Light DOM children first (like flattenShadowDOM)
				for (const c of Array.from(node.childNodes)) {
					if (c.nodeType === 1) walk2(c, path + '/' + tag + '.light', depth+1);
				}
				// Shadow DOM children
				for (const c of Array.from(node.shadowRoot.childNodes)) {
					if (c.nodeType === 1) walk2(c, path + '/' + tag + '.shadow', depth+1);
				}
			} else {
				for (const c of Array.from(node.childNodes)) {
					if (c.nodeType === 1) walk2(c, path + '/' + tag, depth+1);
				}
			}
		}
		walk2(document.documentElement, '', 0);
		d.shadowHostCount = shadowHosts.length;
		d.tagCounts = tags;

		// 3. Check specific elements we expect
		d.hasTR = (tags['tr'] || 0) > 0;
		d.hasTable = (tags['table'] || 0) > 0;
		d.hasCard = (tags['bbva-btge-card-product-select'] || 0) > 0;
		d.hasAccountsTable = (tags['bbva-btge-accounts-solution-table'] || 0) > 0;

		// 4. Dump first 20 shadow hosts to see the nesting
		d.shadowHostPaths = shadowHosts.slice(0, 20);

		return JSON.stringify(d);
	}`, deepQueryJS, sels.AccountRow, sels.AccountCard, sels.AnnouncementModal)

	diagResult, diagErr := page.Eval(diagJS)
	diagJSON := ""
	if diagErr == nil {
		diagJSON = diagResult.Value.Str()
	} else {
		diagJSON = fmt.Sprintf("eval error: %v", diagErr)
	}

	c.JSON(diagJSON, operation, reason)
	return diagJSON
}
