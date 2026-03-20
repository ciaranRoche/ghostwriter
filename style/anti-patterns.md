# Anti-Patterns

This document defines what the generated text should **never** do. These
rules take priority over all other style guidance. If any generated output
contains these patterns, it has failed the style check.

## Formatting

- **Dashes as connectors**: Never use em dashes, en dashes, or double
  hyphens as sentence connectors, for asides, or for pauses. Use commas,
  periods, or natural sentence flow instead.
  <!-- CUSTOMIZE: Remove this rule if you naturally use dashes -->

- **Emoji in technical feedback**: Never use emoji in PR reviews or code
  comments.
  <!-- CUSTOMIZE: Remove this rule if you use emoji in reviews -->

## Tone

- **Corporate/formal language**: Never use phrases like:
  - "Please be advised"
  - "It is imperative"
  - "It is recommended that"
  - "Kindly ensure"
  - "As per best practices"
  - "Per the documentation"
  - "It should be noted that"
  <!-- CUSTOMIZE: Add or remove phrases based on your own anti-patterns -->

- **Overly cautious hedging**: Don't say "I might be wrong but perhaps
  maybe consider..." Be direct while remaining kind.

- **Addressing the person as "the author" or "the developer"**: Use "you"
  or name them directly.

## Content

- **Unexplained comments**: Never just say "change X to Y" or "this is
  wrong" without explaining why it matters.

- **Generic praise**: Never use bare "LGTM", "looks good", or "nice"
  without specifics about what was done well.

- **Bullet-point-only reviews**: Don't just list issues. Weave in context,
  consequences, and rationale.

- **Passive voice where active is clearer**: Prefer "This changes the
  security posture" over "The security posture is changed by this."

## Negative Examples

Study these bad-vs-good comparisons:

<!-- CUSTOMIZE: Replace these with examples from your own style -->

**Dashes instead of natural flow:**
```
BAD:  "This continue skips evalSpan.End() -- the span was started on line 150 -- but never gets closed."
GOOD: "This `continue` skips `evalSpan.End()`, the span was started on line 150 but never gets closed when the resource has an empty ID."
```

**Corporate tone instead of casual:**
```
BAD:  "It is recommended that you implement proper error handling for this edge case. Please ensure the function validates input parameters."
GOOD: "Might be worth adding some error handling here. If someone passes an empty string, this will silently fail and you won't know until production."
```

**Unexplained comment:**
```
BAD:  "Use `http.NewRequestWithContext` instead of `http.Client.Get`."
GOOD: "Better to use `http.NewRequestWithContext` vs using `http.Client.Get`, so that we can pass context here. Then any cancellation/shutdown signals are not ignored."
```
