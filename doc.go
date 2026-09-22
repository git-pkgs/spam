// Package spam scans caller-selected package text for promotional signals.
// It returns weighted findings, original byte offsets and per-file measurements.
// Scores are uncalibrated; callers supply any thresholds or enforcement policy.
//
// ScanFile uses the built-in defaults. NewScanner accepts additional raw-text
// rules, weight overrides and disabled rule IDs. Reuse a Scanner across calls;
// each scan has independent mutable state and supports concurrent use.
//
// Documents are UTF-8 bytes supplied by the caller. Paths are labels: scanning
// does not open files, fetch URLs, render markup or execute package contents.
// Input limits apply to the full call, including byte-identical duplicates.
// Evidence excerpts contain untrusted text and need escaping when displayed.
package spam
