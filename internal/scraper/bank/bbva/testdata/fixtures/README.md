# Fixture Metadata
bank: bbva
captured_at: 2026-02-02T17:23:33-03:00
captured_by: luken

## Files
See .html files in this directory.
Screenshots (.png) provided for visual reference.

## Iframe Handling

Iframe content is automatically inlined during capture as:

    <div data-captured-iframe="true" data-iframe-src="..." data-iframe-name="...">
      <style data-from-iframe="true">/* iframe styles */</style>
      <!-- iframe body content -->
    </div>

Parse inlined iframe content with goquery:

    doc.Find("[data-captured-iframe] .your-selector")

Target a specific iframe by attribute:

    doc.Find("[data-iframe-name='content'] .balance-amount")

## Notes
- These fixtures should be sanitized before committing
- Update when bank portal changes
- Re-run capture if tests start failing
