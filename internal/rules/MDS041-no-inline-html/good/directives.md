# Document with Directives

Block-level directives are parsed as ProcessingInstruction
nodes, not HTMLBlocks, so they are never flagged.

Inline directives inside paragraphs are RawHTML nodes whose
bytes start with `<?`, so they are skipped unconditionally.

The placeholder <?foo?> is ignored.

No diagnostics are emitted regardless of settings.
