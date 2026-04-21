# HNG 14 Tasks

This repository is used to track and store my progress on tasks assigned during the HNG 3-month internship program.

It serves as a structured space for documenting solutions, experiments, and improvements made throughout the internship journey.

## Natural Language Query (NLQ) — Strategy, Coverage & Limitations

### Strategy: Rule-Based Keyword Matching

The NLQ parser (`utils.go` → `parseNLQuery`) is a **pure keyword-scanning approach** — it's primarily by Rule-based parsing only, no ML, no grammar engine, no intent inference. It works simply by:

1. Lowercasing the entire query string
2. Splitting it into individual tokens
3. Scanning those tokens for a fixed set of recognised keywords
4. Mapping matched keywords to fields on a `ProfileFilter` struct, which is then passed directly to the existing filtered DB query

This means every NLQ search ultimately resolves to the same SQL filter path used by the structured `/profiles` endpoint — the NLQ layer is purely a translation step.

---

## What It Covers

| Intent                | Recognised trigger words                                           | Filter produced              |
| --------------------- | ------------------------------------------------------------------ | ---------------------------- |
| **Gender**            | `male`, `males`, `man`, `men`, `boy`, `boys`                       | `Gender = "male"`            |
|                       | `female`, `females`, `woman`, `women`, `girl`, `girls`             | `Gender = "female"`          |
| **Age group**         | `teenager`, `teen`, `teenagers`, `teens`                           | `AgeGroup = "teenager"`      |
|                       | `adult`, `adults`                                                  | `AgeGroup = "adult"`         |
|                       | `child`, `children`, `kid`, `kids`                                 | `AgeGroup = "child"`         |
|                       | `senior`, `seniors`, `elderly`                                     | `AgeGroup = "senior"`        |
| **"Young" shorthand** | `young`                                                            | `MinAge = 16`, `MaxAge = 24` |
| **Minimum age**       | `above X`, `over X` (number must follow immediately)               | `MinAge = X`                 |
| **Maximum age**       | `below X`, `under X` (number must follow immediately)              | `MaxAge = X`                 |
| **Country**           | `from <country name>` (longest-prefix match against hardcoded map) | `CountryID = <ISO code>`     |

### Example queries that work

```
"young females from Nigeria"
"adult males above 25"
"senior women from Japan"
"men below 40 from Germany"
"teens from Brazil"
```

---

## Limitations

### Vocabulary gaps

- **Adjective country forms are not supported.** `"Nigerian males"` or `"French women"` will not extract a country — only the `"from <name>"` pattern works.
- **ISO codes in the query don't resolve.** `"from NG"` or `"from US"` will not match — only full country names are recognised.
- **Hardcoded country list (~70 entries).** Countries outside the map (e.g. Libya, Cuba, Nepal, Bolivia) are silently ignored — no filter is applied and no error is raised.
- **No synonym support.** Words like `"gentleman"`, `"gent"`, `"lad"`, `"lass"`, `"toddler"`, `"pensioner"`, `"retiree"` are not recognised.

### Logic gaps

- **No negation.** Queries like `"not male"` or `"excluding seniors"` are not understood; the keywords `male` and `senior` would still be matched positively.
- **No range shorthand.** `"between 20 and 40"` is unrecognised. The equivalent must be written as `"above 20 below 40"`.
- **`young` + `above/over X` conflict.** `young` sets `MinAge=16, MaxAge=24`. If `above X` also appears, it overrides `MinAge` but leaves `MaxAge=24`, which can produce an impossible range (e.g. `"young above 30"` → `MinAge=30, MaxAge=24`).
- **No OR logic.** `"males or females from Brazil"` would detect both gender keywords and cancel them out — no gender filter is applied.
- **No sorting from NLQ.** `"oldest males"` or `"youngest women from India"` will correctly extract gender/country but the `oldest`/`youngest` intent is silently ignored; results always return in `ASC` order.

### Structural gaps

- **No name search.** `"profiles named James"` or `"find John"` is completely unsupported.
- **Word order sensitivity.** `"above 30 males from Nigeria"` works, but a trailing dangling `from` (e.g. `"males from"`) will attempt a country lookup against an empty string and silently produce no country filter.
- **No feedback on unrecognised queries.** If zero keywords are matched, the handler returns a `422` with the message `"Unable to interpret query"` — the caller receives no hint about what syntax is accepted.

---

## Summary

The NLQ layer is best understood as a **structured-query shorthand translator**, not a general-purpose language understander. It handles simple, well-formed plain-English filters efficiently and without any external dependencies. Anything requiring semantic understanding, negation, synonyms outside its fixed vocabulary, or compound/range logic is outside its scope.
