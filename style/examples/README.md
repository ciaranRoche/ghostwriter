# Writing Examples

Place your curated writing samples in this directory. These examples are
the most important input to the system. AI tools learn your style primarily
from seeing how you actually write.

## How many?

10-15 examples is the sweet spot. Enough to cover your range of styles
without overwhelming the context window.

## What makes a good example?

Pick comments that showcase **different aspects** of your writing:

- A **nitpick** (brief, low-pressure)
- A **bug catch** (direct, with a concrete fix)
- A **documentation feedback** comment (empathetic, reader-focused)
- A **praise / positive review** (genuine, specific)
- An **architecture concern** (detailed, with alternatives)
- A **clarifying question** (genuine curiosity, no hedging)
- A **review summary** (severity calibration)
- A **teaching moment** (explains a general principle)
- A **cross-cutting concern** (flags impact across components)

Avoid picking 10 examples that all sound the same. Variety is key.

## File format

Each file should be a `.md` file with the example label as the filename:

```
style/examples/
├── bug-catch-with-fix.md
├── nitpick.md
├── architecture-concern.md
├── positive-review.md
├── clarifying-question.md
├── documentation-feedback.md
├── review-summary-nonblocking.md
├── review-summary-substantive.md
├── teaching-moment.md
└── cross-component-coupling.md
```

Each file should contain just the raw comment text, exactly as you wrote
it. No metadata or headers needed. For example:

```markdown
This `continue` skips `evalSpan.End()`, the span was started on line 150
but never gets closed when the resource has an empty ID. Over time this'll
leak spans in the tracing backend and accumulate memory in long-running
processes.

Quick fix, just end the span before continuing:

\`\`\`go
evalSpan.End()
continue
\`\`\`
```

## Where to find your examples

- **GitHub PR reviews**: Use `gh api` or browse your review history
- **JIRA comments**: Export from your ticket history
- **Slack messages**: Search for your longer technical messages

The `rag/collect-github-reviews.sh` script will pull your full PR review
history automatically. You can pick your best examples from that corpus.
