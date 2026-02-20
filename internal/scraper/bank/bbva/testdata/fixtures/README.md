# Fixture Metadata
bank: bbva
captured_at: 2026-02-20T15:36:15-03:00
captured_by: luken

## Files
See .html files in this directory.
Screenshots (.png) provided for visual reference.

## Shadow DOM + Iframe Flattening

Fixtures are captured using FlattenShadowDOM which recursively inlines both
shadow DOM content and iframe documents into a single parseable HTML document.

Shadow root content appears as:

    <custom-element>
      <!-- light DOM children -->
      <div data-shadow-root="true" data-shadow-host="custom-element">
        <style data-from-shadow="true">/* shadow styles */</style>
        <!-- shadow DOM content -->
      </div>
    </custom-element>

Iframe content appears as:

    <div data-captured-iframe="true" data-iframe-src="..." data-iframe-id="...">
      <style data-from-iframe="true">/* iframe styles */</style>
      <!-- iframe body content -->
    </div>

Parse flattened content with goquery:

    doc.Find("[data-shadow-root] .your-selector")
    doc.Find("[data-captured-iframe] .your-selector")

Target a specific shadow host:

    doc.Find("[data-shadow-host='bbva-btge-accounts-solution-page'] .balance")

## Notes
- These fixtures should be sanitized before committing
- Update when bank portal changes
- Re-run capture if tests start failing
