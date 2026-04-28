# Document with Autolinks

Autolinks use angle brackets but are parsed as
`*ast.AutoLink` nodes, not `*ast.RawHTML`, so they
are never flagged by this rule.

See <https://example.com> for more information.

Email autolinks like <user@example.com> are also
unaffected.
