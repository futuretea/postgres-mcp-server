// Package masking removes configured field names and values from text before
// a tool result is encoded. It does not detect every private value, compressed
// payload, or encoding.
//
// A policy is inactive unless enabled is true and at least one rule is set.
// Absent configuration is inactive. ParseJSON only parses one document.
// config.LoadConfig reads MCP_MASKING_JSON. When that variable is present,
// LoadConfig replaces the YAML masking object with the parsed policy. An
// empty or whitespace-only value is invalid. An unset variable leaves the
// YAML object in place.
//
// Name patterns are parsed on their own and then anchored to the whole name
// in the syntax tree. A pattern such as a)|(b is rejected because it is not a
// pattern by itself. A grouped alternation such as (?:a)|(b) matches only the
// whole names a and b. Value literals and value patterns are substring matches
// on the original text, applied once. The replacement is not searched again.
// A one-character literal is valid and can match inside ordinary words. A
// pattern is rejected when any alternative can match without consuming input,
// including a word-boundary branch. RE2 returns the leftmost-first match, so
// sk_|sk_live_[0-9]+ replaces the shorter sk_ alternative and does not search
// for a longer one.
//
// Pass one *Budget to every MaskText, MaskBytes, and MatchScalar call that
// belongs to the same result. A nil budget caps that call alone. Each visited
// string is added, and a JSON document is added again as the container, so a
// JSON value can exhaust the budget before a plain string of the same size.
// That accounting is fail-closed. Output size is checked before the replaced
// text is built. Span lists are capped the same way. WithinInput reports
// whether an encoded cell is small enough to decode. Callers check it before
// they allocate the decoded bytes.
//
// MatchScalar reports a hit on the original numeric or bool text. The caller
// replaces the whole scalar with null and does not edit inside the number.
// An unmatched number keeps its original literal, including trailing zeros.
//
// The guarantee is narrow. Callers choose which values are names, text,
// numbers, bools, or decoded bytes. Value patterns also run on ASCII
// subsequences inside invalid UTF-8.
// The placeholder and the fixed public errors are not scanned. A short
// literal that also occurs inside the placeholder or a fixed error can still
// be seen there. schema_version and the tool name are server constants and
// are not treated as stored data.
//
// A JSON null after masking can be a real SQL NULL or a redacted non-string.
// Types and nullability in the JSON can change when a rule matches.
package masking
