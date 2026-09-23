---
name: preserve-existing-behavior
description: "Preserves existing product behavior when extending, refactoring, migrating, or generalizing code. Must be used for changes to existing call paths, routing, defaults, shared helpers, data contracts, frontend content/resource resolution, completion semantics, or cross-group compatibility unless the user explicitly requests a compatibility break."
---

# Preserve Existing Behavior

Treat existing behavior as the default product contract. A new requirement may change only the scope the user explicitly named; all other established behavior must remain complete and unchanged.

Use this skill together with any language- or subsystem-specific skill required by the task.

## Non-negotiable rules

- Do not infer permission to change old behavior from a request to support a new group, data shape, resource type, or workflow.
- New behavior must have an explicit, stable boundary. Existing cases must continue through their original path or an equivalent path proven by tests.
- Preserve behavior across the whole flow, not only inside the edited helper: input, routing, transformation, persistence, API response, UI rendering, navigation, and completion state all count.
- Prefer a small explicit branch over a generalized helper that silently changes every caller.
- If the old behavior is unclear or cannot be proven unchanged, stop and ask the user before implementing.
- A deliberate compatibility break requires explicit user acknowledgement and must be recorded in tests and the change summary.

## Required workflow

### 1. Define the compatibility invariant

Before editing, write one sentence in this form:

```text
This change affects only <new scope>; <existing scope> retains <old behavior>.
```

If the sentence cannot identify both the new boundary and the old behavior, investigation is incomplete.

### 2. Inventory every affected path

Use `rg` to find all producers and consumers of the changed function, field, route, data model, helper, and rendered value. Classify at least:

- existing and new groups or business types;
- legacy and current data shapes;
- API, service, storage, and migration paths;
- frontend parsing, rendering, click/navigation, and completion paths;
- shared-helper callers and tests.

Do not review only the file named in the request. Follow the data until its user-visible behavior is determined.

### 3. Capture the old behavior first

Before changing implementation, add or identify a regression test, fixture, or exact reproducible check for the established behavior. It must assert the user-visible semantic result, not merely that an internal helper was called.

For a reported regression, first reproduce the failing old scenario. If automated coverage is impractical, document the exact input, action, expected result, and observed result and run that check after the change.

### 4. Build a compatibility matrix

For cross-group, migration, routing, or content-resolution changes, evaluate all relevant combinations rather than only the new happy path:

| Dimension | Cases to preserve or add |
| --- | --- |
| Group/data age | existing group + existing data; new group + legacy data; new group + new data |
| Resource binding | bound asset; direct URL; missing or partial binding |
| Reading metadata | page/range metadata; whole document; no page metadata |
| Content type | internal asset; external HTML; Markdown; image/audio/video; ordinary link |
| Completion | aggregate completion; per-item completion; mixed or partial completion |

Remove irrelevant rows, but never omit a case merely because the new implementation does not model it.

### 5. Make the narrowest change

- Gate new logic on stable domain facts such as an explicit group/type/version/capability/feature flag.
- Keep legacy inputs on the legacy semantic path.
- Do not broaden a condition only to reuse code.
- Do not move a decision to an earlier layer unless all required semantic information remains available there.
- Do not replace structured domain data with a transport representation before the final consumer has made its decision.

## Preserve semantic information across layers

Generalized routing must not take priority over existing semantic metadata.

In particular:

- A transport URL such as `/api/assets/{id}/download` says how bytes can be fetched; it does not by itself say whether the user should open the whole file, a page range, a reader, or another view.
- Do not infer resource or media semantics solely from URL shape or file extension when first-class metadata exists.
- Preserve asset ID, MIME type, category, page range, source title, task/item identity, and completion semantics until the layer that owns the final decision.
- When both structured metadata and a generic fallback exist, evaluate the structured existing behavior first and use the fallback only when its prerequisites are absent.

Typical regression pattern:

```text
structured resource + page range
  -> prematurely converted to generic download URL
  -> generic URL router wins
  -> page-range behavior is lost
```

The compatible design keeps structured resource metadata intact and lets the final navigation/rendering layer choose the range-aware behavior before considering generic URL fallback.

## Required verification

Tests must cover all of the following:

1. Existing behavior remains unchanged for a representative old case.
2. The intended new behavior works for the explicitly requested new case.
3. A boundary case proves existing inputs do not enter the new branch.
4. A realistic end-to-end combination proves metadata survives across layers; isolated helper tests alone are insufficient for routing or UI behavior.

For bug fixes, the regression test must fail on the faulty implementation and pass after the fix whenever reasonably possible.

Before finishing, inspect the complete diff and confirm:

- every changed line is required by the explicit request;
- no old caller acquired a new default, validation, conversion, filter, route, or completion rule;
- no structured field was discarded before its last semantic consumer;
- unchanged behavior is asserted, not merely assumed;
- any intentional behavior change is explicitly authorized and documented.

## Stop conditions

Stop and ask the user when:

- two plausible interpretations would change different existing behavior;
- production behavior cannot be reconstructed from code, tests, data, or a reproducible example;
- the requested new behavior inherently conflicts with an old contract outside the named scope;
- verification requires unavailable data or an external decision that materially affects compatibility.

Do not resolve these cases by silently choosing the more generalized implementation.
