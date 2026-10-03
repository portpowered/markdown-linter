# Strunk and White editorial rules

The opt-in `text:strunk-white` pack contains **10 checks: 8 atomic lexical checks and 2 paired-construction checks**. All produce review warnings and offer no automatic rewrites. They run locally and need no dictionary, model, or network access.

## Source analysis and counts

The fourth-edition publisher contents enumerate **43 principles: 11 usage rules, 11 composition principles, and 21 style reminders**. Matters of form and the usage glossary are unnumbered, so 43 is the count of numbered principles, not every possible editorial observation. Concision, concrete language, active voice, positive statements, qualifiers, familiar vocabulary, and conventional usage motivate this pack. The contextual principles require editorial judgment. [Pearson fourth-edition contents](https://www.pearson.com/en-gb/subject-catalog/p/Strunk-Elements-of-Style-The-Pearson-New-International-Edition-4th-Edition/P200000005508)

Strunk's original edition provides the accessible source text for examples of wordiness and usage. It distinguishes reviewable constructions from decisions about paragraphs, emphasis, and coordination. We paraphrase guidance and use our own examples; we do not distribute the later copyrighted book. [Strunk's original text, Project Gutenberg](https://www.gutenberg.org/cache/epub/37134/pg37134-images.html)

The publisher also describes the later Strunk and White edition as guidance on clarity and active voice. The pack name acknowledges that tradition; the implementation does not claim complete book coverage or author endorsement. [Penguin Random House edition](https://www.penguinrandomhouse.com/books/294830/the-elements-of-style-illustrated-by-strunk-white-kalman/)

Our engineering decomposition contains ten independently configurable checks. Multiple checks implement parts of one broad principle, and several principles motivate a single check. **Ten checks therefore does not mean ten of the 43 principles are fully automated.** General subject agreement, comma splices, dangling modifiers, pronoun case, topic sentences, tense consistency, rhetorical emphasis, and parallel grammar remain human review. A finite token detector cannot reliably determine these from technical prose.

## Activate and compose

```sh
marklint init --preset text:strunk-white --output .marklint.yaml
marklint --root . --fail-on warning docs
marklint rules describe text.strunk-white.passive-voice
```

To retain the normal Markdown checks, compose the packs:

```yaml
version: 1
extends: [markdown:recommended, text:strunk-white]
overrides:
  - id: text.strunk-white.qualifiers
    options:
      allow: [quite]
      scope: prose
  - id: text.strunk-white.passive-voice
    include: [handbook/**]
```

`scope: prose` includes paragraphs and headings. `scope: heading` includes headings only. `allow` contains complete matched expressions, case insensitive, with whitespace normalized. Include/exclude patterns, severity overrides, reasoned suppressions, baselines, and `--only` use the existing rule-pack contract. A domain term can be allowed, or a rule can be disabled for a document family. No editorial pack is enabled by the default recommended pack.

## Rule checklist and exact boundaries

The following table is the complete shipped behavior. Examples are original test fixtures.

| Implemented rule ID | Signal | Review example | Cleaner example | Accepted boundary |
| --- | --- | --- | --- | --- |
| text.strunk-white.needless-words | Eight finite wordy expressions | Run in order to verify. | Run to verify. | Put the records in order. |
| text.strunk-white.fancy-words | Five finite formal words | Utilize the cache. | Use the cache. | A code identifier such as `utilization` |
| text.strunk-white.qualifiers | very, really, rather, quite | A very large result. | A result of 20 MB. | Every result is cached. |
| text.strunk-white.negative-phrases | Four finite indirect expressions | This is not uncommon. | This is common. | Do not retry failed writes. |
| text.strunk-white.passive-voice | Auxiliary, listed participle, then by | The record was created by the worker. | The worker created the record. | The worker was ready by noon. |
| text.strunk-white.usage | irregardless and five modal/of pairs | It should of worked. | It should have worked. | A person of interest arrived. |
| text.strunk-white.exclamations | Exclamation runs in prose | Done! | Done. | Markdown image markers |
| text.strunk-white.existential-openings | Sentence-opening there plus is/are/was/were | There are three records. | Three records exist. | A mid-sentence occurrence of there is |
| text.strunk-white.redundant-pairs | Six finite redundant expressions | Review the final outcome. | Review the outcome. | This is the final record. |
| text.strunk-white.correlative-pairs | both/or, either/and, neither/or before a matching counterpart | Use both red or blue. | Use both red and blue. | Use both red and blue or choose green. |

Needless-word patterns are: in order to; due to the fact that; at this point in time; in the event that; for the purpose of; the question as to whether; in a manner that; on a daily basis. Fancy-word patterns are: utilize; utilization; aforementioned; heretofore; henceforth. Negative phrases are: not uncommon; not impossible; not unlikely; not without. Redundant pairs are: advance planning; basic fundamentals; final outcome; each and every; free gift; past history. These lists are implementation policy, not a comprehensive vocabulary taken from the book.

Passive detection uses only is/are/was/were/be/been/being followed by approved, created, deleted, modified, sent, written, read, processed, returned, requested, performed, used, made, done, seen, given, taken, built, stored, or generated; optional intervening adverbs are carefully, automatically, or explicitly. It requires a following by. Agentless passive sentences are outside this detector. Active and passive choices both have legitimate uses.

Correlative detection inspects text within a paragraph or heading and stops at a period, exclamation, question mark, or semicolon. It only detects a conflicting counterpart. It does not infer grammatical parallelism, demand a second half in sentence fragments, or understand nested coordination. It is a warning because a nested construction can match the finite pattern despite being intentional.

## Interactions and pairing analysis

There are **2 executable pairing checks** and **4 editorial interactions** worth reviewing together:

| Interaction | Checks or packs | Customer decision |
| --- | --- | --- |
| Concision and emphasis | needless-words, qualifiers, redundant-pairs | Review all signals before deleting words; retained emphasis can be deliberate. |
| Positive wording and actors | negative-phrases, passive-voice, existential-openings | Preserve negation, responsibility, and uncertainty while rewriting. |
| Familiar wording and domain vocabulary | fancy-words, usage, text.terminology | Share exceptions for technical terms and avoid configuring contradictory preferred terms. |
| Dash punctuation and internal policy | text:strunk-white, portos:internal | The source permits rhetorical dashes. Portos prohibits prose dashes; this pack adds no competing punctuation rule. |

These four interactions are documentation groupings, not four additional algorithms. The ten-rule pack never silently enables terminology or the Portos prose bans.

## Tests and false positives

`pkg/rulepack/strunk_white_test.go` verifies a triggering example, a cleaner example, and an accepted boundary for every rule. It also verifies code/URL exclusions, multi-line and emphasized phrases, paragraph boundaries, frontmatter exclusions, allow lists, heading scope, option validation, canceled analysis, paired constructions, and composition with recommended rules. The composed fixture asserts exact finding count, warning severity, originating preset, and the effect of disabling a rule.

Checks inspect parsed text in paragraphs and headings. They exclude fenced and inline code, image alternative text, raw HTML tokens, link destinations, bare URLs, and YAML frontmatter. Link labels and block-quoted prose remain reviewable. Source byte positions are preserved across inline emphasis. Expressions never bridge separate paragraphs.

The pack is English lexical guidance. Quotes, headings, mathematical terminology, and deliberate emphasis can produce useful or unwanted suggestions. For example, a utilization metric may legitimately use that noun; add it to the allow list. A phrase such as not impossible carries weaker certainty than possible; never rewrite automatically. An exclamation in quoted dialogue may be intentional. Runtime warnings remain warnings unless customers promote them through configuration or `--fail-on warning`.
